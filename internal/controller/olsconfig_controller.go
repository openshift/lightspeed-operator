/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller implements the main Kubernetes controller for managing
// the OpenShift Lightspeed operator lifecycle.
//
// This package contains the OLSConfigReconciler, which is the central orchestrator
// for the entire operator. It coordinates reconciliation across all components
// (appserver, postgres, console) and manages the OLSConfig custom resource.
//
// Key Responsibilities:
//   - Reconcile the OLSConfig custom resource
//   - Coordinate component reconciliation (appserver, postgres, console)
//   - Manage status conditions and CR status updates
//   - Delegate LLM provider secret reconciliation to appserver package
//   - Set up resource watchers for automatic updates (secrets, configmaps)
//   - Manage operator-level resources (service monitors, network policies)
//
// The main reconciliation flow:
//  1. Reconcile operator-level resources (service monitor, network policy)
//  2. Delegate LLM provider secret reconciliation to appserver.ReconcileLLMSecrets()
//  3. Reconcile PostgreSQL database (if conversation cache enabled)
//  4. Reconcile Console UI plugin
//  5. Reconcile application server components
//  6. Update status conditions based on deployment readiness
//
// The OLSConfigReconciler implements the reconciler.Reconciler interface,
// allowing it to be passed to component packages for isolated reconciliation
// without circular dependencies.
package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/appserver"
	"github.com/openshift/lightspeed-operator/internal/controller/console"
	"github.com/openshift/lightspeed-operator/internal/controller/lcore"
	"github.com/openshift/lightspeed-operator/internal/controller/postgres"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	"github.com/openshift/lightspeed-operator/internal/controller/watchers"
)

// OLSConfigReconciler reconciles a OLSConfig object
type OLSConfigReconciler struct {
	client.Client
	Logger            logr.Logger
	Options           utils.OLSConfigReconcilerOptions
	WatcherConfig     *utils.WatcherConfig
	NextReconcileTime time.Time
}

// Implement reconciler.Reconciler interface
func (r *OLSConfigReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme()
}

func (r *OLSConfigReconciler) GetLogger() logr.Logger {
	return r.Logger
}

func (r *OLSConfigReconciler) GetNamespace() string {
	return r.Options.Namespace
}

func (r *OLSConfigReconciler) GetPostgresImage() string {
	return r.Options.LightspeedServicePostgresImage
}

func (r *OLSConfigReconciler) GetConsoleUIImage() string {
	return r.Options.ConsoleUIImage
}

func (r *OLSConfigReconciler) GetOpenShiftMajor() string {
	return r.Options.OpenShiftMajor
}

func (r *OLSConfigReconciler) GetOpenshiftMinor() string {
	return r.Options.OpenshiftMinor
}

func (r *OLSConfigReconciler) GetAppServerImage() string {
	return r.Options.LightspeedServiceImage
}

func (r *OLSConfigReconciler) GetOpenShiftMCPServerImage() string {
	return r.Options.OpenShiftMCPServerImage
}

func (r *OLSConfigReconciler) GetDataverseExporterImage() string {
	return r.Options.DataverseExporterImage
}

func (r *OLSConfigReconciler) GetLCoreImage() string {
	return r.Options.LightspeedCoreImage
}

func (r *OLSConfigReconciler) IsPrometheusAvailable() bool {
	return r.Options.PrometheusAvailable
}

func (r *OLSConfigReconciler) GetWatcherConfig() interface{} {
	return r.WatcherConfig
}

func (r *OLSConfigReconciler) UseLCore() bool {
	return r.Options.UseLCore
}

// shouldWatchSecret is a predicate that determines if a secret should be watched.
// This provides performance optimization by filtering at the watch level, preventing
// unnecessary reconciliation triggers for secrets the operator doesn't care about.
// Returns true if:
// 1. The secret has the watcher annotation (annotated by operator for change tracking)
// 2. The secret is configured as a system resource (external resource like pull-secret)
func (r *OLSConfigReconciler) shouldWatchSecret(obj client.Object) bool {
	// Check 1: Has watcher annotation?
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if _, exists := annotations[utils.WatcherAnnotationKey]; exists {
			return true
		}
	}

	// Check 2: Is it a configured system secret?
	if r.WatcherConfig != nil {
		for _, systemSecret := range r.WatcherConfig.Secrets.SystemResources {
			if obj.GetNamespace() == systemSecret.Namespace &&
				obj.GetName() == systemSecret.Name {
				return true
			}
		}
	}

	return false
}

// shouldWatchConfigMap is a predicate that determines if a configmap should be watched.
// This provides performance optimization by filtering at the watch level, preventing
// unnecessary reconciliation triggers for configmaps the operator doesn't care about.
// Returns true if:
// 1. The configmap has the watcher annotation (annotated by operator for change tracking)
// 2. The configmap is configured as a system resource (external resource like CA bundle)
func (r *OLSConfigReconciler) shouldWatchConfigMap(obj client.Object) bool {
	// Check 1: Has watcher annotation?
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if _, exists := annotations[utils.WatcherAnnotationKey]; exists {
			return true
		}
	}

	// Check 2: Is it a configured system configmap?
	if r.WatcherConfig != nil {
		for _, systemConfigMap := range r.WatcherConfig.ConfigMaps.SystemResources {
			if obj.GetNamespace() == systemConfigMap.Namespace &&
				obj.GetName() == systemConfigMap.Name {
				return true
			}
		}
	}

	return false
}

// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/finalizers,verbs=update
// RBAC for managing deployments of OLS application server
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// Service for exposing lightspeed service API endpoints
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// ServiceAccount to run OLS application server
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// ConfigMap for OLS application server configuration
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// ConfigMap in other namespaces:
// - client CA certificate from openshift-monitoring namespace
// OLM cannot create a role and rolebinding for a specific single namespace that is not the namespace the operator is installed in and/or watching
// This has to be a cluster role
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// Secret access for conversation cache server configuration
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete;deletecollection
// Secret access for telemetry pull secret, must be a cluster role due to OLM limitations in managing roles in operator namespace
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=secrets,resourceNames=pull-secret,verbs=get;list;watch
// ConsolePlugin for install console plugin
// +kubebuilder:rbac:groups=console.openshift.io,resources=consolelinks;consoleexternalloglinks;consoleplugins;consoleplugins/finalizers,verbs=get;create;update;delete
// Modify console CR to activate console plugin
// +kubebuilder:rbac:groups=operator.openshift.io,resources=consoles,verbs=watch;list;get;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=list;create;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace=openshift-lightspeed,resources=roles;rolebindings,verbs=*

// RBAC for application server to authorize user for API access
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create

// ServiceMonitor for monitoring OLS application server
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// PrometheusRule for aggregating OLS metrics for telemetry
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete

// clusterversion for checking the openshift cluster version
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions;apiservers,verbs=get;list;watch

// NetworkPolicy for restricting access to OLS pods
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// PVC access for the Postgres PVC
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *OLSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Reconcile operator's resources first
	operatorReconcileFuncs := []utils.OperatorReconcileFuncs{}

	// Skip ServiceMonitor in local development mode (requires Prometheus Operator CRDs)
	// Set LOCAL_DEV_MODE=true when running locally with "make run-local"
	if os.Getenv("LOCAL_DEV_MODE") != "true" {
		operatorReconcileFuncs = append(operatorReconcileFuncs,
			utils.OperatorReconcileFuncs{
				Name: "service monitor for operator",
				Fn:   r.ReconcileServiceMonitorForOperator,
			})
	}

	// Network policy works in all environments
	operatorReconcileFuncs = append(operatorReconcileFuncs,
		utils.OperatorReconcileFuncs{
			Name: "network policy for operator",
			Fn:   r.ReconcileNetworkPolicyForOperator,
		})

	for _, reconcileFunc := range operatorReconcileFuncs {
		err := reconcileFunc.Fn(ctx)
		if err != nil {
			r.Logger.Error(err, fmt.Sprintf("Failed to reconcile %s", reconcileFunc.Name))
			return ctrl.Result{}, err
		}
	}
	// The operator reconciles only for OLSConfig CR with a specific name
	if req.Name != utils.OLSConfigName {
		r.Logger.Info(fmt.Sprintf("Ignoring OLSConfig CR other than %s", utils.OLSConfigName), "name", req.Name)
		return ctrl.Result{}, nil
	}

	olsconfig := &olsv1alpha1.OLSConfig{}
	err := r.Get(ctx, req.NamespacedName, olsconfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Logger.Info("olsconfig resource not found. Ignoring since object must be deleted")
			err = console.RemoveConsoleUI(r, ctx)
			if err != nil {
				r.Logger.Error(err, "Failed to remove console UI")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.Logger.Error(err, "Failed to get olsconfig")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, err
	}
	r.Logger.Info("reconciliation starts", "olsconfig generation", olsconfig.Generation)

	// Annotation Step: Annotate external resources and validate they exist
	// This ensures all user-provided external resources (secrets, configmaps) are properly
	// annotated for watching before we reconcile deployments that depend on them.
	// This also validates that the resources exist (fail fast if they're missing).
	if err := r.annotateExternalResources(ctx, olsconfig); err != nil {
		r.Logger.Error(err, "Failed to annotate external resources")
		r.UpdateStatusCondition(ctx, olsconfig, utils.TypeCRReconciled, false, "Failed", err)
		return ctrl.Result{RequeueAfter: 1 * time.Second}, err
	}

	// Phase 1: Reconcile independent resources for all components
	// This phase creates ConfigMaps, Secrets, ServiceAccounts, Roles, NetworkPolicies, etc.
	// These resources are independent and can be reconciled in any order without race conditions.
	// We use a continue-on-error pattern here to reconcile as many resources as possible,
	// even if some fail, to maximize progress in each reconciliation loop.
	resourceSteps := []utils.ReconcileSteps{
		{Name: "console UI resources", Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
			return console.ReconcileConsoleUIResources(r, ctx, cr)
		}},
		{Name: "postgres resources", Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
			return postgres.ReconcilePostgresResources(r, ctx, cr)
		}},
	}

	// Conditionally add either LCore or AppServer resource reconciliation
	if r.Options.UseLCore {
		resourceSteps = append(resourceSteps, utils.ReconcileSteps{
			Name: "LCore resources",
			Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return lcore.ReconcileLCoreResources(r, ctx, cr)
			},
		})
	} else {
		resourceSteps = append(resourceSteps, utils.ReconcileSteps{
			Name: "application server resources",
			Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return appserver.ReconcileAppServerResources(r, ctx, cr)
			},
		})
	}

	// Reconcile all independent resources (continue on error to reconcile as many as possible)
	resourceFailures := make(map[string]error)
	for _, step := range resourceSteps {
		if err := step.Fn(ctx, olsconfig); err != nil {
			r.Logger.Error(err, "Resource reconciliation failed", "resource", step.Name)
			resourceFailures[step.Name] = err
		}
	}

	if len(resourceFailures) > 0 {
		taskNames := make([]string, 0, len(resourceFailures))
		for taskName := range resourceFailures {
			taskNames = append(taskNames, taskName)
		}
		return ctrl.Result{}, fmt.Errorf("failed to reconcile resources: %v", taskNames)
	}

	// Phase 2: Reconcile deployments and their dependent resources
	// This phase creates Deployments, Services, TLS certificates, ServiceMonitors, etc.
	// These resources depend on Phase 1 resources being available (e.g., Services must exist
	// before TLS certificates can be created by service-ca-operator, ConfigMaps must exist
	// before Deployments can mount them). We use a fail-fast pattern here because deployment
	// failures should stop the reconciliation and update status conditions appropriately.
	deploymentSteps := []utils.ReconcileSteps{
		{Name: "console UI deployment", Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
			return console.ReconcileConsoleUIDeploymentAndPlugin(r, ctx, cr)
		}, ConditionType: utils.TypeConsolePluginReady, Deployment: utils.ConsoleUIDeploymentName},
		{Name: "postgres deployment", Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
			return postgres.ReconcilePostgresDeployment(r, ctx, cr)
		}, ConditionType: utils.TypeCacheReady, Deployment: utils.PostgresDeploymentName},
	}

	// Conditionally add either LCore or AppServer deployment reconciliation
	if r.Options.UseLCore {
		deploymentSteps = append(deploymentSteps, utils.ReconcileSteps{
			Name: "LCore deployment",
			Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return lcore.ReconcileLCoreDeployment(r, ctx, cr)
			},
			ConditionType: utils.TypeApiReady,
			Deployment:    "lightspeed-stack-deployment",
		})
	} else {
		deploymentSteps = append(deploymentSteps, utils.ReconcileSteps{
			Name: "application server deployment",
			Fn: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return appserver.ReconcileAppServerDeployment(r, ctx, cr)
			},
			ConditionType: utils.TypeApiReady,
			Deployment:    utils.OLSAppServerDeploymentName,
		})
	}

	// Execute deployment reconciliation (fail-fast on errors)
	failedTasks := make(map[string]error)
	progressing := false

	for _, step := range deploymentSteps {
		err := step.Fn(ctx, olsconfig)
		if err != nil {
			r.Logger.Error(err, fmt.Sprintf("Failed to reconcile %s", step.Name))
			r.UpdateStatusCondition(ctx, olsconfig, step.ConditionType, false, "Failed", err)
			failedTasks[step.Name] = err
		} else {
			// Get corresponding deployment
			deployment := &appsv1.Deployment{}
			err := r.Get(ctx, client.ObjectKey{Name: step.Deployment, Namespace: r.Options.Namespace}, deployment)
			if err != nil {
				r.UpdateStatusCondition(ctx, olsconfig, step.ConditionType, false, "Failed", err)
				failedTasks[step.Name] = err
			} else {
				message, err := r.checkDeploymentStatus(deployment)
				if err != nil {
					if message == utils.DeploymentInProgress {
						// Deployment is not ready
						r.UpdateStatusCondition(ctx, olsconfig, step.ConditionType, false, message, nil)
						progressing = true
					} else {
						// Deployment failed
						r.UpdateStatusCondition(ctx, olsconfig, step.ConditionType, false, "Failed", err)
						failedTasks[step.Name] = err
					}
				} else {
					// Update status condition for successful reconciliation
					r.UpdateStatusCondition(ctx, olsconfig, step.ConditionType, true, "All components are successfully deployed", nil)
				}
			}
		}
	}

	if len(failedTasks) > 0 {
		// One of the deployment reconciliations failed
		taskNames := make([]string, 0, len(failedTasks))
		for taskName := range failedTasks {
			taskNames = append(taskNames, taskName)
		}
		return ctrl.Result{}, fmt.Errorf("failed deployment reconciliation tasks: %v", taskNames)
	}
	if progressing {
		return ctrl.Result{RequeueAfter: r.Options.ReconcileInterval}, nil
	}

	r.Logger.Info("reconciliation done", "olsconfig generation", olsconfig.Generation)

	// Update status condition for Custom Resource
	r.UpdateStatusCondition(ctx, olsconfig, utils.TypeCRReconciled, true, "Custom resource successfully reconciled", nil)

	// Requeue if no reconciliation is scheduled in future.
	if r.NextReconcileTime.After(time.Now()) {
		return ctrl.Result{}, nil
	}
	r.NextReconcileTime = time.Now().Add(r.Options.ReconcileInterval)
	r.Logger.Info("Next automatic reconciliation scheduled at", "nextReconcileTime", r.NextReconcileTime)
	return ctrl.Result{RequeueAfter: r.Options.ReconcileInterval}, nil
}

// updateStatusCondition updates the status condition of the OLSConfig Custom Resource instance.
// TODO: Should we support Unknown status and ObservedGeneration?
// TODO: conditionType must be metav1.Condition?
func (r *OLSConfigReconciler) UpdateStatusCondition(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig, conditionType string, status bool, message string, err error, inCluster ...bool) {
	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	condition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionUnknown,
		LastTransitionTime: metav1.Time{},
		Reason:             "Reconciling",
	}

	if status {
		condition.Status = metav1.ConditionTrue
	} else {
		condition.Status = metav1.ConditionFalse
	}

	if err != nil {
		condition.Message = fmt.Sprintf("%s: %v", message, err)
	} else {
		condition.Message = message
	}

	if inClusterValue {
		// Retry status update on conflicts, refetching latest version each time
		if updateErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Get latest version for status update
			currentOLSConfig := &olsv1alpha1.OLSConfig{}
			if getErr := r.Get(ctx, client.ObjectKey{Name: olsconfig.Name, Namespace: olsconfig.Namespace}, currentOLSConfig); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					r.Logger.V(1).Info("OLSConfig not found during status update, skipping", "name", olsconfig.Name)
					return nil // Don't retry NotFound errors
				}
				return getErr
			}

			// Apply the condition to the current version
			meta.SetStatusCondition(&currentOLSConfig.Status.Conditions, condition)

			// Attempt status update
			return r.Status().Update(ctx, currentOLSConfig)
		}); updateErr != nil {
			if !apierrors.IsNotFound(updateErr) {
				r.Logger.Error(updateErr, utils.ErrUpdateCRStatusCondition, "name", olsconfig.Name)
			}
		}
	} else {
		meta.SetStatusCondition(&olsconfig.Status.Conditions, condition)
		if updateErr := r.Status().Update(ctx, olsconfig); updateErr != nil {
			r.Logger.Error(updateErr, utils.ErrUpdateCRStatusCondition)
		}
	}
}

// checkDeploymentStatus checks if the deployment is ready and available
func (r *OLSConfigReconciler) checkDeploymentStatus(deployment *appsv1.Deployment) (string, error) {

	// Check if deployment has the expected number of replicas ready
	if deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
		return utils.DeploymentInProgress, fmt.Errorf("deployment not ready: %d replicas available",
			deployment.Status.ReadyReplicas)
	}

	// Check deployment conditions
	for _, condition := range deployment.Status.Conditions {
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			if condition.Status != corev1.ConditionTrue {
				return utils.DeploymentInProgress, fmt.Errorf("deployment not available: %s - %s", condition.Reason, condition.Message)
			}
		case appsv1.DeploymentProgressing:
			if condition.Status == corev1.ConditionFalse {
				return utils.DeploymentInProgress, fmt.Errorf("deployment not progressing: %s - %s", condition.Reason, condition.Message)
			}
		case appsv1.DeploymentReplicaFailure:
			if condition.Status == corev1.ConditionTrue {
				return "Fail", fmt.Errorf("deployment replica failure: %s - %s", condition.Reason, condition.Message)
			}
		}
	}

	return "", nil
}

// annotateExternalResources annotates all external resources (secrets and configmaps)
// that the operator watches for changes. This centralizes annotation logic between
// Phase 1 (resource reconciliation) and Phase 2 (deployment reconciliation).
func (r *OLSConfigReconciler) annotateExternalResources(ctx context.Context,
	cr *olsv1alpha1.OLSConfig) error {

	var errs []error

	// Annotate all external secrets
	err := utils.ForEachExternalSecret(cr, func(name string, source string) error {
		if err := r.annotateSecretIfNeeded(ctx, name, r.Options.Namespace); err != nil {
			r.Logger.Error(err, "Failed to annotate secret", "source", source, "secret", name)
			errs = append(errs, err)
		}
		return nil // Continue iteration even on error
	})
	if err != nil {
		errs = append(errs, err)
	}

	// Annotate all external configmaps
	err = utils.ForEachExternalConfigMap(cr, func(name string, source string) error {
		if err := r.annotateConfigMapIfNeeded(ctx, name, r.Options.Namespace); err != nil {
			r.Logger.Error(err, "Failed to annotate configmap", "source", source, "configmap", name)
			errs = append(errs, err)
		}
		return nil // Continue iteration even on error
	})
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to annotate %d external resources", len(errs))
	}
	return nil
}

// annotateSecretIfNeeded annotates a secret with the watcher annotation if it doesn't already have it.
// Returns nil if the secret doesn't exist (will be picked up on next reconciliation).
func (r *OLSConfigReconciler) annotateSecretIfNeeded(ctx context.Context, name, namespace string) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Resource will be picked up on next reconciliation
		}
		return err
	}

	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}

	if _, exists := secret.Annotations[utils.WatcherAnnotationKey]; exists {
		return nil // Already annotated
	}

	secret.Annotations[utils.WatcherAnnotationKey] = utils.OLSConfigName
	return r.Update(ctx, secret)
}

// annotateConfigMapIfNeeded annotates a configmap with the watcher annotation if it doesn't already have it.
// Returns nil if the configmap doesn't exist (will be picked up on next reconciliation).
func (r *OLSConfigReconciler) annotateConfigMapIfNeeded(ctx context.Context, name, namespace string) error {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Resource will be picked up on next reconciliation
		}
		return err
	}

	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}

	if _, exists := cm.Annotations[utils.WatcherAnnotationKey]; exists {
		return nil // Already annotated
	}

	cm.Annotations[utils.WatcherAnnotationKey] = utils.OLSConfigName
	return r.Update(ctx, cm)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OLSConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Logger = ctrl.Log.WithName("Reconciler")
	r.NextReconcileTime = time.Now()

	return ctrl.NewControllerManagedBy(mgr).
		For(&olsv1alpha1.OLSConfig{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(&corev1.Secret{},
			&watchers.SecretUpdateHandler{Reconciler: r},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					// For Create events, allow all secrets in our namespace
					// This handles recreated secrets that don't have annotations yet
					return e.Object.GetNamespace() == r.Options.Namespace
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// For Update events, use strict filtering
					return r.shouldWatchSecret(e.ObjectNew)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// Ignore delete events - nothing to reconcile when resource is gone
					return false
				},
			})).
		Watches(&corev1.ConfigMap{},
			&watchers.ConfigMapUpdateHandler{Reconciler: r},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					// For Create events, allow all configmaps in our namespace
					// This handles recreated configmaps that don't have annotations yet
					return e.Object.GetNamespace() == r.Options.Namespace
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// For Update events, use strict filtering
					return r.shouldWatchConfigMap(e.ObjectNew)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// Ignore delete events - nothing to reconcile when resource is gone
					return false
				},
			})).
		Owns(&consolev1.ConsolePlugin{}).
		Owns(&monv1.ServiceMonitor{}).
		Owns(&monv1.PrometheusRule{}).
		Complete(r)
}

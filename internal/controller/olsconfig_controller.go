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

package controller

import (
	"context"
	"fmt"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// Definitions to manage status conditions
const (
	typeApiReady           = "ApiReady"
	typeCacheReady         = "CacheReady"
	typeConsolePluginReady = "ConsolePluginReady"
	typeCRReconciled       = "Reconciled"
)

// OLSConfigReconciler reconciles a OLSConfig object
type OLSConfigReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	logger            logr.Logger
	stateCache        map[string]string
	Options           OLSConfigReconcilerOptions
	NextReconcileTime time.Time
}

type OLSConfigReconcilerOptions struct {
	LightspeedServiceImage         string
	LightspeedServicePostgresImage string
	ConsoleUIImage                 string
	OpenShiftMCPServerImage        string
	Namespace                      string
	ReconcileInterval              time.Duration
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
	/*
		err := r.reconcileServiceMonitorForOperator(ctx)
		if err != nil {
			r.logger.Error(err, "Failed to reconcile service monitor for operator")
			return ctrl.Result{}, err
		}
	*/
	err := r.reconcileNetworkPolicyForOperator(ctx)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile network policy for operator")
		return ctrl.Result{}, err
	}
	// The operator reconciles only for OLSConfig CR with a specific name
	if req.NamespacedName.Name != OLSConfigName {
		r.logger.Info(fmt.Sprintf("Ignoring OLSConfig CR other than %s", OLSConfigName), "name", req.NamespacedName.Name)
		return ctrl.Result{}, nil
	}

	olsconfig := &olsv1alpha1.OLSConfig{}
	err = r.Get(ctx, req.NamespacedName, olsconfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Info("olsconfig resource not found. Ignoring since object must be deleted")
			_, err = r.removeConsoleUI(ctx)
			if err != nil {
				r.logger.Error(err, "Failed to remove console UI")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.logger.Error(err, "Failed to get olsconfig")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, err
	}
	r.logger.Info("reconciliation starts", "olsconfig generation", olsconfig.Generation)
	message, err := r.reconcileConsoleUI(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile console UI")
		r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, false, message, err)
		if message != "In Progress" {
			return ctrl.Result{RequeueAfter: r.Options.ReconcileInterval}, nil
		}
	}
	// Update status condition for Console Plugin
	r.updateStatusCondition(ctx, olsconfig, typeConsolePluginReady, true, "All components are successfully deployed", nil)

	message, err = r.reconcilePostgresServer(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile ols postgres")
		r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, false, message, nil)
		if message == "In Progress" {
			return ctrl.Result{RequeueAfter: 1 * time.Second}, err
		}
	}
	// Update status condition for Postgres cache
	r.updateStatusCondition(ctx, olsconfig, typeCacheReady, true, "All components are successfully deployed", nil)

	_, err = r.reconcileLLMSecrets(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile LLM Provider Secrets")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, err
	}

	message, err = r.reconcileAppServer(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile application server")
		r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, false, message, err)
		if message != "In Progress" {
			return ctrl.Result{RequeueAfter: r.Options.ReconcileInterval}, nil
		}
	}
	// Update status condition for API server
	r.updateStatusCondition(ctx, olsconfig, typeApiReady, true, "All components are successfully deployed", nil)

	r.logger.Info("reconciliation done", "olsconfig generation", olsconfig.Generation)

	// Update status condition for Custom Resource
	r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, true, "Custom resource successfully reconciled", nil)

	// Requeue if no reconciliation is scheduled in future.
	if r.NextReconcileTime.After(time.Now()) {
		return ctrl.Result{}, nil
	}
	r.NextReconcileTime = time.Now().Add(r.Options.ReconcileInterval)
	r.logger.Info("Next automatic reconciliation scheduled at", "nextReconcileTime", r.NextReconcileTime)
	return ctrl.Result{RequeueAfter: r.Options.ReconcileInterval}, nil
}

// updateStatusCondition updates the status condition of the OLSConfig Custom Resource instance.
// TODO: Should we support Unknown status and ObservedGeneration?
// TODO: conditionType must be metav1.Condition?
func (r *OLSConfigReconciler) updateStatusCondition(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig, conditionType string, status bool, message string, err error) {
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

	meta.SetStatusCondition(&olsconfig.Status.Conditions, condition)

	if updateErr := r.Status().Update(ctx, olsconfig); updateErr != nil {
		r.logger.Error(updateErr, ErrUpdateCRStatusCondition)
	}
}

// checkDeploymentStatus checks if the deployment is ready and available
func (r *OLSConfigReconciler) checkDeploymentStatus(deployment *appsv1.Deployment) (string, error) {
	// Check if deployment has the expected number of replicas ready
	if deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
		return "In Progress", fmt.Errorf("deployment not ready: %d replicas available",
			deployment.Status.ReadyReplicas)
	}

	// Check deployment conditions
	for _, condition := range deployment.Status.Conditions {
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			if condition.Status != corev1.ConditionTrue {
				return "In Progress", fmt.Errorf("deployment not available: %s - %s", condition.Reason, condition.Message)
			}
		case appsv1.DeploymentProgressing:
			if condition.Status == corev1.ConditionFalse {
				return "In Progress", fmt.Errorf("deployment not progressing: %s - %s", condition.Reason, condition.Message)
			}
		case appsv1.DeploymentReplicaFailure:
			if condition.Status == corev1.ConditionTrue {
				return "Fail", fmt.Errorf("deployment replica failure: %s - %s", condition.Reason, condition.Message)
			}
		}
	}

	return "", nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OLSConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = ctrl.Log.WithName("Reconciler")
	r.stateCache = make(map[string]string)
	r.NextReconcileTime = time.Now()

	generationChanged := builder.WithPredicates(predicate.GenerationChangedPredicate{})
	return ctrl.NewControllerManagedBy(mgr).
		For(&olsv1alpha1.OLSConfig{}).
		Owns(&appsv1.Deployment{}, generationChanged).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(secretWatcherFilter)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(telemetryPullSecretWatcherFilter)).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.configMapWatcherFilter)).
		Owns(&consolev1.ConsolePlugin{}).
		Owns(&monv1.ServiceMonitor{}).
		Owns(&monv1.PrometheusRule{}).
		Complete(r)
}

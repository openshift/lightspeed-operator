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
// (appserver/lcore, postgres, console) and manages the OLSConfig custom resource.
//
// The controller code is organized into multiple files:
//   - olsconfig_controller.go: Core type definition, Reconcile(), and SetupWithManager()
//   - olsconfig_helpers.go: Interface implementations, status management, and annotation logic
//   - olsconfig_watchers.go: Watcher predicate helpers for secrets and configmaps
//   - operator_assets.go: Operator infrastructure resources (ServiceMonitor, NetworkPolicy)
//
// Key Responsibilities:
//   - Reconcile the OLSConfig custom resource
//   - Coordinate component reconciliation (console, postgres, appserver/lcore)
//   - Manage status conditions and CR status updates
//   - Set up resource watchers for automatic updates (secrets, configmaps)
//   - Manage operator-level resources (service monitors, network policies)
//
// The main reconciliation flow:
//  1. Reconcile operator-level resources (service monitor, network policy)
//  2. Handle finalizer logic (cleanup on deletion, add on creation)
//  3. Fetch and validate OLSConfig CR
//  4. Annotate external resources for change tracking
//  5. Phase 1: Reconcile independent resources (ConfigMaps, Secrets, ServiceAccounts, etc.)
//  6. Phase 2: Reconcile deployments and dependent resources (Services, TLS certs, etc.)
//  7. Update status conditions based on deployment readiness
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
	imagev1 "github.com/openshift/api/image/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

// OLSConfigReconciler reconciles a OLSConfig object.
// This controller is fully event-driven and does not use periodic reconciliation.
// All changes are detected via watches:
//   - Owned resources (Deployments, Services, etc.) via Owns()
//   - External resources (Secrets, ConfigMaps) via Watches() with custom predicates
//
// Controller-runtime handles error retries with exponential backoff.
type OLSConfigReconciler struct {
	client.Client
	Logger        logr.Logger
	Options       utils.OLSConfigReconcilerOptions
	WatcherConfig *utils.WatcherConfig
}

// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/finalizers,verbs=update
// RBAC for managing deployments of OLS application server
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// RBAC for reading pod status for diagnostics
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
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
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;create;update;patch;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace=openshift-lightspeed,resources=roles;rolebindings,verbs=*
// NonResourceURLs for Lightspeed access control and metrics
// +kubebuilder:rbac:urls=/ls-access,verbs=get
// +kubebuilder:rbac:urls=/ols-metrics-access,verbs=get

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

// ImageStream access
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams,verbs=get;list;watch;create;update;patch;delete

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *OLSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Reconcile operator's resources first
	// The operator reconciles only for OLSConfig CR with a specific name
	if req.Name != utils.OLSConfigName {
		r.Logger.Info(fmt.Sprintf("Ignoring OLSConfig CR other than %s", utils.OLSConfigName), "name", req.Name)
		return ctrl.Result{}, nil
	}

	// Fetch the OLSConfig CR first to determine if it exists
	// This prevents unnecessary operator-level reconciliation after CR deletion
	olsconfig := &olsv1alpha1.OLSConfig{}
	err := r.Get(ctx, req.NamespacedName, olsconfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// CR was deleted - this is expected after finalizer completes
			// Return silently without logging to avoid noise from watch-triggered reconciliations
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		// Controller-runtime handles error retries with exponential backoff.
		r.Logger.Error(err, "Failed to get olsconfig")
		return ctrl.Result{}, err
	}

	// ========== Finalizer Handling ==========
	// Check if CR is being deleted (DeletionTimestamp is set)
	// Handle this BEFORE operator reconciliation to avoid wasteful work during deletion
	if !olsconfig.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(olsconfig, utils.OLSConfigFinalizer) {
			r.Logger.Info("OLSConfig CR is being deleted, running finalizer cleanup")

			// Run finalizer cleanup logic
			if err := r.finalizeOLSConfig(ctx, olsconfig); err != nil {
				r.Logger.Error(err, "Failed to finalize OLSConfig CR")
				return ctrl.Result{}, err
			}

			// Re-fetch the CR to get the latest ResourceVersion before removing finalizer
			// This prevents conflict errors if the CR was updated during cleanup
			r.Logger.Info("Removing finalizer from OLSConfig CR")
			if err := r.Get(ctx, req.NamespacedName, olsconfig); err != nil {
				if apierrors.IsNotFound(err) {
					// CR already deleted, nothing to do
					return ctrl.Result{}, nil
				}
				r.Logger.Error(err, "Failed to re-fetch OLSConfig CR before finalizer removal")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(olsconfig, utils.OLSConfigFinalizer)
			if err := r.Update(ctx, olsconfig); err != nil {
				if apierrors.IsNotFound(err) {
					// CR was deleted between Get and Update, that's fine
					r.Logger.V(1).Info("OLSConfig CR deleted during finalizer removal, skipping")
					return ctrl.Result{}, nil
				}
				if apierrors.IsConflict(err) {
					// Conflict means CR was updated by someone else, controller-runtime will retry
					r.Logger.V(1).Info("Conflict removing finalizer, will retry")
					return ctrl.Result{}, err
				}
				r.Logger.Error(err, "Failed to remove finalizer from OLSConfig CR")
				return ctrl.Result{}, err
			}
		}
		// CR is being deleted and finalizer is removed (or never existed), nothing to do
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present (for new or existing CRs without finalizer)
	if !controllerutil.ContainsFinalizer(olsconfig, utils.OLSConfigFinalizer) {
		r.Logger.Info("Adding finalizer to OLSConfig CR")
		controllerutil.AddFinalizer(olsconfig, utils.OLSConfigFinalizer)
		if err := r.Update(ctx, olsconfig); err != nil {
			r.Logger.Error(err, "Failed to add finalizer to OLSConfig CR")
			return ctrl.Result{}, err
		}
		r.Logger.Info("Finalizer added to OLSConfig CR")
		// Return here to ensure finalizer is persisted before proceeding
		// Controller-runtime will requeue automatically
		return ctrl.Result{}, nil
	}
	// ========== End Finalizer Handling ==========

	// Reconcile operator-level resources (ServiceMonitor, NetworkPolicy)
	// These are reconciled on every CR reconciliation to ensure they stay in sync
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

	r.Logger.Info("reconciliation starts", "olsconfig generation", olsconfig.Generation)

	// Annotation Step: Annotate external resources and validate they exist
	// This ensures all user-provided external resources (secrets, configmaps) are properly
	// annotated for watching before we reconcile deployments that depend on them.
	// This also validates that the resources exist (fail fast if they're missing).
	if err := r.annotateExternalResources(ctx, olsconfig); err != nil {
		// Controller-runtime handles error retries with exponential backoff.
		r.Logger.Error(err, "Failed to annotate external resources")
		return ctrl.Result{}, err
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
		// Update status to show resource reconciliation failures
		failureStatus := olsv1alpha1.OLSConfigStatus{
			Conditions:     []metav1.Condition{},
			OverallStatus:  olsv1alpha1.OverallStatusNotReady,
			DiagnosticInfo: []olsv1alpha1.PodDiagnostic{},
		}

		// Add a condition for each failed resource
		for taskName, err := range resourceFailures {
			condition := metav1.Condition{
				Type:               "ResourceReconciliation",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: olsconfig.Generation,
				Reason:             "Failed",
				Message:            fmt.Sprintf("Failed to reconcile %s: %v", taskName, err),
				LastTransitionTime: metav1.Now(),
			}
			failureStatus.Conditions = append(failureStatus.Conditions, condition)
		}

		// Update status before returning error
		if updateErr := r.UpdateStatusCondition(ctx, olsconfig, failureStatus); updateErr != nil {
			r.Logger.Error(updateErr, "Failed to update status after resource reconciliation failure")
		}

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
	// Create status structure to populate as we check each deployment
	failedTasks := make(map[string]error)
	newStatus := olsv1alpha1.OLSConfigStatus{
		Conditions:     []metav1.Condition{},
		OverallStatus:  olsv1alpha1.OverallStatusReady,
		DiagnosticInfo: []olsv1alpha1.PodDiagnostic{},
	}

	for _, step := range deploymentSteps {
		err := step.Fn(ctx, olsconfig)
		if err != nil {
			r.Logger.Error(err, fmt.Sprintf("Failed to reconcile %s", step.Name))
			// Add condition to status structure
			condition := metav1.Condition{
				Type:               step.ConditionType,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: olsconfig.Generation,
				Reason:             "Failed",
				Message:            fmt.Sprintf("Failed: %v", err),
				LastTransitionTime: metav1.Now(),
			}
			newStatus.Conditions = append(newStatus.Conditions, condition)
			failedTasks[step.Name] = err
			newStatus.OverallStatus = olsv1alpha1.OverallStatusNotReady
		} else {
			// Get corresponding deployment
			deployment := &appsv1.Deployment{}
			err := r.Get(ctx, client.ObjectKey{Name: step.Deployment, Namespace: r.Options.Namespace}, deployment)
			if err != nil {
				// Add condition to status structure
				condition := metav1.Condition{
					Type:               step.ConditionType,
					Status:             metav1.ConditionFalse,
					ObservedGeneration: olsconfig.Generation,
					Reason:             "Failed",
					Message:            fmt.Sprintf("Failed: %v", err),
					LastTransitionTime: metav1.Now(),
				}
				newStatus.Conditions = append(newStatus.Conditions, condition)
				failedTasks[step.Name] = err
				newStatus.OverallStatus = olsv1alpha1.OverallStatusNotReady
			} else {
				status, diagnostics, err := r.checkDeploymentStatus(ctx, deployment, step.ConditionType)
				// Append diagnostics from this deployment (will be empty for Ready/Progressing)
				newStatus.DiagnosticInfo = append(newStatus.DiagnosticInfo, diagnostics...)

				switch status {
				case string(olsv1alpha1.DeploymentStatusReady):
					// Deployment is ready
					condition := metav1.Condition{
						Type:               step.ConditionType,
						Status:             metav1.ConditionTrue,
						ObservedGeneration: olsconfig.Generation,
						Reason:             "Available",
						Message:            "Ready",
						LastTransitionTime: metav1.Now(),
					}
					newStatus.Conditions = append(newStatus.Conditions, condition)
				case string(olsv1alpha1.DeploymentStatusProgressing):
					// Deployment is not ready yet - status will be updated on next reconciliation
					// triggered by deployment watch
					condition := metav1.Condition{
						Type:               step.ConditionType,
						Status:             metav1.ConditionFalse,
						ObservedGeneration: olsconfig.Generation,
						Reason:             "Progressing",
						Message:            status,
						LastTransitionTime: metav1.Now(),
					}
					newStatus.Conditions = append(newStatus.Conditions, condition)
					newStatus.OverallStatus = olsv1alpha1.OverallStatusNotReady
				default:
					// Deployment failed
					condition := metav1.Condition{
						Type:               step.ConditionType,
						Status:             metav1.ConditionFalse,
						ObservedGeneration: olsconfig.Generation,
						Reason:             "Failed",
						Message:            fmt.Sprintf("Failed: %v", err),
						LastTransitionTime: metav1.Now(),
					}
					newStatus.Conditions = append(newStatus.Conditions, condition)
					failedTasks[step.Name] = err
					newStatus.OverallStatus = olsv1alpha1.OverallStatusNotReady
				}
			}
		}
	}

	// Update status once, regardless of outcome (with retry on conflict)
	if updateErr := r.UpdateStatusCondition(ctx, olsconfig, newStatus); updateErr != nil {
		r.Logger.Error(updateErr, "Failed to update status")
		return ctrl.Result{}, updateErr
	}

	// Determine reconciliation result based on deployment status
	var reconcileErr error

	// If we have diagnostics, return error to trigger exponential backoff retry
	if len(newStatus.DiagnosticInfo) > 0 {
		r.Logger.Info("deployment has pod failures, will retry with exponential backoff",
			"diagnosticCount", len(newStatus.DiagnosticInfo))
		reconcileErr = fmt.Errorf("deployment has failing pods (count: %d), see status.diagnosticInfo for details",
			len(newStatus.DiagnosticInfo))
	} else if newStatus.OverallStatus == olsv1alpha1.OverallStatusReady {
		r.Logger.Info("reconciliation done", "olsconfig generation", olsconfig.Generation)
	} else {
		// Deployments are progressing - keep checking to detect issues early
		r.Logger.Info("reconciliation in progress, waiting for deployments to become ready")
		reconcileErr = fmt.Errorf("deployments not ready yet, retrying")
	}

	return ctrl.Result{}, reconcileErr
}

// finalizeOLSConfig performs cleanup when OLSConfig CR is deleted.
// It ensures all resources are properly cleaned up before removing the finalizer.
func (r *OLSConfigReconciler) finalizeOLSConfig(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	r.Logger.Info("Starting OLSConfig finalization")

	// Step 1: Remove Console UI (deactivate plugin, delete ConsolePlugin CR)
	// This must be done manually because ConsolePlugin is cluster-scoped and
	// needs to be removed from the Console CR's plugin list
	r.Logger.V(1).Info("Removing Console UI during finalization")
	if err := console.RemoveConsoleUI(r, ctx); err != nil {
		r.Logger.Error(err, "Failed to remove Console UI during finalization")
		// Log the error but don't block finalization
		// We still want the finalizer to complete to avoid CR being stuck in Terminating
		// The Console removal functions handle NotFound errors gracefully
		r.Logger.V(1).Info("Proceeding with finalization despite Console UI removal error")
	}

	// Step 2: List all owned resources once (avoids duplicate API calls)
	r.Logger.V(1).Info("Listing owned resources for cleanup")
	resourceGroups, err := r.listOwnedResources(ctx, cr)
	if err != nil {
		r.Logger.Error(err, "Failed to list owned resources")
		return fmt.Errorf("failed to list owned resources: %w", err)
	}

	// Step 3: Explicitly delete owned resources
	// With blockOwnerDeletion=true, cascade deletion won't start until we delete children
	totalResources := 0
	resourceCounts := make(map[string]int)
	for _, group := range resourceGroups {
		count := len(group.Items)
		totalResources += count
		resourceCounts[group.Type] = count
	}

	r.Logger.Info("Deleting owned resources", "total", totalResources, "counts", resourceCounts)
	if err := r.deleteOwnedResources(ctx, resourceGroups); err != nil {
		r.Logger.Error(err, "Error deleting owned resources")
		// Continue anyway - wait logic will handle remaining resources
	}

	// Step 4: Wait for owned resources to be deleted
	// This prevents race conditions when recreating OLSConfig CR quickly
	r.Logger.V(1).Info("Waiting for owned resources to be deleted")
	if err := r.waitForOwnedResourcesDeletion(ctx, cr); err != nil {
		r.Logger.Error(err, "Timeout or error waiting for owned resources deletion")
		// Don't return error here - we want to remove the finalizer anyway after timeout
		// This prevents the CR from being stuck in Terminating state forever
		r.Logger.V(1).Info("Proceeding with finalizer removal despite cleanup timeout")
	}

	r.Logger.Info("OLSConfig finalization completed successfully")
	return nil
}

// ResourceGroup holds a collection of Kubernetes resources of the same type
type ResourceGroup struct {
	Type  string
	Items []client.Object
}

// listOwnedResources returns all resources owned by the OLSConfig CR grouped by type.
// Uses owner references for reliable filtering - more trustworthy than labels.
func (r *OLSConfigReconciler) listOwnedResources(ctx context.Context, cr *olsv1alpha1.OLSConfig) ([]ResourceGroup, error) {
	var groups []ResourceGroup

	// List all Deployments in namespace
	deploymentList := &appsv1.DeploymentList{}
	if err := r.List(ctx, deploymentList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	var deploymentObjs []client.Object
	for i := range deploymentList.Items {
		if isOwnedBy(&deploymentList.Items[i], cr) {
			deploymentObjs = append(deploymentObjs, &deploymentList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "deployment", Items: deploymentObjs})

	// List all PVCs in namespace
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list pvcs: %w", err)
	}
	var pvcObjs []client.Object
	for i := range pvcList.Items {
		if isOwnedBy(&pvcList.Items[i], cr) {
			pvcObjs = append(pvcObjs, &pvcList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "pvc", Items: pvcObjs})

	// List all Services in namespace
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	var serviceObjs []client.Object
	for i := range serviceList.Items {
		if isOwnedBy(&serviceList.Items[i], cr) {
			serviceObjs = append(serviceObjs, &serviceList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "service", Items: serviceObjs})

	// List all ConfigMaps in namespace
	configMapList := &corev1.ConfigMapList{}
	if err := r.List(ctx, configMapList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list configmaps: %w", err)
	}
	var configMapObjs []client.Object
	for i := range configMapList.Items {
		if isOwnedBy(&configMapList.Items[i], cr) {
			configMapObjs = append(configMapObjs, &configMapList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "configmap", Items: configMapObjs})

	// List all Secrets in namespace
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	var secretObjs []client.Object
	for i := range secretList.Items {
		if isOwnedBy(&secretList.Items[i], cr) {
			secretObjs = append(secretObjs, &secretList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "secret", Items: secretObjs})

	// List all ServiceAccounts in namespace
	serviceAccountList := &corev1.ServiceAccountList{}
	if err := r.List(ctx, serviceAccountList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list serviceaccounts: %w", err)
	}
	var serviceAccountObjs []client.Object
	for i := range serviceAccountList.Items {
		if isOwnedBy(&serviceAccountList.Items[i], cr) {
			serviceAccountObjs = append(serviceAccountObjs, &serviceAccountList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "serviceaccount", Items: serviceAccountObjs})

	// List all NetworkPolicies in namespace
	networkPolicyList := &networkingv1.NetworkPolicyList{}
	if err := r.List(ctx, networkPolicyList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list networkpolicies: %w", err)
	}
	var networkPolicyObjs []client.Object
	for i := range networkPolicyList.Items {
		if isOwnedBy(&networkPolicyList.Items[i], cr) {
			networkPolicyObjs = append(networkPolicyObjs, &networkPolicyList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "networkpolicy", Items: networkPolicyObjs})

	// List all Roles in namespace
	roleList := &rbacv1.RoleList{}
	if err := r.List(ctx, roleList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	var roleObjs []client.Object
	for i := range roleList.Items {
		if isOwnedBy(&roleList.Items[i], cr) {
			roleObjs = append(roleObjs, &roleList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "role", Items: roleObjs})

	// List all RoleBindings in namespace
	roleBindingList := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, roleBindingList, client.InNamespace(r.Options.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list rolebindings: %w", err)
	}
	var roleBindingObjs []client.Object
	for i := range roleBindingList.Items {
		if isOwnedBy(&roleBindingList.Items[i], cr) {
			roleBindingObjs = append(roleBindingObjs, &roleBindingList.Items[i])
		}
	}
	groups = append(groups, ResourceGroup{Type: "rolebinding", Items: roleBindingObjs})

	// List all ServiceMonitors in namespace (if Prometheus Operator is installed)
	serviceMonitorList := &monitoringv1.ServiceMonitorList{}
	if err := r.List(ctx, serviceMonitorList, client.InNamespace(r.Options.Namespace)); err != nil {
		// ServiceMonitor CRD might not be installed, ignore error
		r.Logger.V(1).Info("Could not list ServiceMonitors, Prometheus Operator may not be installed", "error", err)
	} else {
		var serviceMonitorObjs []client.Object
		for i := range serviceMonitorList.Items {
			if isOwnedBy(&serviceMonitorList.Items[i], cr) {
				serviceMonitorObjs = append(serviceMonitorObjs, &serviceMonitorList.Items[i])
			}
		}
		groups = append(groups, ResourceGroup{Type: "servicemonitor", Items: serviceMonitorObjs})
	}

	// List all PrometheusRules in namespace (if Prometheus Operator is installed)
	prometheusRuleList := &monitoringv1.PrometheusRuleList{}
	if err := r.List(ctx, prometheusRuleList, client.InNamespace(r.Options.Namespace)); err != nil {
		// PrometheusRule CRD might not be installed, ignore error
		r.Logger.V(1).Info("Could not list PrometheusRules, Prometheus Operator may not be installed", "error", err)
	} else {
		var prometheusRuleObjs []client.Object
		for i := range prometheusRuleList.Items {
			if isOwnedBy(&prometheusRuleList.Items[i], cr) {
				prometheusRuleObjs = append(prometheusRuleObjs, &prometheusRuleList.Items[i])
			}
		}
		groups = append(groups, ResourceGroup{Type: "prometheusrule", Items: prometheusRuleObjs})
	}

	return groups, nil
}

// isOwnedBy checks if a resource is owned by the given OLSConfig CR.
func isOwnedBy(obj client.Object, cr *olsv1alpha1.OLSConfig) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == cr.UID {
			return true
		}
	}
	return false
}

// deleteOwnedResources explicitly deletes the provided resources.
// This triggers cleanup that would otherwise be blocked by blockOwnerDeletion=true.
// Accepts pre-fetched resource groups to avoid duplicate API calls.
func (r *OLSConfigReconciler) deleteOwnedResources(ctx context.Context, resourceGroups []ResourceGroup) error {
	deletedResources := []string{}

	// Delete all resources from all groups
	for _, group := range resourceGroups {
		for _, obj := range group.Items {
			if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
				r.Logger.Error(err, "Failed to delete "+group.Type, group.Type, obj.GetName())
			} else {
				deletedResources = append(deletedResources, group.Type+"/"+obj.GetName())
			}
		}
	}

	// Log all deleted resources in one line
	if len(deletedResources) > 0 {
		r.Logger.Info("Deleted resources", "count", len(deletedResources), "resources", deletedResources)
	}

	return nil
}

// waitForOwnedResourcesDeletion waits for all resources owned by the OLSConfig CR to be deleted.
// Uses listOwnedResources to dynamically check what still exists via owner references.
// Waits for all resource types for complete cleanup and to prevent race conditions.
func (r *OLSConfigReconciler) waitForOwnedResourcesDeletion(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	timeout := 3 * time.Minute
	interval := 5 * time.Second
	firstCheck := true

	return wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		resourceGroups, err := r.listOwnedResources(ctx, cr)
		if err != nil {
			r.Logger.Error(err, "Error listing resources during cleanup wait")
			// Continue polling despite error
			return false, nil
		}

		// Calculate total resources across all groups
		totalResources := 0
		for _, group := range resourceGroups {
			totalResources += len(group.Items)
		}

		if totalResources == 0 {
			r.Logger.Info("All owned resources have been deleted")
			return true, nil
		}

		// Only log if we're actually waiting (not on first check)
		if !firstCheck {
			// Build list of remaining resource counts for logging
			remaining := []string{}
			for _, group := range resourceGroups {
				if len(group.Items) > 0 {
					remaining = append(remaining, fmt.Sprintf("%d %s", len(group.Items), group.Type+"s"))
				}
			}

			r.Logger.V(1).Info("Waiting for resource deletion", "remaining", remaining, "total", totalResources)
		}
		firstCheck = false

		return false, nil // Not all deleted yet, keep polling
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *OLSConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Logger = ctrl.Log.WithName("Reconciler")

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
		Owns(&monitoringv1.ServiceMonitor{}).
		Owns(&monitoringv1.PrometheusRule{}).
		Owns(&imagev1.ImageStream{}).
		Complete(r)
}

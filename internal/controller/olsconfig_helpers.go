package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// This file contains support functions for OLSConfigReconciler:
//   - Interface implementations: Satisfy the reconciler.Reconciler interface, allowing
//     OLSConfigReconciler to be passed to component packages without circular dependencies
//   - Watcher predicates: Filter events at watch level for performance optimization
//   - Status management: Update OLSConfig CR status conditions and check deployment readiness
//   - External resource annotation: Mark user-provided secrets/configmaps for change tracking

// Interface implementation methods for reconciler.Reconciler
// These getters allow component packages (appserver, postgres, console, lcore) to access
// reconciler capabilities without importing the controller package, preventing circular dependencies.

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

// Status management

// UpdateStatusCondition updates the status condition of the OLSConfig Custom Resource instance.
// The condition's ObservedGeneration is set to the current CR generation to track which spec version
// the status reflects, following Kubernetes API conventions.
func (r *OLSConfigReconciler) UpdateStatusCondition(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig, conditionType string, status bool, message string, err error, inCluster ...bool) {
	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	condition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: olsconfig.Generation,
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

// External resource annotation

// annotateExternalResources annotates all external resources (secrets and configmaps)
// that the operator watches for changes. This centralizes annotation logic between
// Phase 1 (resource reconciliation) and Phase 2 (deployment reconciliation).
// It also validates LLM credentials before proceeding with annotation.
func (r *OLSConfigReconciler) annotateExternalResources(ctx context.Context,
	cr *olsv1alpha1.OLSConfig) error {

	// Validate LLM credentials first (fail fast)
	if err := utils.ValidateLLMCredentials(r, ctx, cr); err != nil {
		return fmt.Errorf("LLM credentials validation failed: %w", err)
	}

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

// Watcher predicate helpers

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

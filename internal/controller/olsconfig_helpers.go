package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (r *OLSConfigReconciler) GetOcpRagImage() string {
	return r.Options.OcpRagImage
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

// UpdateStatusCondition updates the complete status of the OLSConfig Custom Resource instance.
// Uses retry with conflict handling to ensure the update succeeds even under concurrent modifications.
func (r *OLSConfigReconciler) UpdateStatusCondition(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig, newStatus olsv1alpha1.OLSConfigStatus, inCluster ...bool) error {
	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	// Helper to preserve LastTransitionTime for unchanged conditions
	preserveTransitionTimes := func(newStatus *olsv1alpha1.OLSConfigStatus, existingConditions []metav1.Condition) {
		for i := range newStatus.Conditions {
			for _, existing := range existingConditions {
				if newStatus.Conditions[i].Type == existing.Type &&
					newStatus.Conditions[i].Status == existing.Status &&
					newStatus.Conditions[i].Reason == existing.Reason {
					// Condition hasn't changed, preserve the transition time
					newStatus.Conditions[i].LastTransitionTime = existing.LastTransitionTime
					break
				}
			}
		}
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

			// Preserve LastTransitionTime for conditions that haven't changed
			preserveTransitionTimes(&newStatus, currentOLSConfig.Status.Conditions)

			// Apply the new status to the current version
			currentOLSConfig.Status = newStatus

			// Attempt status update
			return r.Status().Update(ctx, currentOLSConfig)
		}); updateErr != nil {
			if !apierrors.IsNotFound(updateErr) {
				r.Logger.Error(updateErr, utils.ErrUpdateCRStatusCondition, "name", olsconfig.Name)
			}
			return updateErr
		}
		return nil
	} else {
		// Apply the new status directly (for tests)
		// Preserve LastTransitionTime for conditions that haven't changed
		preserveTransitionTimes(&newStatus, olsconfig.Status.Conditions)

		olsconfig.Status = newStatus
		if updateErr := r.Status().Update(ctx, olsconfig); updateErr != nil {
			r.Logger.Error(updateErr, utils.ErrUpdateCRStatusCondition)
			return updateErr
		}
		return nil
	}
}

// checkDeploymentStatus checks if the deployment is ready and collects diagnostics on failure.
// Returns the status (Ready/Progressing/Failed), diagnostics array, and error.
func (r *OLSConfigReconciler) checkDeploymentStatus(
	ctx context.Context,
	deployment *appsv1.Deployment,
	conditionType string,
) (string, []olsv1alpha1.PodDiagnostic, error) {

	// Check if deployment is Available
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return string(olsv1alpha1.DeploymentStatusReady), nil, nil
		}
	}

	// Deployment is not Available - check pod status for diagnostics
	diagnostics := r.collectDeploymentDiagnostics(ctx, deployment, conditionType)

	// If we found pod problems, check if they're terminal failures
	if len(diagnostics) > 0 {
		for _, diag := range diagnostics {
			// These are terminal/recurring failures - mark as Failed
			if diag.Reason == "CrashLoopBackOff" ||
				diag.Reason == "ImagePullBackOff" ||
				diag.Reason == "ErrImagePull" ||
				diag.Reason == "OOMKilled" ||
				strings.HasPrefix(diag.Reason, "PreviousCrash:") {
				return string(olsv1alpha1.DeploymentStatusFailed), diagnostics,
					fmt.Errorf("deployment has failing pods: %s", diag.Reason)
			}
		}
	}

	// No terminal failures, still progressing
	return string(olsv1alpha1.DeploymentStatusProgressing), diagnostics, nil
}

// collectDeploymentDiagnostics collects pod-level diagnostics for a failed deployment.
// Returns a slice of PodDiagnostic entries describing what went wrong with each pod.
func (r *OLSConfigReconciler) collectDeploymentDiagnostics(
	ctx context.Context,
	deployment *appsv1.Deployment,
	conditionType string,
) []olsv1alpha1.PodDiagnostic {

	// Return empty diagnostics if selector is not set (shouldn't happen in real deployments)
	if deployment.Spec.Selector == nil || len(deployment.Spec.Selector.MatchLabels) == 0 {
		return nil
	}

	pods := &corev1.PodList{}
	err := r.List(ctx, pods,
		client.InNamespace(deployment.Namespace),
		client.MatchingLabels(deployment.Spec.Selector.MatchLabels))

	if err != nil {
		r.Logger.Error(err, "failed to list pods for diagnostics",
			"deployment", deployment.Name)
		return nil
	}

	var diagnostics []olsv1alpha1.PodDiagnostic
	now := metav1.Now()
	// Track pods that have container-level diagnostics to avoid redundant PodCondition entries
	podsWithContainerDiags := make(map[string]bool)

	for _, pod := range pods.Items {
		// Check container statuses for issues
		// Note: Don't skip Running pods - they can have containers in CrashLoopBackOff
		for _, containerStatus := range pod.Status.ContainerStatuses {
			// Skip containers that are running and ready - they're healthy
			if containerStatus.State.Running != nil && containerStatus.Ready {
				continue
			}

			// Waiting state (ImagePullBackOff, ContainerCreating, etc.)
			if containerStatus.State.Waiting != nil {
				waiting := containerStatus.State.Waiting
				diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
					FailedComponent: conditionType,
					PodName:         pod.Name,
					ContainerName:   containerStatus.Name,
					Reason:          waiting.Reason,
					Message:         messageOrDefault(waiting.Message, "Container waiting - check pod status for details"),
					Type:            olsv1alpha1.DiagnosticTypeContainerWaiting,
					LastUpdated:     now,
				})
				podsWithContainerDiags[pod.Name] = true
			}

			// Terminated state with non-zero exit code
			if containerStatus.State.Terminated != nil &&
				containerStatus.State.Terminated.ExitCode != 0 {
				term := containerStatus.State.Terminated
				exitCode := term.ExitCode
				diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
					FailedComponent: conditionType,
					PodName:         pod.Name,
					ContainerName:   containerStatus.Name,
					Reason:          term.Reason,
					Message:         messageOrDefault(term.Message, "Container terminated - check pod logs for details"),
					ExitCode:        &exitCode,
					Type:            olsv1alpha1.DiagnosticTypeContainerTerminated,
					LastUpdated:     now,
				})
				podsWithContainerDiags[pod.Name] = true
			}

			// Last termination state (for CrashLoopBackOff context)
			// Only collect this if current state is Waiting (not Terminated) to avoid duplicate diagnostics
			if containerStatus.State.Waiting != nil &&
				containerStatus.LastTerminationState.Terminated != nil {
				term := containerStatus.LastTerminationState.Terminated
				exitCode := term.ExitCode
				diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
					FailedComponent: conditionType,
					PodName:         pod.Name,
					ContainerName:   containerStatus.Name,
					Reason:          fmt.Sprintf("PreviousCrash: %s", term.Reason),
					Message:         messageOrDefault(term.Message, "Previous container crash - check pod logs for details"),
					ExitCode:        &exitCode,
					Type:            olsv1alpha1.DiagnosticTypeContainerTerminated,
					LastUpdated:     now,
				})
				podsWithContainerDiags[pod.Name] = true
			}
		}

		// Check init container statuses
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.State.Waiting != nil {
				waiting := containerStatus.State.Waiting
				diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
					FailedComponent: conditionType,
					PodName:         pod.Name,
					ContainerName:   fmt.Sprintf("init/%s", containerStatus.Name),
					Reason:          waiting.Reason,
					Message:         messageOrDefault(waiting.Message, "Init container waiting - check pod status for details"),
					Type:            olsv1alpha1.DiagnosticTypeContainerWaiting,
					LastUpdated:     now,
				})
				podsWithContainerDiags[pod.Name] = true
			}
		}

		// Check pod conditions (scheduling, readiness issues)
		for _, condition := range pod.Status.Conditions {
			// Pod scheduling failures
			if condition.Type == corev1.PodScheduled &&
				condition.Status == corev1.ConditionFalse {
				diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
					FailedComponent: conditionType,
					PodName:         pod.Name,
					Reason:          condition.Reason,
					Message:         condition.Message,
					Type:            olsv1alpha1.DiagnosticTypePodScheduling,
					LastUpdated:     now,
				})
			}

			// Pod readiness failures (after being scheduled)
			// Only add this if we don't already have more specific container diagnostics
			if condition.Type == corev1.PodReady &&
				condition.Status == corev1.ConditionFalse &&
				pod.Status.Phase == corev1.PodRunning &&
				!podsWithContainerDiags[pod.Name] {
				diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
					FailedComponent: conditionType,
					PodName:         pod.Name,
					Reason:          condition.Reason,
					Message:         condition.Message,
					Type:            olsv1alpha1.DiagnosticTypePodCondition,
					LastUpdated:     now,
				})
			}
		}

		// Pod phase issues (non-Running, non-Pending, non-Succeeded)
		if pod.Status.Phase == corev1.PodFailed ||
			pod.Status.Phase == corev1.PodUnknown {
			diagnostics = append(diagnostics, olsv1alpha1.PodDiagnostic{
				FailedComponent: conditionType,
				PodName:         pod.Name,
				Reason:          string(pod.Status.Phase),
				Message:         pod.Status.Message,
				Type:            olsv1alpha1.DiagnosticTypePodCondition,
				LastUpdated:     now,
			})
		}
	}

	return diagnostics
}

// messageOrDefault returns the provided message if non-empty, otherwise returns the default message
func messageOrDefault(message, defaultMessage string) string {
	if message == "" {
		return defaultMessage
	}
	return message
}

// External resource annotation

// annotateExternalResources annotates all external resources (secrets and configmaps)
// that the operator watches for changes. This centralizes annotation logic between
// Phase 1 (resource reconciliation) and Phase 2 (deployment reconciliation).
// It validates external secrets (LLM credentials, TLS) before proceeding with annotation.
func (r *OLSConfigReconciler) annotateExternalResources(ctx context.Context,
	cr *olsv1alpha1.OLSConfig) error {

	// Validate external secrets first (fail fast)
	if err := utils.ValidateLLMCredentials(r, ctx, cr); err != nil {
		return fmt.Errorf("LLM credentials validation failed: %w", err)
	}

	// Validate TLS secret if custom TLS is configured
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		if err := utils.ValidateTLSSecret(r, ctx, cr); err != nil {
			return fmt.Errorf("TLS secret validation failed: %w", err)
		}
	}

	// Clear the mappings before repopulating them
	// This ensures that resources removed from CR are also removed from mappings
	if r.WatcherConfig != nil {
		r.WatcherConfig.AnnotatedConfigMapMapping = make(map[string][]string)
		r.WatcherConfig.AnnotatedSecretMapping = make(map[string][]string)
	}

	var errs []error

	// Annotate all external secrets
	err := utils.ForEachExternalSecret(cr, func(name string, source string) error {
		// TLS secrets affect both console (CA cert) and backend (server cert)
		// All other secrets use the default behavior (ACTIVE_BACKEND only)
		if r.WatcherConfig != nil && source == "tls" {
			r.WatcherConfig.AnnotatedSecretMapping[name] = []string{
				utils.ConsoleUIDeploymentName,
				"ACTIVE_BACKEND",
			}
		}

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
	// All external ConfigMaps use the default behavior (restart ACTIVE_BACKEND only)
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

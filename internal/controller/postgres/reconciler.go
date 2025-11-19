// Package postgres provides reconciliation logic for the PostgreSQL database component
// used by OpenShift Lightspeed for conversation cache storage.
//
// This package manages:
//   - PostgreSQL deployment and pod lifecycle
//   - Database initialization and bootstrap secrets
//   - PersistentVolumeClaim for data persistence
//   - Service configuration for database access
//   - ConfigMap for PostgreSQL configuration
//   - Network policies for database security
//   - CA certificate management for secure connections
//
// The PostgreSQL instance is used to cache conversation history and maintain
// session state for the OLS application server. The main entry point is
// ReconcilePostgres, which ensures all PostgreSQL resources are properly configured.
package postgres

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcilePostgresResources reconciles all resources except the deployment (Phase 1)
// Uses continue-on-error pattern since these resources are independent
func ReconcilePostgresResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcilePostgresResources starts")
	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile Postgres ConfigMap",
			Task: reconcilePostgresConfigMap,
		},
		{
			Name: "reconcile Postgres Bootstrap Secret",
			Task: reconcilePostgresBootstrapSecret,
		},
		{
			Name: "reconcile Postgres Secret",
			Task: reconcilePostgresSecret,
		},
		{
			Name: "generate Postgres Network Policy",
			Task: reconcilePostgresNetworkPolicy,
		},
	}

	failedTasks := make(map[string]error)

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcilePostgresResources error", "task", task.Name)
			failedTasks[task.Name] = err
		}
	}

	if len(failedTasks) > 0 {
		taskNames := make([]string, 0, len(failedTasks))
		for taskName, err := range failedTasks {
			taskNames = append(taskNames, taskName)
			r.GetLogger().Error(err, "Task failed in reconcilePostgresResources", "task", taskName)
		}
		return fmt.Errorf("failed tasks: %v", taskNames)
	}

	r.GetLogger().Info("reconcilePostgresResources completes")
	return nil
}

// ReconcilePostgresDeployment reconciles the deployment and related resources (Phase 2)
func ReconcilePostgresDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcilePostgresDeployment starts")

	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile Postgres PVC",
			Task: reconcilePostgresPVC,
		},
		{
			Name: "reconcile Postgres Deployment",
			Task: reconcilePostgresDeployment,
		},
		{
			Name: "reconcile Postgres Service",
			Task: reconcilePostgresService,
		},
	}

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcilePostgresDeployment error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("reconcilePostgresDeployment completes")
	return nil
}

func reconcilePostgresDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GeneratePostgresDeployment(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new OLS postgres deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresDeployment, err)
	}

	err = UpdatePostgresDeployment(r, ctx, cr, existingDeployment, desiredDeployment)

	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdatePostgresDeployment, err)
	}

	r.GetLogger().Info("OLS postgres deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func reconcilePostgresPVC(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {

	if cr.Spec.OLSConfig.Storage == nil {
		return nil
	}
	pvc, err := GeneratePostgresPVC(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresPVC, err)
	}

	foundPVC := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresPVCName, Namespace: r.GetNamespace()}, foundPVC)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, pvc)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresPVC, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresPVC, err)
	}
	r.GetLogger().Info("OLS postgres PVC reconciled", "pvc", pvc.Name)
	return nil
}

func reconcilePostgresService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GeneratePostgresService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresService, err)
	}

	foundService := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresServiceName, Namespace: r.GetNamespace()}, foundService)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresService, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresService, err)
	}
	r.GetLogger().Info("OLS postgres service reconciled", "service", service.Name)
	return nil
}

func reconcilePostgresConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	configMap, err := GeneratePostgresConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresConfigMap, err)
	}

	foundConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresConfigMap, Namespace: r.GetNamespace()}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, configMap)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresConfigMap, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresConfigMap, err)
	}
	r.GetLogger().Info("OLS postgres configmap reconciled", "configmap", configMap.Name)
	return nil
}

func reconcilePostgresBootstrapSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := GeneratePostgresBootstrapSecret(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresBootstrapSecret, err)
	}

	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresBootstrapSecretName, Namespace: r.GetNamespace()}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresBootstrapSecret, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresBootstrapSecret, err)
	}
	r.GetLogger().Info("OLS postgres bootstrap secret reconciled", "secret", secret.Name)
	return nil
}

func reconcilePostgresSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := GeneratePostgresSecret(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresSecret, err)
	}
	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: r.GetNamespace()}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		err = deleteOldPostgresSecrets(r, ctx)
		if err != nil {
			return err
		}
		r.GetLogger().Info("creating a new Postgres secret", "secret", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresSecret, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresSecret, err)
	}

	// Check if secret data has changed
	if string(foundSecret.Data[utils.PostgresSecretKeyName]) == string(secret.Data[utils.PostgresSecretKeyName]) {
		r.GetLogger().Info("OLS postgres secret reconciliation skipped", "secret", foundSecret.Name)
		return nil
	}

	secret.Data[utils.PostgresSecretKeyName] = foundSecret.Data[utils.PostgresSecretKeyName]
	err = r.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdatePostgresSecret, err)
	}
	r.GetLogger().Info("OLS postgres secret reconciled", "secret", secret.Name)
	return nil
}

func deleteOldPostgresSecrets(r reconciler.Reconciler, ctx context.Context) error {
	labelSelector := labels.Set{"app.kubernetes.io/name": "lightspeed-service-postgres"}.AsSelector()
	matchingLabels := client.MatchingLabelsSelector{Selector: labelSelector}
	oldSecrets := &corev1.SecretList{}
	err := r.List(ctx, oldSecrets, &client.ListOptions{Namespace: r.GetNamespace(), LabelSelector: labelSelector})
	if err != nil {
		return fmt.Errorf("failed to list old Postgres secrets: %w", err)
	}
	r.GetLogger().Info("deleting old Postgres secrets", "count", len(oldSecrets.Items))

	deleteOptions := &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			Namespace:     r.GetNamespace(),
			LabelSelector: matchingLabels,
		},
	}
	if err := r.DeleteAllOf(ctx, &corev1.Secret{}, deleteOptions); err != nil {
		return fmt.Errorf("failed to delete old Postgres secrets: %w", err)
	}
	return nil
}

func reconcilePostgresNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	networkPolicy, err := GeneratePostgresNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresNetworkPolicy, err)
	}
	foundNetworkPolicy := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresNetworkPolicyName, Namespace: r.GetNamespace()}, foundNetworkPolicy)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, networkPolicy)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresNetworkPolicy, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresNetworkPolicy, err)
	}
	if utils.NetworkPolicyEqual(foundNetworkPolicy, networkPolicy) {
		r.GetLogger().Info("OLS postgres network policy unchanged, reconciliation skipped", "network policy", networkPolicy.Name)
		return nil
	}
	foundNetworkPolicy.Spec = networkPolicy.Spec
	err = r.Update(ctx, foundNetworkPolicy)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdatePostgresNetworkPolicy, err)
	}
	r.GetLogger().Info("OLS postgres network policy reconciled", "network policy", networkPolicy.Name)
	return nil
}

// RestartPostgres triggers a rolling restart of the Postgres deployment by updating its pod template annotation.
// This is useful when configuration changes require a pod restart (e.g., ConfigMap or Secret updates).
func RestartPostgres(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	// If deployment is provided, use it; otherwise fetch it
	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		// Get the Postgres deployment
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresDeploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			r.GetLogger().Info("failed to get deployment", "deploymentName", utils.PostgresDeploymentName, "error", err)
			return err
		}
	}

	// Initialize annotations map if empty
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}

	// Bump the annotation to trigger a rolling update (new template hash)
	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	// Update the deployment
	r.GetLogger().Info("triggering Postgres rolling restart", "deployment", dep.Name)
	err = r.Update(ctx, dep)
	if err != nil {
		r.GetLogger().Info("failed to update deployment", "deploymentName", dep.Name, "error", err)
		return err
	}

	return nil
}

// =============================================================================
// Test Helper Functions
// =============================================================================
// The following functions are convenience wrappers used primarily by unit tests.
// Production code should call ReconcilePostgresResources and ReconcilePostgresDeployment directly.

// ReconcilePostgres reconciles all Postgres resources in the original order.
// This function is maintained for backward compatibility with existing tests.
// New code should call ReconcilePostgresResources and ReconcilePostgresDeployment separately.
func ReconcilePostgres(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcilePostgresServer starts")

	// Call Resources phase
	if err := ReconcilePostgresResources(r, ctx, olsconfig); err != nil {
		return err
	}

	// Call Deployment phase
	if err := ReconcilePostgresDeployment(r, ctx, olsconfig); err != nil {
		return err
	}

	r.GetLogger().Info("reconcilePostgresServer completed")
	return nil
}

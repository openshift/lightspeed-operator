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

// ReconcilePostgres reconciles the Postgres server component
func ReconcilePostgres(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcilePostgresServer starts")
	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile Postgres ConfigMap",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresConfigMap(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Postgres Bootstrap Secret",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresBootstrapSecret(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Postgres Secret",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresSecret(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Postgres Service",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresService(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Postgres PVC",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresPVC(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Postgres Deployment",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresDeployment(r, ctx, cr)
			},
		},
		{
			Name: "generate Postgres Network Policy",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcilePostgresNetworkPolicy(r, ctx, cr)
			},
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcilePostgresServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("reconcilePostgresServer completed")

	return nil
}

func reconcilePostgresDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GeneratePostgresDeployment(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.PostgresDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		utils.UpdateDeploymentAnnotations(desiredDeployment, map[string]string{
			utils.PostgresConfigHashKey: r.GetStateCache()[utils.PostgresConfigHashStateCacheKey],
			utils.PostgresSecretHashKey: r.GetStateCache()[utils.PostgresSecretHashStateCacheKey],
		})
		utils.UpdateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			utils.PostgresConfigHashKey: r.GetStateCache()[utils.PostgresConfigHashStateCacheKey],
			utils.PostgresSecretHashKey: r.GetStateCache()[utils.PostgresSecretHashStateCacheKey],
		})
		r.GetLogger().Info("creating a new OLS postgres deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePostgresDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresDeployment, err)
	}

	err = UpdatePostgresDeployment(r, ctx, existingDeployment, desiredDeployment)

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
		r.GetStateCache()[utils.PostgresSecretHashStateCacheKey] = secret.Annotations[utils.PostgresSecretHashKey]
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPostgresSecret, err)
	}
	foundSecretHash, err := utils.HashBytes(foundSecret.Data[utils.PostgresSecretKeyName])
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePostgresSecretHash, err)
	}
	if foundSecretHash == r.GetStateCache()[utils.PostgresSecretHashStateCacheKey] {
		r.GetLogger().Info("OLS postgres secret reconciliation skipped", "secret", foundSecret.Name, "hash", foundSecret.Annotations[utils.PostgresSecretHashKey])
		return nil
	}
	r.GetStateCache()[utils.PostgresSecretHashStateCacheKey] = foundSecretHash
	secret.Annotations[utils.PostgresSecretHashKey] = foundSecretHash
	secret.Data[utils.PostgresSecretKeyName] = foundSecret.Data[utils.PostgresSecretKeyName]
	err = r.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdatePostgresSecret, err)
	}
	r.GetLogger().Info("OLS postgres reconciled", "secret", secret.Name, "hash", secret.Annotations[utils.PostgresSecretHashKey])
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

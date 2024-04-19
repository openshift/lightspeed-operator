package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func (r *OLSConfigReconciler) reconcilePostgresServer(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcilePostgresServer starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile Postgres ConfigMap",
			Task: r.reconcilePgConfigMap,
		},
		{
			Name: "reconcile Postgres Bootstrap Secret",
			Task: r.reconcilePgBootstrapSecret,
		},
		{
			Name: "reconcile Postgres Secret",
			Task: r.reconcilePostgresSecret,
		},
		{
			Name: "reconcile Postgres Service",
			Task: r.reconcilePostgresService,
		},
		{
			Name: "reconcile Postgres Deployment",
			Task: r.reconcilePostgresDeployment,
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.logger.Error(err, "reconcilePostgresServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcilePostgresServer completed")

	return nil
}

func (r *OLSConfigReconciler) reconcilePostgresDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := r.generatePostgresDeployment(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratePostgresDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: PostgresDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			PostgresConfigHashKey: r.stateCache[PostgresConfigHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			PostgresConfigHashKey: r.stateCache[PostgresConfigHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		r.logger.Info("creating a new OLS postgres deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreatePostgresDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetPostgresDeployment, err)
	}

	err = r.updatePostgresDeployment(ctx, existingDeployment, desiredDeployment)

	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdatePostgresDeployment, err)
	}

	r.logger.Info("OLS postgres deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcilePostgresService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generatePostgresService(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratePostgresService, err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: PostgresServiceName, Namespace: r.Options.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreatePostgresService, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetPostgresService, err)
	}
	r.logger.Info("OLS postgres service reconciled", "service", service.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcilePgConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	configMap, err := r.generatePgConfigMap(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratePostgresConfigMap, err)
	}

	foundConfigMap := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: PostgresConfigMap, Namespace: r.Options.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, configMap)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreatePostgresConfigMap, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetPostgresConfigMap, err)
	}
	r.logger.Info("OLS postgres configmap reconciled", "configmap", configMap.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcilePgBootstrapSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := r.generatePgBootstrapSecret(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratePostgresBootstrapSecret, err)
	}

	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: PostgresBootstrapSecretName, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreatePostgresBootstrapSecret, err)
		}
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetPostgresBootstrapSecret, err)
	}
	r.logger.Info("OLS postgres bootstrap secret reconciled", "secret", secret.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcilePostgresSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := r.generatePostgresSecret(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratePostgresSecret, err)
	}
	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		labelSelector := labels.Set{"app.kubernetes.io/name": "lightspeed-service-postgres"}.AsSelector()
		oldSecrets := &corev1.SecretList{}
		err = r.Client.List(ctx, oldSecrets, &client.ListOptions{Namespace: r.Options.Namespace, LabelSelector: labelSelector})
		if err != nil {
			return fmt.Errorf("%s: %w", ErrListOldPostgresSecrets, err)
		}
		for _, oldSecret := range oldSecrets.Items {
			oldSecretCopy := oldSecret // Create a local copy of the loop variable to fix G601
			if err := r.Client.Delete(ctx, &oldSecretCopy); err != nil {
				return fmt.Errorf("%s: %w", ErrDeleteOldPostgresSecrets, err)
			}
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreatePostgresSecret, err)
		}
		r.stateCache[PostgresSecretHashStateCacheKey] = secret.Annotations[PostgresSecretHashKey]
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetPostgresSecret, err)
	}
	foundSecretHash, err := hashBytes(foundSecret.Data[PostgresSecretKeyName])
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratePostgresSecretHash, err)
	}
	if foundSecretHash == r.stateCache[PostgresSecretHashStateCacheKey] {
		r.logger.Info("OLS postgres secret reconciliation skipped", "secret", foundSecret.Name, "hash", foundSecret.Annotations[PostgresSecretHashKey])
		return nil
	}
	r.stateCache[PostgresSecretHashStateCacheKey] = foundSecretHash
	secret.Annotations[PostgresSecretHashKey] = foundSecretHash
	secret.Data[PostgresSecretKeyName] = foundSecret.Data[PostgresSecretKeyName]
	err = r.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdatePostgresSecret, err)
	}
	r.logger.Info("OLS postgres reconciled", "secret", secret.Name, "hash", secret.Annotations[PostgresSecretHashKey])
	return nil
}

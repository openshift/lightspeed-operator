package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func (r *OLSConfigReconciler) reconcileRedisServer(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcileRedisServer starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile Redis Secret",
			Task: r.reconcileRedisSecret,
		},
		{
			Name: "reconcile Redis Service",
			Task: r.reconcileRedisService,
		},
		{
			Name: "reconcile Redis CA Configmap",
			Task: r.reconcileRedisCAConfigMap,
		},
		{
			Name: "reconcile Redis TLS Secret",
			Task: r.reconcileRedisTLSSecret,
		},
		{
			Name: "reconcile Redis Deployment",
			Task: r.reconcileRedisDeployment,
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.logger.Error(err, "reconcileRedisServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileRedisServer completes")

	return nil
}

func (r *OLSConfigReconciler) reconcileRedisDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := r.generateRedisDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis deployment: %w", err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: RedisDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			RedisConfigHashKey: r.stateCache[RedisConfigHashStateCacheKey],
			RedisTLSHashKey:    r.stateCache[RedisTLSHashStateCacheKey],
			RedisCAHashKey:     r.stateCache[RedisCAHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			RedisConfigHashKey: r.stateCache[RedisConfigHashStateCacheKey],
			RedisTLSHashKey:    r.stateCache[RedisTLSHashStateCacheKey],
			RedisCAHashKey:     r.stateCache[RedisCAHashStateCacheKey],
		})
		r.logger.Info("creating a new OLS redis deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("failed to create OLS redis deployment: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS redis deployment: %w", err)
	}

	err = r.updateRedisDeployment(ctx, existingDeployment, desiredDeployment)

	if err != nil {
		return fmt.Errorf("failed to update OLS redis deployment: %w", err)
	}

	r.logger.Info("OLS redis deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generateRedisService(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis service: %w", err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: RedisServiceName, Namespace: r.Options.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create OLS redis service: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get OLS redis service: %w", err)
	}
	r.logger.Info("OLS redis service reconciled", "service", service.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := r.generateRedisSecret(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis secret: %w", err)
	}
	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		labelSelector := labels.Set{"app.kubernetes.io/name": "lightspeed-service-redis"}.AsSelector()
		oldSecrets := &corev1.SecretList{}
		err = r.Client.List(ctx, oldSecrets, &client.ListOptions{Namespace: r.Options.Namespace, LabelSelector: labelSelector})
		if err != nil {
			return fmt.Errorf("failed to list old OLS redis secrets: %w", err)
		}
		for _, oldSecret := range oldSecrets.Items {
			oldSecretCopy := oldSecret // Create a local copy of the loop variable to fix G601
			if err := r.Client.Delete(ctx, &oldSecretCopy); err != nil {
				return fmt.Errorf("failed to delete old OLS redis secret: %w", err)
			}
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("failed to create OLS redis secret: %w", err)
		}
		r.stateCache[RedisSecretHashStateCacheKey] = secret.Annotations[RedisSecretHashKey]
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS redis secret: %w", err)
	}
	foundSecretHash, err := hashBytes(foundSecret.Data[RedisSecretKeyName])
	if err != nil {
		return fmt.Errorf("failed to generate hash for the existing OLS redis secret: %w", err)
	}
	if foundSecretHash == r.stateCache[RedisSecretHashStateCacheKey] {
		r.logger.Info("OLS redis secret reconciliation skipped", "secret", foundSecret.Name, "hash", foundSecret.Annotations[RedisSecretHashKey])
		return nil
	}
	r.stateCache[RedisSecretHashStateCacheKey] = foundSecretHash
	secret.Annotations[RedisSecretHashKey] = foundSecretHash
	secret.Data[RedisSecretKeyName] = foundSecret.Data[RedisSecretKeyName]
	err = r.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to update OLS redis secret: %w", err)
	}
	r.logger.Info("OLS redis secret reconciled", "secret", secret.Name, "hash", secret.Annotations[RedisSecretHashKey])
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisTLSSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	var secretValues map[string]string
	foundSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: RedisCertsSecretName, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to fetch secret: %w", err)
	}
	secretValues, err = getSecretContent(r.Client, RedisCertsSecretName, r.Options.Namespace, []string{"tls.key", "tls.crt"})
	if err != nil {
		return fmt.Errorf("failed fetching tls certs from secret: %s. error: %w", RedisCertsSecretName, err)
	}
	if err = controllerutil.SetControllerReference(cr, foundSecret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference to secret: %s. error: %w", foundSecret.Name, err)
	}
	err = r.Update(ctx, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	foundTLSSecretHash, err := hashBytes([]byte(secretValues["tls.key"] + secretValues["tls.crt"]))
	if err != nil {
		return fmt.Errorf("failed to generate OLS app tls certs hash %w", err)
	}
	if foundTLSSecretHash == r.stateCache[RedisTLSHashStateCacheKey] {
		r.logger.Info("Redis tls secret reconciliation skipped", "hash", foundTLSSecretHash)
		return nil
	}
	r.stateCache[RedisTLSHashStateCacheKey] = foundTLSSecretHash
	r.logger.Info("Redis tls secret reconciled", "hash", foundTLSSecretHash)
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisCAConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	foundConfigMap := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: RedisCAConfigMap, Namespace: r.Options.Namespace}, foundConfigMap)
	if err != nil {
		return fmt.Errorf("failed to fetch ConfigMap: %w", err)
	}
	if err = controllerutil.SetControllerReference(cr, foundConfigMap, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference to configmap: %s. error: %w", foundConfigMap.Name, err)
	}
	err = r.Update(ctx, foundConfigMap)
	if err != nil {
		return fmt.Errorf("failed to update configmap:%s. error: %w", foundConfigMap.Name, err)
	}
	configMapCACert, ok := foundConfigMap.Data["service-ca.crt"]
	if !ok {
		return fmt.Errorf("failed to get configmap:%s content", foundConfigMap.Name)
	}

	foundCAHash, err := hashBytes([]byte(configMapCACert))
	if err != nil {
		return fmt.Errorf("failed to generate Redis CA cert hash %w", err)
	}
	if foundCAHash == r.stateCache[RedisCAHashStateCacheKey] {
		r.logger.Info("Redis CA cert reconciliation skipped", "hash", foundCAHash)
		return nil
	}
	r.stateCache[RedisCAHashStateCacheKey] = foundCAHash
	r.stateCache[OLSCAHashStateCacheKey] = foundCAHash
	r.logger.Info("Redis CA cert reconciled", "hash", foundCAHash)
	return nil
}

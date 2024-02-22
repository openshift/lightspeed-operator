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

func (r *OLSConfigReconciler) reconcileAppServer(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcileAppServer starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile ServiceAccount",
			Task: r.reconcileServiceAccount,
		},
		{
			Name: "reconcile Redis Secret",
			Task: r.reconcileRedisSecret,
		},
		{
			Name: "reconcile OLSConfigMap",
			Task: r.reconcileOLSConfigMap,
		},
		{
			Name: "reconcile App Service",
			Task: r.reconcileService,
		},
		{
			Name: "reconcile Redis Service",
			Task: r.reconcileRedisService,
		},
		{
			Name: "reconcile App Deployment",
			Task: r.reconcileDeployment,
		},
		{
			Name: "reconcile Redis Deployment",
			Task: r.reconcileRedisDeployment,
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.logger.Error(err, "reconcileAppServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileAppServer completes")

	return nil
}

func (r *OLSConfigReconciler) reconcileOLSConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := r.generateOLSConfigMap(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS configmap: %w", err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSConfigCmName, Namespace: cr.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create OLS configmap: %w", err)
		}
		r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]

		return nil

	} else if err != nil {
		return fmt.Errorf("failed to get OLS configmap: %w", err)
	}
	foundCmHash, err := hashBytes([]byte(foundCm.Data[OLSConfigFilename]))
	if err != nil {
		return fmt.Errorf("failed to generate hash for the existing OLS configmap: %w", err)
	}
	// update the state cache with the hash of the existing configmap.
	// so that we can skip the reconciling the deployment if the configmap has not changed.
	r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
	if foundCmHash == cm.Annotations[OLSConfigHashKey] {
		r.logger.Info("OLS configmap reconciliation skipped", "configmap", foundCm.Name, "hash", foundCm.Annotations[OLSConfigHashKey])
		return nil
	}

	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("failed to update OLS configmap: %w", err)
	}
	r.logger.Info("OLS configmap reconciled", "configmap", cm.Name, "hash", cm.Annotations[OLSConfigHashKey])
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secretName := OLSAppRedisSecretName
	redisConfig := cr.Spec.OLSConfig.ConversationCache.Redis
	if redisConfig != (olsv1alpha1.RedisSpec{}) {
		if (redisConfig.CredentialsSecretRef != corev1.LocalObjectReference{}) && (redisConfig.CredentialsSecretRef.Name != "") {
			secretName = redisConfig.CredentialsSecretRef.Name
		}
	}
	cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecretRef.Name = secretName
	secret, err := r.generateRedisSecret(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis secret: %w", err)
	}
	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: secretName, Namespace: cr.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		labelSelector := labels.Set{"app.kubernetes.io/name": "lightspeed-service-redis"}.AsSelector()
		oldSecrets := &corev1.SecretList{}
		err = r.Client.List(ctx, oldSecrets, &client.ListOptions{Namespace: cr.Namespace, LabelSelector: labelSelector})
		if err != nil {
			return fmt.Errorf("failed to list old OLS redis secrets: %w", err)
		}
		for _, oldSecret := range oldSecrets.Items {
			if err := r.Client.Delete(ctx, &oldSecret); err != nil {
				return fmt.Errorf("failed to delete old OLS redis secret: %w", err)
			}
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("failed to create OLS redis secret: %w", err)
		}
		r.stateCache[OLSRedisSecretHashStateCacheKey] = secret.Annotations[OLSRedisSecretHashKey]
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS redis secret: %w", err)
	}
	foundSecretHash, err := hashBytes(foundSecret.Data[OLSRedisSecretKeyName])
	if err != nil {
		return fmt.Errorf("failed to generate hash for the existing OLS redis secret: %w", err)
	}
	if foundSecretHash == r.stateCache[OLSRedisSecretHashStateCacheKey] {
		r.logger.Info("OLS redis secret reconciliation skipped", "secret", foundSecret.Name, "hash", foundSecret.Annotations[OLSRedisSecretHashKey])
		return nil
	}
	r.stateCache[OLSRedisSecretHashStateCacheKey] = foundSecretHash
	secret.Annotations[OLSRedisSecretHashKey] = foundSecretHash
	secret.Data[OLSRedisSecretKeyName] = foundSecret.Data[OLSRedisSecretKeyName]
	err = r.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to update OLS redis secret: %w", err)
	}
	r.logger.Info("OLS redis secret reconciled", "secret", secret.Name, "hash", secret.Annotations[OLSRedisSecretHashKey])
	return nil
}

func (r *OLSConfigReconciler) reconcileServiceAccount(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := r.generateServiceAccount(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS service account: %w", err)
	}

	foundSa := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceAccountName, Namespace: cr.Namespace}, foundSa)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service account", "serviceAccount", sa.Name)
		err = r.Create(ctx, sa)
		if err != nil {
			return fmt.Errorf("failed to create OLS service account: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS service account: %w", err)
	}
	r.logger.Info("OLS service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := r.generateOLSDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS deployment: %w", err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: cr.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			OLSRedisSecretHashKey: r.stateCache[OLSRedisSecretHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			OLSRedisSecretHashKey: r.stateCache[OLSRedisSecretHashStateCacheKey],
		})
		r.logger.Info("creating a new deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("failed to create OLS deployment: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS deployment: %w", err)
	}

	err = r.updateOLSDeployment(ctx, existingDeployment, desiredDeployment)

	if err != nil {
		return fmt.Errorf("failed to update OLS deployment: %w", err)
	}

	return nil
}

func (r *OLSConfigReconciler) reconcileService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generateService(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS service: %w", err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceName, Namespace: cr.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service", "service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create OLS service: %w", err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS service: %w", err)
	}
	r.logger.Info("OLS service reconciled", "service", service.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := r.generateRedisDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis deployment: %w", err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppRedisDeploymentName, Namespace: cr.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
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
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppRedisServiceName, Namespace: cr.Namespace}, foundService)
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

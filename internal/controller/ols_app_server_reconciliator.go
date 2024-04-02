package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
			Name: "reconcile SARRole",
			Task: r.reconcileSARRole,
		},
		{
			Name: "reconcile SARRoleBinding",
			Task: r.reconcileSARRoleBinding,
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
			Name: "reconcile App TLS Certs",
			Task: r.reconcileTLSSecret,
		},
		{
			Name: "reconcile App Deployment",
			Task: r.reconcileDeployment,
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
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSConfigCmName, Namespace: r.Options.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create OLS configmap: %w", err)
		}
		r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
		// TODO: Update DB
		//r.stateCache[RedisConfigHashStateCacheKey] = cm.Annotations[RedisConfigHashKey]

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
	// TODO: Update DB
	//r.stateCache[RedisConfigHashStateCacheKey] = cm.Annotations[RedisConfigHashKey]
	if foundCmHash == cm.Annotations[OLSConfigHashKey] {
		r.logger.Info("OLS configmap reconciliation skipped", "configmap", foundCm.Name, "hash", foundCm.Annotations[OLSConfigHashKey])
		return nil
	}
	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	err = r.Update(ctx, foundCm)
	if err != nil {
		return fmt.Errorf("failed to update OLS configmap: %w", err)
	}
	r.logger.Info("OLS configmap reconciled", "configmap", cm.Name, "hash", cm.Annotations[OLSConfigHashKey])
	return nil
}

func (r *OLSConfigReconciler) reconcileServiceAccount(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := r.generateServiceAccount(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS service account: %w", err)
	}

	foundSa := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceAccountName, Namespace: r.Options.Namespace}, foundSa)
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

func (r *OLSConfigReconciler) reconcileSARRole(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	role, err := r.generateSARClusterRole(cr)
	if err != nil {
		return fmt.Errorf("failed to generate SAR cluster role: %w", err)
	}

	foundRole := &rbacv1.ClusterRole{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: role.Name}, foundRole)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new SAR cluster role", "ClusterRole", role.Name)
		err = r.Create(ctx, role)
		if err != nil {
			return fmt.Errorf("failed to create SAR cluster role: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get SAR cluster role: %w", err)
	}
	r.logger.Info("SAR cluster role reconciled", "ClusterRole", role.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileSARRoleBinding(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	rb, err := r.generateSARClusterRoleBinding(cr)
	if err != nil {
		return fmt.Errorf("failed to generate SAR cluster role binding: %w", err)
	}

	foundRB := &rbacv1.ClusterRoleBinding{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: rb.Name}, foundRB)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new SAR cluster role binding", "ClusterRoleBinding", rb.Name)
		err = r.Create(ctx, rb)
		if err != nil {
			return fmt.Errorf("failed to create SAR cluster role binding: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get SAR cluster role binding: %w", err)
	}
	r.logger.Info("SAR cluster role binding reconciled", "ClusterRoleBinding", rb.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := r.generateOLSDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS deployment: %w", err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
			OLSCAHashKey:     r.stateCache[OLSCAHashStateCacheKey],
			OLSAppTLSHashKey: r.stateCache[OLSAppTLSHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
			OLSCAHashKey:     r.stateCache[OLSCAHashStateCacheKey],
			OLSAppTLSHashKey: r.stateCache[OLSAppTLSHashStateCacheKey],
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
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceName, Namespace: r.Options.Namespace}, foundService)
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

	if serviceEqual(foundService, service) &&
		foundService.ObjectMeta.Annotations != nil &&
		foundService.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] == service.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] {
		r.logger.Info("OLS service unchanged, reconciliation skipped", "service", service.Name)
		return nil
	}

	err = r.Update(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to update OLS service: %w", err)
	}

	r.logger.Info("OLS service reconciled", "service", service.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileTLSSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	var secretValues map[string]string
	foundSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: OLSCertsSecretName, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to fetch secret: %w", err)
	}
	secretValues, err = getSecretContent(r.Client, OLSCertsSecretName, r.Options.Namespace, []string{"tls.key", "tls.crt"})
	if err != nil {
		return fmt.Errorf("failed fetching tls certs from secret: %s. error: %w", OLSCertsSecretName, err)
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
	if foundTLSSecretHash == r.stateCache[OLSAppTLSHashStateCacheKey] {
		r.logger.Info("OLS app tls secret reconciliation skipped", "hash", foundTLSSecretHash)
		return nil
	}
	r.stateCache[OLSAppTLSHashStateCacheKey] = foundTLSSecretHash
	r.logger.Info("OLS app tls secret reconciled", "hash", foundTLSSecretHash)
	return nil
}

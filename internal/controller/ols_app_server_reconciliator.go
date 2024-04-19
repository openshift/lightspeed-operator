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
		return fmt.Errorf("%s: %w", ErrGenerateAPIConfigmap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSConfigCmName, Namespace: r.Options.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIConfigmap, err)
			return fmt.Errorf("%s: %w", ErrCreateAPIConfigmap, err)
		}
		r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
		r.stateCache[PostgresConfigHashStateCacheKey] = cm.Annotations[PostgresConfigHashKey]

		return nil

	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetAPIConfigmap, err)
	}
	foundCmHash, err := hashBytes([]byte(foundCm.Data[OLSConfigFilename]))
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateHash, err)
	}
	// update the state cache with the hash of the existing configmap.
	// so that we can skip the reconciling the deployment if the configmap has not changed.
	r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
	r.stateCache[PostgresConfigHashStateCacheKey] = cm.Annotations[PostgresConfigHashKey]
	if foundCmHash == cm.Annotations[OLSConfigHashKey] {
		r.logger.Info("OLS configmap reconciliation skipped", "configmap", foundCm.Name, "hash", foundCm.Annotations[OLSConfigHashKey])
		return nil
	}
	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	err = r.Update(ctx, foundCm)
	if err != nil {

		r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrUpdateAPIConfigmap, err)
		return fmt.Errorf("%s: %w", ErrUpdateAPIConfigmap, err)
	}
	r.logger.Info("OLS configmap reconciled", "configmap", cm.Name, "hash", cm.Annotations[OLSConfigHashKey])
	return nil
}

func (r *OLSConfigReconciler) reconcileServiceAccount(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := r.generateServiceAccount(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateAPIServiceAccount, err)
	}

	foundSa := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceAccountName, Namespace: r.Options.Namespace}, foundSa)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service account", "serviceAccount", sa.Name)
		err = r.Create(ctx, sa)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIServiceAccount, err)
			return fmt.Errorf("%s: %w", ErrCreateAPIServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetAPIServiceAccount, err)
	}
	r.logger.Info("OLS service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileSARRole(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	role, err := r.generateSARClusterRole(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateSARClusterRole, err)
	}

	foundRole := &rbacv1.ClusterRole{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: role.Name}, foundRole)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new SAR cluster role", "ClusterRole", role.Name)
		err = r.Create(ctx, role)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateSARClusterRole, err)
			return fmt.Errorf("%s: %w", ErrCreateSARClusterRole, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetSARClusterRole, err)
	}
	r.logger.Info("SAR cluster role reconciled", "ClusterRole", role.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileSARRoleBinding(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	rb, err := r.generateSARClusterRoleBinding(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateSARClusterRoleBinding, err)
	}

	foundRB := &rbacv1.ClusterRoleBinding{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: rb.Name}, foundRB)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new SAR cluster role binding", "ClusterRoleBinding", rb.Name)
		err = r.Create(ctx, rb)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateSARClusterRoleBinding, err)
			return fmt.Errorf("%s: %w", ErrCreateSARClusterRoleBinding, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetSARClusterRoleBinding, err)
	}
	r.logger.Info("SAR cluster role binding reconciled", "ClusterRoleBinding", rb.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := r.generateOLSDeployment(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateAPIDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			LLMProviderHashKey:    r.stateCache[LLMProviderHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			LLMProviderHashKey:    r.stateCache[LLMProviderHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		r.logger.Info("creating a new deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIDeployment, err)
			return fmt.Errorf("%s: %w", ErrCreateAPIDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetAPIDeployment, err)
	}

	err = r.updateOLSDeployment(ctx, existingDeployment, desiredDeployment)
	if err != nil {
		r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrUpdateAPIDeployment, err)
		return fmt.Errorf("%s: %w", ErrUpdateAPIDeployment, err)
	}

	return nil
}

func (r *OLSConfigReconciler) reconcileService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generateService(cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateAPIService, err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceName, Namespace: r.Options.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service", "service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIService, err)
			return fmt.Errorf("%s: %w", ErrCreateAPIService, err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetAPIServiceAccount, err)
	}

	if serviceEqual(foundService, service) &&
		foundService.ObjectMeta.Annotations != nil &&
		foundService.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] == service.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] {
		r.logger.Info("OLS service unchanged, reconciliation skipped", "service", service.Name)
		return nil
	}

	err = r.Update(ctx, service)
	if err != nil {
		r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrUpdateAPIService, err)
		return fmt.Errorf("%s: %w", ErrUpdateAPIService, err)
	}

	r.logger.Info("OLS service reconciled", "service", service.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileLLMSecrets(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	providerCredentials := ""
	for _, provider := range cr.Spec.LLMConfig.Providers {
		foundSecret := &corev1.Secret{}
		providerApiToken, err := getSecretContent(r.Client, provider.CredentialsSecretRef.Name, r.Options.Namespace, LLMApiTokenFileName, foundSecret)
		if err != nil {
			return fmt.Errorf("Secret token not found for provider: %s. error: %w", provider.Name, err)
		}
		providerCredentials += providerApiToken
		if err = controllerutil.SetControllerReference(cr, foundSecret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference to secret: %s. error: %w", foundSecret.Name, err)
		}
		err = r.Update(ctx, foundSecret)
		if err != nil {
			return fmt.Errorf("%s: %s error: %w", ErrUpdateProviderSecret, foundSecret.Name, err)
		}
	}
	foundProviderCredentialsHash, err := hashBytes([]byte(providerCredentials))
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateProviderCredentialsHash, err)
	}
	if foundProviderCredentialsHash == r.stateCache[LLMProviderHashStateCacheKey] {
		r.logger.Info("OLS llm secrets reconciliation skipped", "hash", foundProviderCredentialsHash)
		return nil
	}
	r.stateCache[LLMProviderHashStateCacheKey] = foundProviderCredentialsHash
	r.logger.Info("OLS llm secrets reconciled", "hash", foundProviderCredentialsHash)
	return nil
}

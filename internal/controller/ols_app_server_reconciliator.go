package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func (r *OLSConfigReconciler) reconcileAppServer(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) (string, error) {
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
			Name: "reconcile Additional CA ConfigMap",
			Task: r.reconcileOLSAdditionalCAConfigMap,
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
			Name: "reconcile Metrics Reader Secret",
			Task: r.reconcileMetricsReaderSecret,
		},
		{
			Name: "reconcile App ServiceMonitor",
			Task: r.reconcileServiceMonitor,
		},
		{
			Name: "reconcile App PrometheusRule",
			Task: r.reconcilePrometheusRule,
		},
		{
			Name: "reconcile App NetworkPolicy",
			Task: r.reconcileAppServerNetworkPolicy,
		},
		{
			Name: "reconcile Proxy CA ConfigMap",
			Task: r.reconcileProxyCAConfigMap,
		},
		{
			Name: "reconcile App Deployment",
			Task: r.reconcileDeployment,
		},
	}

	for _, task := range tasks {
		message, err := task.Task(ctx, olsconfig)
		if err != nil {
			r.logger.Error(err, "reconcileAppServer error", "task", task.Name)
			return message, fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileAppServer completes")

	return "Ready", nil
}

func (r *OLSConfigReconciler) reconcileOLSConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	err := r.checkLLMCredentials(ctx, cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrCheckLLMCredentials, err)
	}

	cm, err := r.generateOLSConfigMap(ctx, cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateAPIConfigmap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSConfigCmName, Namespace: r.Options.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIConfigmap, err)
			return "Failed", fmt.Errorf("%s: %w", ErrCreateAPIConfigmap, err)
		}
		r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
		r.stateCache[PostgresConfigHashStateCacheKey] = cm.Annotations[PostgresConfigHashKey]

		return "", nil

	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetAPIConfigmap, err)
	}
	foundCmHash, err := hashBytes([]byte(foundCm.Data[OLSConfigFilename]))
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateHash, err)
	}
	// update the state cache with the hash of the existing configmap.
	// so that we can skip the reconciling the deployment if the configmap has not changed.
	r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
	r.stateCache[PostgresConfigHashStateCacheKey] = cm.Annotations[PostgresConfigHashKey]
	if foundCmHash == cm.Annotations[OLSConfigHashKey] {
		r.logger.Info("OLS configmap reconciliation skipped", "configmap", foundCm.Name, "hash", foundCm.Annotations[OLSConfigHashKey])
		return "", nil
	}
	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	err = r.Update(ctx, foundCm)
	if err != nil {

		r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrUpdateAPIConfigmap, err)
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateAPIConfigmap, err)
	}
	r.logger.Info("OLS configmap reconciled", "configmap", cm.Name, "hash", cm.Annotations[OLSConfigHashKey])
	return "", nil
}

func (r *OLSConfigReconciler) reconcileOLSAdditionalCAConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef == nil {
		// no additional CA certs, skip
		r.logger.Info("Additional CA not configured, reconciliation skipped")
		return "", nil
	}

	// annotate the configmap for watcher
	cm := &corev1.ConfigMap{}

	err := r.Client.Get(ctx, client.ObjectKey{Name: cr.Spec.OLSConfig.AdditionalCAConfigMapRef.Name, Namespace: r.Options.Namespace}, cm)

	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetAdditionalCACM, err)
	}

	annotateConfigMapWatcher(cm)

	err = r.Update(ctx, cm)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateAdditionalCACM, err)
	}

	certBytes := []byte{}
	for key, value := range cm.Data {
		certBytes = append(certBytes, []byte(key)...)
		certBytes = append(certBytes, []byte(value)...)
	}

	foundCmHash, err := hashBytes(certBytes)
	if err != nil {
		return "Failed", fmt.Errorf("failed to generate additional CA certs hash %w", err)
	}
	if foundCmHash == r.stateCache[AdditionalCAHashStateCacheKey] {
		r.logger.Info("Additional CA reconciliation skipped", "hash", foundCmHash)
		return "", nil
	}
	r.stateCache[AdditionalCAHashStateCacheKey] = foundCmHash

	r.logger.Info("additional CA configmap reconciled", "configmap", cm.Name, "hash", foundCmHash)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileProxyCAConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	if cr.Spec.OLSConfig.ProxyConfig == nil || cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef == nil {
		// no proxy CA certs, skip
		r.logger.Info("Proxy CA not configured, reconciliation skipped")
		return "", nil
	}

	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name, Namespace: r.Options.Namespace}, cm)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetProxyCACM, err)
	}
	annotateConfigMapWatcher(cm)
	err = r.Update(ctx, cm)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateProxyCACM, err)
	}

	r.logger.Info("proxy CA configmap reconciled", "configmap", cm.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileServiceAccount(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	sa, err := r.generateServiceAccount(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateAPIServiceAccount, err)
	}

	foundSa := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceAccountName, Namespace: r.Options.Namespace}, foundSa)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service account", "serviceAccount", sa.Name)
		err = r.Create(ctx, sa)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIServiceAccount, err)
			return "Failed", fmt.Errorf("%s: %w", ErrCreateAPIServiceAccount, err)
		}
		return "Ready", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetAPIServiceAccount, err)
	}
	r.logger.Info("OLS service account reconciled", "serviceAccount", sa.Name)
	return "Ready", nil
}

func (r *OLSConfigReconciler) reconcileSARRole(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	role, err := r.generateSARClusterRole(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateSARClusterRole, err)
	}

	foundRole := &rbacv1.ClusterRole{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: role.Name}, foundRole)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new SAR cluster role", "ClusterRole", role.Name)
		err = r.Create(ctx, role)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateSARClusterRole, err)
			return "Failed", fmt.Errorf("%s: %w", ErrCreateSARClusterRole, err)
		}
		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetSARClusterRole, err)
	}
	r.logger.Info("SAR cluster role reconciled", "ClusterRole", role.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileSARRoleBinding(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	rb, err := r.generateSARClusterRoleBinding(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateSARClusterRoleBinding, err)
	}

	foundRB := &rbacv1.ClusterRoleBinding{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: rb.Name}, foundRB)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new SAR cluster role binding", "ClusterRoleBinding", rb.Name)
		err = r.Create(ctx, rb)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateSARClusterRoleBinding, err)
			return "Failed", fmt.Errorf("%s: %w", ErrCreateSARClusterRoleBinding, err)
		}
		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetSARClusterRoleBinding, err)
	}
	r.logger.Info("SAR cluster role binding reconciled", "ClusterRoleBinding", rb.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	desiredDeployment, err := r.generateOLSDeployment(cr)
	if err != nil {
		return "failed", fmt.Errorf("%s: %w", ErrGenerateAPIDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			OLSAppTLSHashKey:      r.stateCache[OLSAppTLSHashStateCacheKey],
			LLMProviderHashKey:    r.stateCache[LLMProviderHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			OLSAppTLSHashKey:      r.stateCache[OLSAppTLSHashStateCacheKey],
			LLMProviderHashKey:    r.stateCache[LLMProviderHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		r.logger.Info("creating a new deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIDeployment, err)
			return DeploymentInProgress, fmt.Errorf("%s: %w", ErrCreateAPIDeployment, err)
		}

		// Deployment was just created – it cannot be ready yet.
		return DeploymentInProgress, fmt.Errorf("deployment is not ready, 0 replicas available")
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetAPIDeployment, err)
	}

	err = r.updateOLSDeployment(ctx, existingDeployment, desiredDeployment)
	if err != nil {
		r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrUpdateAPIDeployment, err)
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateAPIDeployment, err)
	}

	// Check deployment status
	r.logger.Info("checking deployment status", "deployment", OLSAppServerDeploymentName)

	// ----------  Deployment status  ----------
	// Re-fetch to obtain freshest conditions & ready replicas
	dep := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), dep); err != nil {
		return "Failed", fmt.Errorf("failed to read deployment status: %w", err)
	}
	return r.checkDeploymentStatus(dep)
}

func (r *OLSConfigReconciler) reconcileService(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	service, err := r.generateService(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateAPIService, err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceName, Namespace: r.Options.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service", "service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrCreateAPIService, err)
			return "Failed", fmt.Errorf("%s: %w", ErrCreateAPIService, err)
		}

		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetAPIServiceAccount, err)
	}

	if serviceEqual(foundService, service) && foundService.ObjectMeta.Annotations != nil {
		if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.CAcertificate != "" {
			r.logger.Info("OLS service unchanged, reconciliation skipped", "service", service.Name)
			return "", nil

		} else if foundService.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] == service.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] {
			r.logger.Info("OLS service unchanged, reconciliation skipped", "service", service.Name)
			return "", nil
		}
	}

	err = r.Update(ctx, service)
	if err != nil {
		r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrUpdateAPIService, err)
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateAPIService, err)
	}

	r.logger.Info("OLS service reconciled", "service", service.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileLLMSecrets(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	providerCredentials := ""
	for _, provider := range cr.Spec.LLMConfig.Providers {
		foundSecret := &corev1.Secret{}
		secretValues, err := getAllSecretContent(r.Client, provider.CredentialsSecretRef.Name, r.Options.Namespace, foundSecret)
		if err != nil {
			r.updateStatusCondition(ctx, cr, typeApiReady, false, ErrGetLLMSecret, err)
			return "Failed", fmt.Errorf("secret token not found for provider: %s. error: %w", provider.Name, err)
		}
		for key, value := range secretValues {
			providerCredentials += key + "=" + value + "\n"
		}
		annotateSecretWatcher(foundSecret)
		err = r.Update(ctx, foundSecret)
		if err != nil {
			return "Failed", fmt.Errorf("%s: %s error: %w", ErrUpdateProviderSecret, foundSecret.Name, err)
		}
	}
	foundProviderCredentialsHash, err := hashBytes([]byte(providerCredentials))
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateProviderCredentialsHash, err)
	}
	if foundProviderCredentialsHash == r.stateCache[LLMProviderHashStateCacheKey] {
		r.logger.Info("OLS llm secrets reconciliation skipped", "hash", foundProviderCredentialsHash)
		return "", nil
	}
	r.stateCache[LLMProviderHashStateCacheKey] = foundProviderCredentialsHash
	r.logger.Info("OLS llm secrets reconciled", "hash", foundProviderCredentialsHash)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileMetricsReaderSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	secret, err := r.generateMetricsReaderSecret(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateMetricsReaderSecret, err)
	}
	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new metrics reader secret", "secret", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			return "Failed", fmt.Errorf("%s: %w", ErrCreateMetricsReaderSecret, err)
		}
		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetMetricsReaderSecret, err)
	}

	if foundSecret.Type != secret.Type || foundSecret.Annotations["kubernetes.io/service-account.name"] != MetricsReaderServiceAccountName {
		foundSecret.Type = secret.Type
		foundSecret.Annotations["kubernetes.io/service-account.name"] = MetricsReaderServiceAccountName
		err = r.Update(ctx, foundSecret)
		if err != nil {
			return "Failed", fmt.Errorf("%s: %w", ErrUpdateMetricsReaderSecret, err)
		}
	}
	r.logger.Info("OLS metrics reader secret reconciled", "secret", secret.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileServiceMonitor(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	sm, err := r.generateServiceMonitor(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateServiceMonitor, err)
	}

	foundSm := &monv1.ServiceMonitor{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: AppServerServiceMonitorName, Namespace: r.Options.Namespace}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service monitor", "serviceMonitor", sm.Name)
		err = r.Create(ctx, sm)
		if err != nil {
			return "Failed", fmt.Errorf("%s: %w", ErrCreateServiceMonitor, err)
		}
		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetServiceMonitor, err)
	}
	if serviceMonitorEqual(foundSm, sm) {
		r.logger.Info("OLS service monitor unchanged, reconciliation skipped", "serviceMonitor", sm.Name)
		return "", nil
	}
	foundSm.Spec = sm.Spec
	err = r.Update(ctx, foundSm)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateServiceMonitor, err)
	}
	r.logger.Info("OLS service monitor reconciled", "serviceMonitor", sm.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcilePrometheusRule(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	rule, err := r.generatePrometheusRule(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGeneratePrometheusRule, err)
	}

	foundRule := &monv1.PrometheusRule{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: AppServerPrometheusRuleName, Namespace: r.Options.Namespace}, foundRule)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new prometheus rule", "prometheusRule", rule.Name)
		err = r.Create(ctx, rule)
		if err != nil {
			return "Failed", fmt.Errorf("%s: %w", ErrCreatePrometheusRule, err)
		}
		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetPrometheusRule, err)
	}
	if prometheusRuleEqual(foundRule, rule) {
		r.logger.Info("OLS prometheus rule unchanged, reconciliation skipped", "prometheusRule", rule.Name)
		return "", nil
	}
	foundRule.Spec = rule.Spec
	err = r.Update(ctx, foundRule)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateServiceMonitor, err)
	}
	r.logger.Info("OLS prometheus rule reconciled", "prometheusRule", rule.Name)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileTLSSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	foundSecret := &corev1.Secret{}
	var err, lastErr error
	var secretValues map[string]string
	secretName := OLSCertsSecretName
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		secretName = cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name
	}
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		secretValues, err = getSecretContent(r.Client, secretName, r.Options.Namespace, []string{"tls.key", "tls.crt"}, foundSecret)
		if err != nil {
			lastErr = fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", secretName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		r.updateStatusCondition(ctx, cr, typeApiReady, false, fmt.Sprintf("%s - %s", ErrGetTLSSecret, OLSCertsSecretName), err)
		return "Failed", fmt.Errorf("%s -%s - wait err %w; last error: %w", ErrGetTLSSecret, OLSCertsSecretName, err, lastErr)
	}

	annotateSecretWatcher(foundSecret)
	err = r.Update(ctx, foundSecret)
	if err != nil {
		return "Failed", fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	foundTLSSecretHash, err := hashBytes([]byte(secretValues["tls.key"] + secretValues["tls.crt"]))
	if err != nil {
		return "Failed", fmt.Errorf("failed to generate OLS app TLS certs hash %w", err)
	}
	if foundTLSSecretHash == r.stateCache[OLSAppTLSHashStateCacheKey] {
		r.logger.Info("OLS app TLS secret reconciliation skipped", "hash", foundTLSSecretHash)
		return "", nil
	}
	r.stateCache[OLSAppTLSHashStateCacheKey] = foundTLSSecretHash
	r.logger.Info("OLS app TLS secret reconciled", "hash", foundTLSSecretHash)
	return "", nil
}

func (r *OLSConfigReconciler) reconcileAppServerNetworkPolicy(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	networkPolicy, err := r.generateAppServerNetworkPolicy(cr)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGenerateAppServerNetworkPolicy, err)
	}

	foundNP := &networkingv1.NetworkPolicy{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerNetworkPolicyName, Namespace: r.Options.Namespace}, foundNP)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new network policy", "networkPolicy", networkPolicy.Name)
		err = r.Create(ctx, networkPolicy)
		if err != nil {
			return "Failed", fmt.Errorf("%s: %w", ErrCreateAppServerNetworkPolicy, err)
		}
		return "", nil
	} else if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrGetAppServerNetworkPolicy, err)
	}
	if networkPolicyEqual(foundNP, networkPolicy) {
		r.logger.Info("OLS app server network policy unchanged, reconciliation skipped", "networkPolicy", networkPolicy.Name)
		return "", nil
	}
	foundNP.Spec = networkPolicy.Spec
	err = r.Update(ctx, foundNP)
	if err != nil {
		return "Failed", fmt.Errorf("%s: %w", ErrUpdateAppServerNetworkPolicy, err)
	}
	r.logger.Info("OLS app server network policy reconciled", "networkPolicy", networkPolicy.Name)
	return "", nil
}

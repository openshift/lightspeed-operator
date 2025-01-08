package controller

import (
	"context"
	"fmt"
	"path"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func generateAppServerSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

func (r *OLSConfigReconciler) generateServiceAccount(cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerServiceAccountName,
			Namespace: r.Options.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &sa, r.Scheme); err != nil {
		return nil, err
	}

	return &sa, nil
}

func (r *OLSConfigReconciler) generateSARClusterRole(cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSAppServerSARRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"config.openshift.io"},
				Resources: []string{"clusterversions"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{"pull-secret"},
				Verbs:         []string{"get"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &role, r.Scheme); err != nil {
		return nil, err
	}

	return &role, nil
}

func (r *OLSConfigReconciler) generateSARClusterRoleBinding(cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRoleBinding, error) {
	rb := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSAppServerSARRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      OLSAppServerServiceAccountName,
				Namespace: r.Options.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     OLSAppServerSARRoleName,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rb, r.Scheme); err != nil {
		return nil, err
	}

	return &rb, nil
}

func (r *OLSConfigReconciler) generateOLSConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	providerConfigs := []ProviderConfig{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name)
		modelConfigs := []ModelConfig{}
		for _, model := range provider.Models {
			modelConfig := ModelConfig{
				Name: model.Name,
				URL:  model.URL,
				Parameters: ModelParameters{
					MaxTokensForResponse: model.Parameters.MaxTokensForResponse,
				},
				ContextWindowSize: model.ContextWindowSize,
			}
			modelConfigs = append(modelConfigs, modelConfig)
		}
		var providerConfig ProviderConfig
		if provider.Type == AzureOpenAIType {
			providerConfig = ProviderConfig{
				Name:   provider.Name,
				Type:   provider.Type,
				Models: modelConfigs,
				AzureOpenAIConfig: &AzureOpenAIConfig{
					URL:                 provider.URL,
					CredentialsPath:     credentialPath,
					AzureDeploymentName: provider.AzureDeploymentName,
					APIVersion:          provider.APIVersion,
				},
			}
		} else {
			providerConfig = ProviderConfig{
				Name:            provider.Name,
				Type:            provider.Type,
				URL:             provider.URL,
				CredentialsPath: credentialPath,
				Models:          modelConfigs,
				WatsonProjectID: provider.WatsonProjectID,
			}
		}
		providerConfigs = append(providerConfigs, providerConfig)
	}
	// TODO: Update DB
	// redisMaxMemory := intstr.FromString(RedisMaxMemory)
	// redisMaxMemoryPolicy := RedisMaxMemoryPolicy
	// redisSecretName := RedisSecretName
	// redisConfig := cr.Spec.OLSConfig.ConversationCache.Redis
	// if redisConfig.MaxMemory != nil && redisConfig.MaxMemory.String() != "" {
	// 	redisMaxMemory = *cr.Spec.OLSConfig.ConversationCache.Redis.MaxMemory
	// }
	// if redisConfig.MaxMemoryPolicy != "" {
	// 	redisMaxMemoryPolicy = cr.Spec.OLSConfig.ConversationCache.Redis.MaxMemoryPolicy
	// }
	// if redisConfig.CredentialsSecret != "" {
	// 	redisSecretName = cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret
	// }
	// redisPasswordPath := path.Join(CredentialsMountRoot, redisSecretName, OLSComponentPasswordFileName)
	// conversationCache := ConversationCacheConfig{
	// 	Type: string(OLSDefaultCacheType),
	// 	Redis: RedisCacheConfig{
	// 		Host:            strings.Join([]string{RedisServiceName, r.Options.Namespace, "svc"}, "."),
	// 		Port:            RedisServicePort,
	// 		MaxMemory:       &redisMaxMemory,
	// 		MaxMemoryPolicy: redisMaxMemoryPolicy,
	// 		PasswordPath:    redisPasswordPath,
	// 		CACertPath:      path.Join(OLSAppCertsMountRoot, RedisCertsSecretName, RedisCAVolume, "service-ca.crt"),
	// 	},
	// }

	conversationCache := ConversationCacheConfig{
		Type: "memory",
		Memory: MemoryCacheConfig{
			MaxEntries: 1000,
		},
	}

	major, minor, err := r.getClusterVersion(ctx)
	if err != nil {
		return nil, err
	}
	// We want to disable the data collector if the user has explicitly disabled it
	// or if the data collector is not enabled in the cluster with pull secret

	dataCollectorEnabled, _ := r.dataCollectorEnabled(cr)

	tlsConfig := TLSConfig{
		TLSCertificatePath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.crt"),
		TLSKeyPath:         path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.key"),
	}

	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		tlsConfig.TLSCertificatePath = path.Join(OLSAppCertsMountRoot, cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name, "tls.crt")
		tlsConfig.TLSKeyPath = path.Join(OLSAppCertsMountRoot, cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name, "tls.key")
	}

	olsConfig := OLSConfig{
		DefaultModel:    cr.Spec.OLSConfig.DefaultModel,
		DefaultProvider: cr.Spec.OLSConfig.DefaultProvider,
		Logging: LoggingConfig{
			AppLogLevel:     cr.Spec.OLSConfig.LogLevel,
			LibLogLevel:     cr.Spec.OLSConfig.LogLevel,
			UvicornLogLevel: cr.Spec.OLSConfig.LogLevel,
		},
		ConversationCache: conversationCache,
		TLSConfig:         tlsConfig,
		ReferenceContent: ReferenceContent{
			ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/" + major + "." + minor,
			ProductDocsIndexId:   "ocp-product-docs-" + major + "_" + minor,
			EmbeddingsModelPath:  "/app-root/embeddings_model",
		},
		UserDataCollection: UserDataCollectionConfig{
			FeedbackDisabled:    cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled || !dataCollectorEnabled,
			FeedbackStorage:     "/app-root/ols-user-data/feedback",
			TranscriptsDisabled: cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled || !dataCollectorEnabled,
			TranscriptsStorage:  "/app-root/ols-user-data/transcripts",
		},
	}

	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		caFileNames, err := r.getAdditionalCAFileNames(cr)
		if err != nil {
			return nil, fmt.Errorf("failed to generate OLS config file, additional CA error: %w", err)
		}

		olsConfig.ExtraCAs = make([]string, len(caFileNames))
		for i, caFileName := range caFileNames {
			olsConfig.ExtraCAs[i] = path.Join(OLSAppCertsMountRoot, AppAdditionalCACertDir, caFileName)
		}

		olsConfig.CertificateDirectory = path.Join(OLSAppCertsMountRoot, CertBundleDir)
	}

	if queryFilters := getQueryFilters(cr); queryFilters != nil {
		olsConfig.QueryFilters = queryFilters
	}

	userDataCollectorConfig := UserDataCollectorConfig{
		DataStorage: "/app-root/ols-user-data",
		LogLevel:    cr.Spec.OLSDataCollectorConfig.LogLevel,
	}
	var appSrvConfigFile AppSrvConfigFile
	if dataCollectorEnabled {
		appSrvConfigFile = AppSrvConfigFile{
			LLMProviders:            providerConfigs,
			OLSConfig:               olsConfig,
			UserDataCollectorConfig: userDataCollectorConfig,
		}
	} else {
		appSrvConfigFile = AppSrvConfigFile{
			LLMProviders: providerConfigs,
			OLSConfig:    olsConfig,
		}
	}

	configFileBytes, err := yaml.Marshal(appSrvConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file %w", err)
	}
	// TODO: Update DB
	// redisConfigFileBytes, err := yaml.Marshal(conversationCache.Redis)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to generate OLS redis config bytes %w", err)
	// }

	configFileHash, err := hashBytes(configFileBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file hash %w", err)
	}
	// TODO: Update DB
	// redisConfigHash, err := hashBytes(redisConfigFileBytes)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to generate OLS redis config hash %w", err)
	// }

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSConfigCmName,
			Namespace: r.Options.Namespace,
			Labels:    generateAppServerSelectorLabels(),
			Annotations: map[string]string{
				OLSConfigHashKey: configFileHash,
				// TODO: Update DB
				//RedisConfigHashKey: redisConfigHash,
			},
		},
		Data: map[string]string{
			OLSConfigFilename: string(configFileBytes),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &cm, r.Scheme); err != nil {
		return nil, err
	}

	return &cm, nil
}

func (r *OLSConfigReconciler) getAdditionalCAFileNames(cr *olsv1alpha1.OLSConfig) ([]string, error) {
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef == nil {
		return nil, nil
	}
	// get data from the referenced configmap
	cm := &corev1.ConfigMap{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: cr.Spec.OLSConfig.AdditionalCAConfigMapRef.Name, Namespace: r.Options.Namespace}, cm)
	if err != nil {
		return nil, fmt.Errorf("failed to get additional CA configmap %s/%s: %v", r.Options.Namespace, cr.Spec.OLSConfig.AdditionalCAConfigMapRef.Name, err)
	}

	filenames := []string{}

	for key, caStr := range cm.Data {
		err = validateCertificateFormat([]byte(caStr))
		if err != nil {
			return nil, fmt.Errorf("failed to validate additional CA certificate %s: %v", key, err)
		}
		filenames = append(filenames, key)
	}

	return filenames, nil

}

func (r *OLSConfigReconciler) generateService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	annotations := map[string]string{}

	// Let service-ca operator generate a TLS certificate if the user does not provide one
	if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.CAcertificate == "" {
		annotations[ServingCertSecretAnnotationKey] = OLSCertsSecretName
	} else {
		delete(annotations, ServingCertSecretAnnotationKey)
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        OLSAppServerServiceName,
			Namespace:   r.Options.Namespace,
			Labels:      generateAppServerSelectorLabels(),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					Port:       OLSAppServerServicePort,
					TargetPort: intstr.Parse("https"),
				},
			},
			Selector: generateAppServerSelectorLabels(),
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.Scheme); err != nil {
		return nil, err
	}

	return &service, nil
}

func (r *OLSConfigReconciler) generateServiceMonitor(cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
	metaLabels := generateAppServerSelectorLabels()
	metaLabels["monitoring.openshift.io/collection-profile"] = "full"
	metaLabels["app.kubernetes.io/component"] = "metrics"
	metaLabels["openshift.io/user-monitoring"] = "false"

	valFalse := false
	serverName := strings.Join([]string{OLSAppServerServiceName, r.Options.Namespace, "svc"}, ".")

	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerServiceMonitorName,
			Namespace: r.Options.Namespace,
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "https",
					Path:     AppServerMetricsPath,
					Interval: "30s",
					Scheme:   "https",
					TLSConfig: &monv1.TLSConfig{
						CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
						CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
						KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
						SafeTLSConfig: monv1.SafeTLSConfig{
							InsecureSkipVerify: &valFalse,
							ServerName:         &serverName,
						},
					},
					BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: generateAppServerSelectorLabels(),
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &serviceMonitor, r.Scheme); err != nil {
		return nil, err
	}

	return &serviceMonitor, nil
}

func (r *OLSConfigReconciler) generatePrometheusRule(cr *olsv1alpha1.OLSConfig) (*monv1.PrometheusRule, error) {
	metaLabels := generateAppServerSelectorLabels()
	metaLabels["app.kubernetes.io/component"] = "metrics"

	rule := monv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerPrometheusRuleName,
			Namespace: r.Options.Namespace,
			Labels:    metaLabels,
		},
		Spec: monv1.PrometheusRuleSpec{
			Groups: []monv1.RuleGroup{
				{
					Name: "ols.operations.rules",
					Rules: []monv1.Rule{
						{
							Record: "ols:rest_api_query_calls_total:2xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/query\",status_code=~\"2..\"})"),
							Labels: map[string]string{"status_code": "2xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:4xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/query\",status_code=~\"4..\"})"),
							Labels: map[string]string{"status_code": "4xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:5xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/query\",status_code=~\"5..\"})"),
							Labels: map[string]string{"status_code": "5xx"},
						},
						{
							Record: "ols:provider_model_configuration",
							Expr:   intstr.FromString("max by (provider,model) (ols_provider_model_configuration)"),
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rule, r.Scheme); err != nil {
		return nil, err
	}

	return &rule, nil
}

func (r *OLSConfigReconciler) getClusterVersion(ctx context.Context) (string, string, error) {
	key := client.ObjectKey{Name: "version"}
	clusterVersion := &configv1.ClusterVersion{}
	if err := r.Get(ctx, key, clusterVersion); err != nil {
		return "", "", err
	}
	versions := strings.Split(clusterVersion.Status.Desired.Version, ".")
	if len(versions) < 2 {
		return "", "", fmt.Errorf("failed to parse cluster version: %s", clusterVersion.Status.Desired.Version)
	}
	return versions[0], versions[1], nil
}

func getQueryFilters(cr *olsv1alpha1.OLSConfig) []QueryFilters {
	if cr.Spec.OLSConfig.QueryFilters == nil {
		return nil
	}

	filters := []QueryFilters{}
	for _, filter := range cr.Spec.OLSConfig.QueryFilters {
		filters = append(filters, QueryFilters{
			Name:        filter.Name,
			Pattern:     filter.Pattern,
			ReplaceWith: filter.ReplaceWith,
		})
	}
	return filters
}

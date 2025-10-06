package controller

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (r *OLSConfigReconciler) checkLLMCredentials(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	for _, provider := range cr.Spec.LLMConfig.Providers {
		if provider.CredentialsSecretRef.Name == "" {
			return fmt.Errorf("provider %s missing credentials secret", provider.Name)
		}
		secret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Name: provider.CredentialsSecretRef.Name, Namespace: r.Options.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("LLM provider %s credential secret %s not found", provider.Name, provider.CredentialsSecretRef.Name)
			}
			return fmt.Errorf("failed to get LLM provider %s credential secret %s: %w", provider.Name, provider.CredentialsSecretRef.Name, err)
		}
		if provider.Type == AzureOpenAIType {
			// Azure OpenAI secret must contain "apitoken" or 3 keys named "client_id", "tenant_id", "client_secret"
			if _, ok := secret.Data["apitoken"]; ok {
				continue
			}
			for _, key := range []string{"client_id", "tenant_id", "client_secret"} {
				if _, ok := secret.Data[key]; !ok {
					return fmt.Errorf("LLM provider %s credential secret %s missing key '%s'", provider.Name, provider.CredentialsSecretRef.Name, key)
				}
			}
		} else {
			// Other providers (e.g. WatsonX, OpenAI) must contain a key named "apikey"
			if _, ok := secret.Data["apitoken"]; !ok {
				return fmt.Errorf("LLM provider %s credential secret %s missing key 'apitoken'", provider.Name, provider.CredentialsSecretRef.Name)
			}
		}
	}
	return nil
}

func (r *OLSConfigReconciler) postgresCacheConfig(cr *olsv1alpha1.OLSConfig) PostgresCacheConfig {
	postgresSecretName := PostgresSecretName
	postgresConfig := cr.Spec.OLSConfig.ConversationCache.Postgres
	if postgresConfig.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}
	postgresPasswordPath := path.Join(CredentialsMountRoot, postgresSecretName, OLSComponentPasswordFileName)
	return PostgresCacheConfig{
		Host:         strings.Join([]string{PostgresServiceName, r.Options.Namespace, "svc"}, "."),
		Port:         PostgresServicePort,
		User:         PostgresDefaultUser,
		DbName:       PostgresDefaultDbName,
		PasswordPath: postgresPasswordPath,
		SSLMode:      PostgresDefaultSSLMode,
		CACertPath:   path.Join(OLSAppCertsMountRoot, PostgresCertsSecretName, PostgresCAVolume, "service-ca.crt"),
	}
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
				Name:       provider.Name,
				Type:       provider.Type,
				Models:     modelConfigs,
				APIVersion: provider.APIVersion,
				AzureOpenAIConfig: &AzureOpenAIConfig{
					URL:                 provider.URL,
					CredentialsPath:     credentialPath,
					AzureDeploymentName: provider.AzureDeploymentName,
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

	conversationCache := ConversationCacheConfig{
		Type:     string(OLSDefaultCacheType),
		Postgres: r.postgresCacheConfig(cr),
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

	var proxyConfig *ProxyConfig
	if cr.Spec.OLSConfig.ProxyConfig != nil {
		proxyConfig = &ProxyConfig{
			ProxyURL:        cr.Spec.OLSConfig.ProxyConfig.ProxyURL,
			ProxyCACertPath: "",
		}
		if cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef != nil && cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name != "" {
			err := r.validateCertificateInConfigMap(cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name, ProxyCACertFileName)
			if err != nil {
				return nil, fmt.Errorf("failed to validate proxy CA certificate %s: %w", cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name, err)
			}
			proxyConfig.ProxyCACertPath = path.Join(OLSAppCertsMountRoot, ProxyCACertVolumeName, ProxyCACertFileName)
		}
	}

	referenceIndexes := []ReferenceIndex{}
	// OLS-1823: prioritize BYOK content by listing it ahead of the OCP docs
	// Custom reference document is optional
	for i, index := range cr.Spec.OLSConfig.RAG {
		referenceIndex := ReferenceIndex{
			ProductDocsIndexPath: filepath.Join(RAGVolumeMountPath, fmt.Sprintf("rag-%d", i)),
			ProductDocsIndexId:   index.IndexID,
		}
		referenceIndexes = append(referenceIndexes, referenceIndex)
	}
	if !cr.Spec.OLSConfig.ByokRAGOnly {
		ocpReferenceIndex := ReferenceIndex{
			ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/" + major + "." + minor,
			ProductDocsIndexId:   "ocp-product-docs-" + major + "_" + minor,
		}
		referenceIndexes = append(referenceIndexes, ocpReferenceIndex)
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
			Indexes: referenceIndexes,
			// all RAGs use the same embedding model
			EmbeddingsModelPath: "/app-root/embeddings_model",
		},
		UserDataCollection: UserDataCollectionConfig{
			FeedbackDisabled:    cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled || !dataCollectorEnabled,
			FeedbackStorage:     "/app-root/ols-user-data/feedback",
			TranscriptsDisabled: cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled || !dataCollectorEnabled,
			TranscriptsStorage:  "/app-root/ols-user-data/transcripts",
		},
		ProxyConfig: proxyConfig,
	}

	if cr.Spec.OLSConfig.QuotaHandlersConfig != nil {
		olsConfig.QuotaHandlersConfig = &QuotaHandlersConfig{
			Storage: r.postgresCacheConfig(cr),
			Scheduler: SchedulerConfig{
				Period: 300,
			},
			LimitersConfig:     []LimiterConfig{},
			EnableTokenHistory: cr.Spec.OLSConfig.QuotaHandlersConfig.EnableTokenHistory,
		}
		for _, lc := range cr.Spec.OLSConfig.QuotaHandlersConfig.LimitersConfig {
			olsConfig.QuotaHandlersConfig.LimitersConfig = append(
				olsConfig.QuotaHandlersConfig.LimitersConfig,
				LimiterConfig{
					Name:          lc.Name,
					Type:          lc.Type,
					InitialQuota:  lc.InitialQuota,
					QuotaIncrease: lc.QuotaIncrease,
					Period:        lc.Period,
				},
			)
		}
	}

	// Append kube-root-ca.crt certificates
	extraCAs, err := r.addAdditionalCAFileNames(ctx, &corev1.LocalObjectReference{Name: "kube-root-ca.crt"}, AppAdditionalCACertDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate additional certs from kube-root-ca.crt, additional CA error: %w", err)
	}
	olsConfig.ExtraCAs = extraCAs

	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		extraCAs, err := r.addAdditionalCAFileNames(ctx, cr.Spec.OLSConfig.AdditionalCAConfigMapRef, UserCACertDir)
		if err != nil {
			return nil, fmt.Errorf("failed to generate OLS config file, additional CA error: %w", err)
		}
		olsConfig.ExtraCAs = append(olsConfig.ExtraCAs, extraCAs...)
	}
	olsConfig.CertificateDirectory = path.Join(OLSAppCertsMountRoot, CertBundleDir)

	if queryFilters := getQueryFilters(cr); queryFilters != nil {
		olsConfig.QueryFilters = queryFilters
	}

	appSrvConfigFile := AppSrvConfigFile{
		LLMProviders: providerConfigs,
		OLSConfig:    olsConfig,
	}
	if dataCollectorEnabled {
		appSrvConfigFile.UserDataCollectorConfig = UserDataCollectorConfig{
			DataStorage: "/app-root/ols-user-data",
			LogLevel:    cr.Spec.OLSDataCollectorConfig.LogLevel,
		}
	}

	if cr.Spec.OLSConfig.IntrospectionEnabled {
		appSrvConfigFile.MCPServers = []MCPServerConfig{
			{
				Name:      "openshift",
				Transport: StreamableHTTP,
				StreamableHTTP: &StreamableHTTPTransportConfig{
					URL:            fmt.Sprintf(OpenShiftMCPServerURL, OpenShiftMCPServerPort),
					Timeout:        OpenShiftMCPServerTimeout,
					SSEReadTimeout: OpenShiftMCPServerHTTPReadTimeout,
				},
			},
		}
	}

	if cr.Spec.FeatureGates != nil && slices.Contains(cr.Spec.FeatureGates, FeatureGateMCPServer) {
		mcpServers := generateMCPServerConfigs(cr)
		if appSrvConfigFile.MCPServers == nil {
			appSrvConfigFile.MCPServers = mcpServers
		} else {
			appSrvConfigFile.MCPServers = append(appSrvConfigFile.MCPServers, mcpServers...)
		}
	}

	configFileBytes, err := yaml.Marshal(appSrvConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file %w", err)
	}

	postgresConfigFileBytes, err := yaml.Marshal(conversationCache.Postgres)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS postgres config bytes %w", err)
	}

	configFileHash, err := hashBytes(configFileBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file hash %w", err)
	}

	postgresConfigHash, err := hashBytes(postgresConfigFileBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS postgres config hash %w", err)
	}

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSConfigCmName,
			Namespace: r.Options.Namespace,
			Labels:    generateAppServerSelectorLabels(),
			Annotations: map[string]string{
				OLSConfigHashKey:      configFileHash,
				PostgresConfigHashKey: postgresConfigHash,
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

func (r *OLSConfigReconciler) addAdditionalCAFileNames(ctx context.Context, cr *corev1.LocalObjectReference, certDirectory string) ([]string, error) {
	// get data from the referenced configmap
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cr.Name, Namespace: r.Options.Namespace}, cm)
	if err != nil {
		return nil, fmt.Errorf("failed to get additional CA configmap %s/%s: %v", r.Options.Namespace, cr.Name, err)
	}

	filenames := []string{}
	for key, caStr := range cm.Data {
		err = validateCertificateFormat([]byte(caStr))
		if err != nil {
			return nil, fmt.Errorf("failed to validate additional CA certificate %s: %v", key, err)
		}
		filenames = append(filenames, key)
	}

	extraCAs := make([]string, len(filenames))
	for i, caFileName := range filenames {
		extraCAs[i] = path.Join(OLSAppCertsMountRoot, certDirectory, caFileName)
	}

	return extraCAs, nil
}

func (r *OLSConfigReconciler) validateCertificateInConfigMap(cmName, fileName string) error {

	cm := &corev1.ConfigMap{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: cmName, Namespace: r.Options.Namespace}, cm)
	if err != nil {
		return fmt.Errorf("failed to get certificate configmap %s/%s: %v", r.Options.Namespace, cmName, err)
	}

	caStr, ok := cm.Data[fileName]
	if !ok {
		return fmt.Errorf("failed to find certificate %s in configmap %s", fileName, cmName)
	}

	err = validateCertificateFormat([]byte(caStr))
	if err != nil {
		return fmt.Errorf("failed to validate certificate %s: %v", fileName, err)
	}
	return nil
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
					Authorization: &monv1.SafeAuthorization{
						Type: "Bearer",
						Credentials: &corev1.SecretKeySelector{
							Key: "token",
							LocalObjectReference: corev1.LocalObjectReference{
								Name: MetricsReaderServiceAccountTokenSecretName,
							},
						},
					},
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
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"2..\"})"),
							Labels: map[string]string{"status_code": "2xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:4xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"4..\"})"),
							Labels: map[string]string{"status_code": "4xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:5xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"5..\"})"),
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

func (r *OLSConfigReconciler) generateAppServerNetworkPolicy(cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerNetworkPolicyName,
			Namespace: r.Options.Namespace,
			Labels:    generateAppServerSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: generateAppServerSelectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// allow prometheus to scrape metrics
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app.kubernetes.io/name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"prometheus"},
									},
									{
										Key:      "prometheus",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"k8s"},
									},
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-monitoring",
								},
							},
						},
					},

					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(OLSAppServerContainerPort)}[0],
						},
					},
				},
				{
					// allow the console to access the API
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "console",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-console",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(OLSAppServerContainerPort)}[0],
						},
					},
				},
				{
					// allow ingress controller to access the API
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "ingress",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(OLSAppServerContainerPort)}[0],
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &np, r.Scheme); err != nil {
		return nil, err
	}

	return &np, nil
}

func (r *OLSConfigReconciler) generateMetricsReaderSecret(cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MetricsReaderServiceAccountTokenSecretName,
			Namespace: r.Options.Namespace,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": MetricsReaderServiceAccountName,
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":      "service-account-token",
				"app.kubernetes.io/component": "metrics",
				"app.kubernetes.io/part-of":   "lightspeed-operator",
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.Scheme); err != nil {
		return nil, err
	}

	return secret, nil
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

const (
	SSEField int = iota
	StreamableHTTPField
)

func generateMCPServerConfigs(cr *olsv1alpha1.OLSConfig) []MCPServerConfig {
	if cr.Spec.MCPServers == nil {
		return nil
	}

	servers := []MCPServerConfig{}
	for _, server := range cr.Spec.MCPServers {
		servers = append(servers, MCPServerConfig{
			Name:           server.Name,
			Transport:      getMCPTransport(&server),
			SSE:            generateMCPStreamableHTTPTransportConfig(&server, SSEField),
			StreamableHTTP: generateMCPStreamableHTTPTransportConfig(&server, StreamableHTTPField),
		})
	}
	return servers
}

func generateMCPStreamableHTTPTransportConfig(server *olsv1alpha1.MCPServer, field int) *StreamableHTTPTransportConfig {
	if server == nil || server.StreamableHTTP == nil {
		return nil
	}

	switch field {
	case SSEField:
		if !server.StreamableHTTP.EnableSSE {
			return nil
		}
	case StreamableHTTPField:
		if server.StreamableHTTP.EnableSSE {
			return nil
		}
	default:
		return nil
	}

	return &StreamableHTTPTransportConfig{
		URL:            server.StreamableHTTP.URL,
		Timeout:        server.StreamableHTTP.Timeout,
		SSEReadTimeout: server.StreamableHTTP.SSEReadTimeout,
		Headers:        server.StreamableHTTP.Headers,
	}
}

func getMCPTransport(server *olsv1alpha1.MCPServer) MCPTransport {
	if server == nil || server.StreamableHTTP == nil {
		return ""
	}
	if server.StreamableHTTP.EnableSSE {
		return SSE
	}
	return StreamableHTTP
}

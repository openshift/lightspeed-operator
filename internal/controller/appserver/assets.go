package appserver

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func GenerateServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerServiceAccountName,
			Namespace: r.GetNamespace(),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &sa, r.GetScheme()); err != nil {
		return nil, err
	}

	return &sa, nil
}

func GenerateSARClusterRole(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSAppServerSARRoleName,
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

	if err := controllerutil.SetControllerReference(cr, &role, r.GetScheme()); err != nil {
		return nil, err
	}

	return &role, nil
}

func generateSARClusterRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRoleBinding, error) {
	rb := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSAppServerSARRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.OLSAppServerServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OLSAppServerSARRoleName,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rb, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rb, nil
}

func postgresCacheConfig(r reconciler.Reconciler, _ *olsv1alpha1.OLSConfig) utils.PostgresCacheConfig {
	postgresPasswordPath := path.Join(utils.CredentialsMountRoot, utils.PostgresSecretName, utils.OLSComponentPasswordFileName)
	return utils.PostgresCacheConfig{
		Host:         strings.Join([]string{utils.PostgresServiceName, r.GetNamespace(), "svc"}, "."),
		Port:         utils.PostgresServicePort,
		User:         utils.PostgresDefaultUser,
		DbName:       utils.PostgresDefaultDbName,
		PasswordPath: postgresPasswordPath,
		SSLMode:      utils.PostgresDefaultSSLMode,
		CACertPath:   path.Join(utils.OLSAppCertsMountRoot, "postgres-ca", "service-ca.crt"),
	}
}

func GenerateOLSConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	providerConfigs := []utils.ProviderConfig{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialPath := path.Join(utils.APIKeyMountRoot, provider.CredentialsSecretRef.Name)
		modelConfigs := []utils.ModelConfig{}
		for _, model := range provider.Models {
			modelConfig := utils.ModelConfig{
				Name: model.Name,
				URL:  model.URL,
				Parameters: utils.ModelParameters{
					MaxTokensForResponse: model.Parameters.MaxTokensForResponse,
				},
				ContextWindowSize: model.ContextWindowSize,
			}
			modelConfigs = append(modelConfigs, modelConfig)
		}
		var providerConfig utils.ProviderConfig
		if provider.Type == utils.AzureOpenAIType {
			providerConfig = utils.ProviderConfig{
				Name:       provider.Name,
				Type:       provider.Type,
				Models:     modelConfigs,
				APIVersion: provider.APIVersion,
				AzureOpenAIConfig: &utils.AzureOpenAIConfig{
					URL:                 provider.URL,
					CredentialsPath:     credentialPath,
					AzureDeploymentName: provider.AzureDeploymentName,
				},
			}
		} else {
			providerConfig = utils.ProviderConfig{
				Name:            provider.Name,
				Type:            provider.Type,
				URL:             provider.URL,
				CredentialsPath: credentialPath,
				Models:          modelConfigs,
				WatsonProjectID: provider.WatsonProjectID,
			}
		}

		if provider.Type == utils.FakeProviderType {
			providerConfig.FakeProviderConfig = &utils.FakeProviderConfig{
				URL:         "http://example.com",
				Response:    "This is a preconfigured fake response.",
				Chunks:      30,
				Sleep:       0.1,
				Stream:      false,
				MCPToolCall: provider.FakeProviderMCPToolCall,
			}
		}
		providerConfigs = append(providerConfigs, providerConfig)
	}

	conversationCache := utils.ConversationCacheConfig{
		Type:     string(utils.OLSDefaultCacheType),
		Postgres: postgresCacheConfig(r, cr),
	}

	// We want to disable the data collector if the user has explicitly disabled it
	// or if the data collector is not enabled in the cluster with pull secret

	dataCollectorEnabled, _ := dataCollectorEnabled(r, cr)

	// TLS config always uses /etc/certs/lightspeed-tls/ path
	// regardless of whether it's service-ca generated or user-provided
	tlsConfig := utils.TLSConfig{
		TLSCertificatePath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls", "tls.crt"),
		TLSKeyPath:         path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls", "tls.key"),
	}

	var proxyConfig *utils.ProxyConfig
	if cr.Spec.OLSConfig.ProxyConfig != nil {
		proxyConfig = &utils.ProxyConfig{
			ProxyURL:        cr.Spec.OLSConfig.ProxyConfig.ProxyURL,
			ProxyCACertPath: "",
		}
		if cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef != nil && cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name != "" {
			err := validateCertificateInConfigMap(r, ctx, cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name, utils.ProxyCACertFileName)
			if err != nil {
				return nil, fmt.Errorf("failed to validate proxy CA certificate %s: %w", cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name, err)
			}
			proxyConfig.ProxyCACertPath = path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName, utils.ProxyCACertFileName)
		}
	}

	referenceIndexes := []utils.ReferenceIndex{}
	// OLS-1823: prioritize BYOK content by listing it ahead of the OCP docs
	// Custom reference document is optional
	for i, index := range cr.Spec.OLSConfig.RAG {
		referenceIndex := utils.ReferenceIndex{
			ProductDocsIndexPath: filepath.Join(utils.RAGVolumeMountPath, fmt.Sprintf("rag-%d", i)),
			ProductDocsIndexId:   index.IndexID,
			ProductDocsOrigin:    index.Image,
		}
		referenceIndexes = append(referenceIndexes, referenceIndex)
	}
	if !cr.Spec.OLSConfig.ByokRAGOnly {
		ocpReferenceIndex := utils.ReferenceIndex{
			ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/" + r.GetOpenShiftMajor() + "." + r.GetOpenshiftMinor(),
			ProductDocsIndexId:   "ocp-product-docs-" + r.GetOpenShiftMajor() + "_" + r.GetOpenshiftMinor(),
			ProductDocsOrigin:    "Red Hat OpenShift " + r.GetOpenShiftMajor() + "." + r.GetOpenshiftMinor() + " documentation",
		}
		referenceIndexes = append(referenceIndexes, ocpReferenceIndex)
	}

	olsConfig := utils.OLSConfig{
		DefaultModel:    cr.Spec.OLSConfig.DefaultModel,
		DefaultProvider: cr.Spec.OLSConfig.DefaultProvider,
		Logging: utils.LoggingConfig{
			AppLogLevel:     string(cr.Spec.OLSConfig.LogLevel),
			LibLogLevel:     string(cr.Spec.OLSConfig.LogLevel),
			UvicornLogLevel: string(cr.Spec.OLSConfig.LogLevel),
		},
		ConversationCache: conversationCache,
		TLSConfig:         tlsConfig,
		ReferenceContent: utils.ReferenceContent{
			Indexes: referenceIndexes,
			// all RAGs use the same embedding model
			EmbeddingsModelPath: "/app-root/embeddings_model",
		},
		UserDataCollection: utils.UserDataCollectionConfig{
			FeedbackDisabled:    cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled || !dataCollectorEnabled,
			FeedbackStorage:     "/app-root/ols-user-data/feedback",
			TranscriptsDisabled: cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled || !dataCollectorEnabled,
			TranscriptsStorage:  "/app-root/ols-user-data/transcripts",
		},
		ProxyConfig: proxyConfig,
	}

	if cr.Spec.OLSConfig.QuotaHandlersConfig != nil {
		olsConfig.QuotaHandlersConfig = &utils.QuotaHandlersConfig{
			Storage: postgresCacheConfig(r, cr),
			Scheduler: utils.SchedulerConfig{
				Period: 300,
			},
			LimitersConfig:     []utils.LimiterConfig{},
			EnableTokenHistory: cr.Spec.OLSConfig.QuotaHandlersConfig.EnableTokenHistory,
		}
		for _, lc := range cr.Spec.OLSConfig.QuotaHandlersConfig.LimitersConfig {
			olsConfig.QuotaHandlersConfig.LimitersConfig = append(
				olsConfig.QuotaHandlersConfig.LimitersConfig,
				utils.LimiterConfig{
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
	extraCAs, err := addAdditionalCAFileNames(r, ctx, &corev1.LocalObjectReference{Name: "kube-root-ca.crt"}, utils.AppAdditionalCACertDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate additional certs from kube-root-ca.crt, additional CA error: %w", err)
	}
	olsConfig.ExtraCAs = extraCAs

	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		extraCAs, err := addAdditionalCAFileNames(r, ctx, cr.Spec.OLSConfig.AdditionalCAConfigMapRef, utils.UserCACertDir)
		if err != nil {
			return nil, fmt.Errorf("failed to generate OLS config file, additional CA error: %w", err)
		}
		olsConfig.ExtraCAs = append(olsConfig.ExtraCAs, extraCAs...)
	}
	olsConfig.CertificateDirectory = path.Join(utils.OLSAppCertsMountRoot, utils.CertBundleVolumeName)

	if queryFilters := getQueryFilters(cr); queryFilters != nil {
		olsConfig.QueryFilters = queryFilters
	}

	if cr.Spec.OLSConfig.QuerySystemPrompt != "" {
		olsConfig.SystemPromptPath = path.Join(utils.OLSConfigMountRoot, utils.OLSSystemPromptFileName)
	}

	appSrvConfigFile := utils.AppSrvConfigFile{
		LLMProviders: providerConfigs,
		OLSConfig:    olsConfig,
	}
	if dataCollectorEnabled {
		appSrvConfigFile.UserDataCollectorConfig = utils.UserDataCollectorConfig{
			DataStorage: "/app-root/ols-user-data",
			LogLevel:    string(cr.Spec.OLSDataCollectorConfig.LogLevel),
		}
	}

	// Generate MCP servers config (includes both introspection + user-defined servers)
	mcpServers, err := generateMCPServerConfigs(r, cr)
	if err != nil {
		return nil, err
	}
	if len(mcpServers) > 0 {
		appSrvConfigFile.MCPServers = mcpServers
	}

	// Only add tool filtering if feature gate is enabled, config is specified, and there are MCP servers to filter
	if cr.Spec.FeatureGates != nil && slices.Contains(cr.Spec.FeatureGates, utils.FeatureGateToolFiltering) {
		if cr.Spec.OLSConfig.ToolFilteringConfig != nil {
			if len(mcpServers) > 0 {
				// Apply defaults for zero values (happens when user specifies toolFilteringConfig: {})
				cfg := cr.Spec.OLSConfig.ToolFilteringConfig
				alpha, topK, threshold := cfg.Alpha, cfg.TopK, cfg.Threshold
				if alpha == 0.0 {
					alpha = 0.8
				}
				if topK == 0 {
					topK = 10
				}
				if threshold == 0.0 {
					threshold = 0.01
				}

				appSrvConfigFile.OLSConfig.ToolFiltering = &utils.ToolFilteringConfig{
					Alpha:     alpha,
					TopK:      topK,
					Threshold: threshold,
				}
			} else {
				r.GetLogger().Info(
					"ToolFilteringConfig specified but no MCP servers configured. Tool filtering will be disabled.",
					"IntrospectionEnabled", cr.Spec.OLSConfig.IntrospectionEnabled,
					"MCPServersCount", len(cr.Spec.MCPServers),
				)
			}
		}
	}

	configFileBytes, err := yaml.Marshal(appSrvConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file %w", err)
	}

	data := map[string]string{
		utils.OLSConfigFilename: string(configFileBytes),
	}

	if cr.Spec.OLSConfig.QuerySystemPrompt != "" {
		data[utils.OLSSystemPromptFileName] = cr.Spec.OLSConfig.QuerySystemPrompt
	}

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: data,
	}

	if err := controllerutil.SetControllerReference(cr, &cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return &cm, nil
}

func generateExporterConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	serviceID := utils.ServiceIDOLS
	if cr.Labels != nil {
		if _, hasRHOSLightspeedLabel := cr.Labels[utils.RHOSOLightspeedOwnerIDLabel]; hasRHOSLightspeedLabel {
			serviceID = utils.ServiceIDRHOSO
		}
	}

	// Collection interval is set to 300 seconds in production (5 minutes)
	exporterConfigContent := fmt.Sprintf(`service_id: "%s"
ingress_server_url: "https://console.redhat.com/api/ingress/v1/upload"
allowed_subdirs:
 - feedback
 - transcripts
 - config_status
# Collection settings
collection_interval: 300
cleanup_after_send: true
ingress_connection_timeout: 30`, serviceID)

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ExporterConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			utils.ExporterConfigFilename: exporterConfigContent,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return &cm, nil
}

func addAdditionalCAFileNames(r reconciler.Reconciler, ctx context.Context, cr *corev1.LocalObjectReference, certDirectory string) ([]string, error) {
	// get data from the referenced configmap
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cr.Name, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		return nil, fmt.Errorf("failed to get additional CA configmap %s/%s: %v", r.GetNamespace(), cr.Name, err)
	}

	filenames := []string{}
	for key, caStr := range cm.Data {
		err = utils.ValidateCertificateFormat([]byte(caStr))
		if err != nil {
			return nil, fmt.Errorf("failed to validate additional CA certificate %s: %v", key, err)
		}
		filenames = append(filenames, key)
	}

	extraCAs := make([]string, len(filenames))
	for i, caFileName := range filenames {
		extraCAs[i] = path.Join(utils.OLSAppCertsMountRoot, certDirectory, caFileName)
	}

	return extraCAs, nil
}

func validateCertificateInConfigMap(r reconciler.Reconciler, ctx context.Context, cmName, fileName string) error {

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cmName, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		return fmt.Errorf("failed to get certificate configmap %s/%s: %v", r.GetNamespace(), cmName, err)
	}

	caStr, ok := cm.Data[fileName]
	if !ok {
		return fmt.Errorf("failed to find certificate %s in configmap %s", fileName, cmName)
	}

	err = utils.ValidateCertificateFormat([]byte(caStr))
	if err != nil {
		return fmt.Errorf("failed to validate certificate %s: %v", fileName, err)
	}
	return nil
}

func GenerateService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	annotations := map[string]string{}

	// Let service-ca operator generate a TLS certificate if the user does not provide their own
	if cr.Spec.OLSConfig.TLSConfig == nil || cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name == "" {
		annotations[utils.ServingCertSecretAnnotationKey] = utils.OLSCertsSecretName
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        utils.OLSAppServerServiceName,
			Namespace:   r.GetNamespace(),
			Labels:      utils.GenerateAppServerSelectorLabels(),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					Port:       utils.OLSAppServerServicePort,
					TargetPort: intstr.Parse("https"),
				},
			},
			Selector: utils.GenerateAppServerSelectorLabels(),
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GenerateServiceMonitor(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
	metaLabels := utils.GenerateAppServerSelectorLabels()
	metaLabels["monitoring.openshift.io/collection-profile"] = "full"
	metaLabels["app.kubernetes.io/component"] = "metrics"
	metaLabels["openshift.io/user-monitoring"] = "false"

	valFalse := false
	serverName := strings.Join([]string{utils.OLSAppServerServiceName, r.GetNamespace(), "svc"}, ".")
	var schemeHTTPS monv1.Scheme = "https"

	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AppServerServiceMonitorName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "https",
					Path:     utils.AppServerMetricsPath,
					Interval: "30s",
					Scheme:   &schemeHTTPS,
					HTTPConfigWithProxyAndTLSFiles: monv1.HTTPConfigWithProxyAndTLSFiles{
						HTTPConfigWithTLSFiles: monv1.HTTPConfigWithTLSFiles{
							TLSConfig: &monv1.TLSConfig{
								TLSFilesConfig: monv1.TLSFilesConfig{
									CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
									CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
									KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
								},
								SafeTLSConfig: monv1.SafeTLSConfig{
									InsecureSkipVerify: &valFalse,
									ServerName:         &serverName,
								},
							},
							HTTPConfigWithoutTLS: monv1.HTTPConfigWithoutTLS{
								Authorization: &monv1.SafeAuthorization{
									Type: "Bearer",
									Credentials: &corev1.SecretKeySelector{
										Key: "token",
										LocalObjectReference: corev1.LocalObjectReference{
											Name: utils.MetricsReaderServiceAccountTokenSecretName,
										},
									},
								},
							},
						},
					},
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &serviceMonitor, r.GetScheme()); err != nil {
		return nil, err
	}

	return &serviceMonitor, nil
}

func GeneratePrometheusRule(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.PrometheusRule, error) {
	metaLabels := utils.GenerateAppServerSelectorLabels()
	metaLabels["app.kubernetes.io/component"] = "metrics"

	rule := monv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AppServerPrometheusRuleName,
			Namespace: r.GetNamespace(),
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

	if err := controllerutil.SetControllerReference(cr, &rule, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rule, nil
}

func GenerateAppServerNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
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
									"kubernetes.io/metadata.name": utils.ClientCACmNamespace,
								},
							},
						},
					},

					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
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
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
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
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
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

	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, err
	}

	return &np, nil
}

func GenerateMetricsReaderSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.MetricsReaderServiceAccountTokenSecretName,
			Namespace: r.GetNamespace(),
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": utils.MetricsReaderServiceAccountName,
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":      "service-account-token",
				"app.kubernetes.io/component": "metrics",
				"app.kubernetes.io/part-of":   "lightspeed-operator",
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return secret, nil
}

func getQueryFilters(cr *olsv1alpha1.OLSConfig) []utils.QueryFilters {
	if cr.Spec.OLSConfig.QueryFilters == nil {
		return nil
	}

	filters := []utils.QueryFilters{}
	for _, filter := range cr.Spec.OLSConfig.QueryFilters {
		filters = append(filters, utils.QueryFilters{
			Name:        filter.Name,
			Pattern:     filter.Pattern,
			ReplaceWith: filter.ReplaceWith,
		})
	}
	return filters
}

func generateMCPServerConfigs(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) ([]utils.MCPServerConfig, error) {
	servers := []utils.MCPServerConfig{}

	// Add OpenShift MCP server if introspection is enabled
	if cr.Spec.OLSConfig.IntrospectionEnabled {
		servers = append(servers, utils.MCPServerConfig{
			Name:    "openshift",
			URL:     fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort),
			Timeout: utils.OpenShiftMCPServerTimeout,
			Headers: map[string]string{
				utils.K8S_AUTH_HEADER: utils.KUBERNETES_PLACEHOLDER,
			},
		})
	}

	// Add user-defined MCP servers
	if cr.Spec.FeatureGates != nil && slices.Contains(cr.Spec.FeatureGates, utils.FeatureGateMCPServer) && cr.Spec.MCPServers != nil {
		for _, server := range cr.Spec.MCPServers {
			// Build MCP server config
			mcpServer := utils.MCPServerConfig{
				Name: server.Name,
				URL:  server.URL,
			}

			// Add timeout if specified (default is handled by lightspeed-service)
			if server.Timeout > 0 {
				mcpServer.Timeout = server.Timeout
			}

			// Add authorization headers if configured
			if len(server.Headers) > 0 {
				headers := make(map[string]string)
				invalidServer := false
				for _, header := range server.Headers {
					if invalidServer {
						break
					}
					headerName := header.Name
					var headerValue string

					// Determine header value based on discriminator type
					switch header.ValueFrom.Type {
					case olsv1alpha1.MCPHeaderSourceTypeKubernetes:
						headerValue = utils.KUBERNETES_PLACEHOLDER
					case olsv1alpha1.MCPHeaderSourceTypeClient:
						headerValue = utils.CLIENT_PLACEHOLDER
					case olsv1alpha1.MCPHeaderSourceTypeSecret:
						if header.ValueFrom.SecretRef == nil || header.ValueFrom.SecretRef.Name == "" {
							r.GetLogger().Error(
								fmt.Errorf("missing secretRef for type 'secret'"),
								"Skipping MCP server: type is 'secret' but secretRef is not set",
								"server", server.Name,
								"header", headerName,
							)
							invalidServer = true
							continue
						}
						// Use consistent path structure: /etc/mcp/headers/<secretName>/header
						headerValue = path.Join(utils.MCPHeadersMountRoot, header.ValueFrom.SecretRef.Name, utils.MCPSECRETDATAPATH)
					default:
						// This should never happen due to enum validation
						r.GetLogger().Error(
							fmt.Errorf("invalid MCP header type: %s", header.ValueFrom.Type),
							"Skipping MCP server due to invalid header type",
							"server", server.Name,
							"header", headerName,
							"type", header.ValueFrom.Type,
						)
						invalidServer = true
						continue
					}

					headers[headerName] = headerValue
				}

				// Skip this server if any header was invalid
				if invalidServer {
					continue
				}

				if len(headers) > 0 {
					mcpServer.Headers = headers
				}
			}

			servers = append(servers, mcpServer)
		}
	}

	return servers, nil
}

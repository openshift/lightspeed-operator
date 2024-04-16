package controller

import (
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func (r *OLSConfigReconciler) generateOLSConfigMap(cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	providerConfigs := []ProviderConfig{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name, LLMApiTokenFileName)
		modelConfigs := []ModelConfig{}
		for _, model := range provider.Models {
			modelConfig := ModelConfig{
				Name: model.Name,
				URL:  model.URL,
			}
			modelConfigs = append(modelConfigs, modelConfig)
		}

		providerConfig := ProviderConfig{
			Name:            provider.Name,
			URL:             provider.URL,
			CredentialsPath: credentialPath,
			Models:          modelConfigs,
		}
		providerConfigs = append(providerConfigs, providerConfig)
	}

	postgresSharedBuffers := intstr.FromString(PostgresSharedBuffers)
	postgresMaxConnections := PostgresMaxConnections
	postgresSecretName := PostgresSecretName
	postgresConfig := cr.Spec.OLSConfig.ConversationCache.Postgres
	if postgresConfig.SharedBuffers != nil && postgresConfig.SharedBuffers.String() != "" {
		postgresSharedBuffers = *cr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers
	}
	if postgresConfig.MaxConnections != (-1) {
		postgresMaxConnections = cr.Spec.OLSConfig.ConversationCache.Postgres.MaxConnections
	}
	if postgresConfig.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}
	postgresPasswordPath := path.Join(CredentialsMountRoot, postgresSecretName, OLSComponentPasswordFileName)
	conversationCache := ConversationCacheConfig{
		Type: string(OLSDefaultCacheType),
		Postgres: PostgresCacheConfig{
			Host:           strings.Join([]string{PostgresServiceName, r.Options.Namespace, "svc"}, "."),
			Port:           PostgresServicePort,
			User:           PostgresDefaultUser,
			DbName:         PostgresDefaultDbName,
			SharedBuffers:  &postgresSharedBuffers,
			MaxConnections: postgresMaxConnections,
			PasswordPath:   postgresPasswordPath,
			SSLMode:        PostgresDefaultSSLMode,
			CACertPath:     path.Join(OLSAppCertsMountRoot, PostgresCertsSecretName, PostgresCAVolume, "service-ca.crt"),
		},
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
		TLSConfig: TLSConfig{
			TLSCertificatePath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.crt"),
			TLSKeyPath:         path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.key"),
		},
		ReferenceContent: ReferenceContent{
			ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/4.15",
			ProductDocsIndexId:   "ocp-product-docs-4_15",
			EmbeddingsModelPath:  "/app-root/embeddings_model",
		},
	}

	if queryFilters := getQueryFilters(cr); queryFilters != nil {
		olsConfig.QueryFilters = queryFilters
	}

	devConfig := DevConfig{
		DisableAuth: cr.Spec.OLSConfig.DisableAuth,
	}

	appSrvConfigFile := AppSrvConfigFile{
		LLMProviders: providerConfigs,
		OLSConfig:    olsConfig,
		DevConfig:    devConfig,
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

func (r *OLSConfigReconciler) generateService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerServiceName,
			Namespace: r.Options.Namespace,
			Labels:    generateAppServerSelectorLabels(),
			Annotations: map[string]string{
				ServingCertSecretAnnotationKey: OLSCertsSecretName,
			},
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

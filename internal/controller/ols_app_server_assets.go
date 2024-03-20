package controller

import (
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
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
	redisMaxMemory := intstr.FromString(RedisMaxMemory)
	redisMaxMemoryPolicy := RedisMaxMemoryPolicy
	redisSecretName := RedisSecretName
	redisConfig := cr.Spec.OLSConfig.ConversationCache.Redis
	if redisConfig.MaxMemory != nil && redisConfig.MaxMemory.String() != "" {
		redisMaxMemory = *cr.Spec.OLSConfig.ConversationCache.Redis.MaxMemory
	}
	if redisConfig.MaxMemoryPolicy != "" {
		redisMaxMemoryPolicy = cr.Spec.OLSConfig.ConversationCache.Redis.MaxMemoryPolicy
	}
	if redisConfig.CredentialsSecret != "" {
		redisSecretName = cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret
	}
	redisPasswordPath := path.Join(CredentialsMountRoot, redisSecretName, OLSComponentPasswordFileName)
	conversationCache := ConversationCacheConfig{
		Type: string(OLSDefaultCacheType),
		Redis: RedisCacheConfig{
			Host:            strings.Join([]string{RedisServiceName, r.Options.Namespace, "svc"}, "."),
			Port:            RedisServicePort,
			MaxMemory:       &redisMaxMemory,
			MaxMemoryPolicy: redisMaxMemoryPolicy,
			PasswordPath:    redisPasswordPath,
			CACertPath:      path.Join(OLSAppCertsMountRoot, RedisCertsSecretName, RedisCAVolume, "service-ca.crt"),
		},
	}

	olsConfig := OLSConfig{
		DefaultModel:    cr.Spec.OLSConfig.DefaultModel,
		DefaultProvider: cr.Spec.OLSConfig.DefaultProvider,
		Logging: LoggingConfig{
			AppLogLevel: cr.Spec.OLSConfig.LogLevel,
			LibLogLevel: cr.Spec.OLSConfig.LogLevel,
		},
		ConversationCache: conversationCache,
		TLSConfig: TLSConfig{
			TLSCertificatePath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.crt"),
			TLSKeyPath:         path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.key"),
		},
	}

	devConfig := DevConfig{
		DisableTLS: cr.Spec.OLSConfig.DisableTLS,
	}

	if isTLSDisabled(cr) {
		olsConfig.TLSConfig = TLSConfig{}
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

	redisConfigFileBytes, err := yaml.Marshal(conversationCache.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS redis config bytes %w", err)
	}

	configFileHash, err := hashBytes(configFileBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file hash %w", err)
	}

	redisConfigHash, err := hashBytes(redisConfigFileBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS redis config hash %w", err)
	}

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSConfigCmName,
			Namespace: r.Options.Namespace,
			Labels:    generateAppServerSelectorLabels(),
			Annotations: map[string]string{
				OLSConfigHashKey:   configFileHash,
				RedisConfigHashKey: redisConfigHash,
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

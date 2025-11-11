package utils

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"path"
	"strings"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

const TestCACert = `-----BEGIN CERTIFICATE-----
MIIEMDCCAxigAwIBAgIJANqb7HHzA7AZMA0GCSqGSIb3DQEBCwUAMIGkMQswCQYD
VQQGEwJQQTEPMA0GA1UECAwGUGFuYW1hMRQwEgYDVQQHDAtQYW5hbWEgQ2l0eTEk
MCIGA1UECgwbVHJ1c3RDb3IgU3lzdGVtcyBTLiBkZSBSLkwuMScwJQYDVQQLDB5U
cnVzdENvciBDZXJ0aWZpY2F0ZSBBdXRob3JpdHkxHzAdBgNVBAMMFlRydXN0Q29y
IFJvb3RDZXJ0IENBLTEwHhcNMTYwMjA0MTIzMjE2WhcNMjkxMjMxMTcyMzE2WjCB
pDELMAkGA1UEBhMCUEExDzANBgNVBAgMBlBhbmFtYTEUMBIGA1UEBwwLUGFuYW1h
IENpdHkxJDAiBgNVBAoMG1RydXN0Q29yIFN5c3RlbXMgUy4gZGUgUi5MLjEnMCUG
A1UECwweVHJ1c3RDb3IgQ2VydGlmaWNhdGUgQXV0aG9yaXR5MR8wHQYDVQQDDBZU
cnVzdENvciBSb290Q2VydCBDQS0xMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAv463leLCJhJrMxnHQFgKq1mqjQCj/IDHUHuO1CAmujIS2CNUSSUQIpid
RtLByZ5OGy4sDjjzGiVoHKZaBeYei0i/mJZ0PmnK6bV4pQa81QBeCQryJ3pS/C3V
seq0iWEk8xoT26nPUu0MJLq5nux+AHT6k61sKZKuUbS701e/s/OojZz0JEsq1pme
9J7+wH5COucLlVPat2gOkEz7cD+PSiyU8ybdY2mplNgQTsVHCJCZGxdNuWxu72CV
EY4hgLW9oHPY0LJ3xEXqWib7ZnZ2+AYfYW0PVcWDtxBWcgYHpfOxGgMFZA6dWorW
hnAbJN7+KIor0Gqw/Hqi3LJ5DotlDwIDAQABo2MwYTAdBgNVHQ4EFgQU7mtJPHo/
DeOxCbeKyKsZn3MzUOcwHwYDVR0jBBgwFoAU7mtJPHo/DeOxCbeKyKsZn3MzUOcw
DwYDVR0TAQH/BAUwAwEB/zAOBgNVHQ8BAf8EBAMCAYYwDQYJKoZIhvcNAQELBQAD
ggEBACUY1JGPE+6PHh0RU9otRCkZoB5rMZ5NDp6tPVxBb5UrJKF5mDo4Nvu7Zp5I
/5CQ7z3UuJu0h3U/IJvOcs+hVcFNZKIZBqEHMwwLKeXx6quj7LUKdJDHfXLy11yf
ke+Ri7fc7Waiz45mO7yfOgLgJ90WmMCV1Aqk5IGadZQ1nJBfiDcGrVmVCrDRZ9MZ
yonnMlo2HD6CqFqTvsbQZJG2z9m2GM/bftJlo6bEjhcxwft+dtvTheNYsnd6djts
L1Ac59v2Z3kf9YKVmgenFK+P3CghZwnS1k1aHBkcjndcw5QkPTJrS37UeJSDvjdN
zl/HHk484IkzlQsPpTLWPFp5LBk=
-----END CERTIFICATE-----
`

// ========================================
// OLSConfig Custom Resource Fixtures
// ========================================

// GetDefaultOLSConfigCR creates an OLSConfig CR with fully configured specs.
// This is the most commonly used fixture for testing full functionality.
func GetDefaultOLSConfigCR() *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
			UID:  "test-uid",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: "testProvider",
						Type: "bam",
						URL:  "https://testURL",
						Models: []olsv1alpha1.ModelSpec{
							{
								Name:              "testModel",
								URL:               "https://testURL",
								ContextWindowSize: 32768,
								Parameters: olsv1alpha1.ModelParametersSpec{
									MaxTokensForResponse: 20,
								},
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Postgres,
					Postgres: olsv1alpha1.PostgresSpec{
						User:              PostgresDefaultUser,
						DbName:            PostgresDefaultDbName,
						SharedBuffers:     PostgresSharedBuffers,
						MaxConnections:    PostgresMaxConnections,
						CredentialsSecret: PostgresSecretName,
					},
				},
				DefaultModel:    "testModel",
				DefaultProvider: "testProvider",
				LogLevel:        LogLevelInfo,
			},
		},
	}
}

// GetEmptyOLSConfigCR creates an OLSConfig CR with no fields set in its specs.
// This is useful for testing default values and validation.
func GetEmptyOLSConfigCR() *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
}

// GetOLSConfigWithCacheCR creates an OLSConfig CR with only cache configuration.
// This is useful for testing cache-specific functionality.
func GetOLSConfigWithCacheCR() *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: OLSNamespaceDefault,
			UID:       "OLSConfig_created_in_getOLSConfigWithCacheCR", // avoid the "uid must not be empty" error
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			OLSConfig: olsv1alpha1.OLSSpec{
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Postgres,
					Postgres: olsv1alpha1.PostgresSpec{
						User:              PostgresDefaultUser,
						DbName:            PostgresDefaultDbName,
						SharedBuffers:     PostgresSharedBuffers,
						MaxConnections:    PostgresMaxConnections,
						CredentialsSecret: PostgresSecretName,
					},
				},
			},
		},
	}
}

// GetNoCacheCR creates an OLSConfig CR with no cache configuration.
// This is useful for testing in-memory cache scenarios.
func GetNoCacheCR() *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: OLSNamespaceDefault,
			UID:       "OLSConfig_created_in_getNoCacheCR", // avoid the "uid must not be empty" error
		},
	}
}

// ========================================
// OLSConfig Modifier Functions (Builder Pattern)
// ========================================

// WithQueryFilters adds test query filters to an OLSConfig CR.
// This modifies the CR in place and returns it for chaining.
func WithQueryFilters(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.OLSConfig.QueryFilters = []olsv1alpha1.QueryFiltersSpec{
		{
			Name:        "testFilter",
			Pattern:     "testPattern",
			ReplaceWith: "testReplace",
		},
	}
	return cr
}

// WithQuotaLimiters adds test quota limiters to an OLSConfig CR.
// This modifies the CR in place and returns it for chaining.
func WithQuotaLimiters(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.OLSConfig.QuotaHandlersConfig = &olsv1alpha1.QuotaHandlersConfig{
		LimitersConfig: []olsv1alpha1.LimiterConfig{
			{
				Name:          "my_user_limiter",
				Type:          "user_limiter",
				InitialQuota:  10000,
				QuotaIncrease: 100,
				Period:        "1d",
			},
			{
				Name:          "my_cluster_limiter",
				Type:          "cluster_limiter",
				InitialQuota:  20000,
				QuotaIncrease: 200,
				Period:        "30d",
			},
		},
	}
	return cr
}

// WithAzureOpenAIProvider configures the first LLM provider as Azure OpenAI.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
func WithAzureOpenAIProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "openai"
	cr.Spec.LLMConfig.Providers[0].Type = "azure_openai"
	cr.Spec.LLMConfig.Providers[0].AzureDeploymentName = "testDeployment"
	cr.Spec.LLMConfig.Providers[0].APIVersion = "2021-09-01"
	return cr
}

// WithWatsonxProvider configures the first LLM provider as IBM Watsonx.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
func WithWatsonxProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "watsonx"
	cr.Spec.LLMConfig.Providers[0].Type = "watsonx"
	cr.Spec.LLMConfig.Providers[0].WatsonProjectID = "testProjectID"
	return cr
}

// WithRHOAIProvider configures the first LLM provider as RHOAI vLLM.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
func WithRHOAIProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "rhoai_vllm"
	cr.Spec.LLMConfig.Providers[0].Type = "rhoai_vllm"
	return cr
}

// WithRHELAIProvider configures the first LLM provider as RHELAI vLLM.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
func WithRHELAIProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "rhelai_vllm"
	cr.Spec.LLMConfig.Providers[0].Type = "rhelai_vllm"
	return cr
}

// WithProviderType is a generic helper to configure the first LLM provider with a specific type.
// This is useful when you need to test custom provider configurations.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
//
// Example:
//
//	cr = utils.WithProviderType(cr, "custom_provider", "custom")
func WithProviderType(cr *olsv1alpha1.OLSConfig, name, providerType string) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = name
	cr.Spec.LLMConfig.Providers[0].Type = providerType
	return cr
}

// ========================================
// Kubernetes Resource Generators
// ========================================

// GenerateRandomSecret creates a test secret with a random API token.
// This is useful for testing secret-dependent functionality without collision.
func GenerateRandomSecret() (*corev1.Secret, error) {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, err
	}

	token := base64.StdEncoding.EncodeToString(randomBytes)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: OLSNamespaceDefault,
		},
		Data: map[string][]byte{
			"apitoken": []byte(token),
		},
	}

	return secret, nil
}

// GenerateRandomTLSSecret creates a test TLS secret with random key and cert.
// This is useful for testing TLS-dependent functionality without collision.
func GenerateRandomTLSSecret() (*corev1.Secret, error) {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, err
	}

	tlsKey := base64.StdEncoding.EncodeToString(randomBytes)
	tlsCert := base64.StdEncoding.EncodeToString(randomBytes)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tls-secret",
			Namespace: OLSNamespaceDefault,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.key": []byte(tlsKey),
			"tls.crt": []byte(tlsCert),
		},
	}

	return secret, nil
}

// GenerateRandomConfigMap creates a test ConfigMap with sample data.
// This is useful for testing ConfigMap-dependent functionality without collision.
func GenerateRandomConfigMap() (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: OLSNamespaceDefault,
		},
		Data: map[string]string{
			"testkey": "testvalue",
		},
	}

	return configMap, nil
}

// ========================================
// Kubernetes Resource Creation/Deletion
// ========================================

// CreateTelemetryPullSecret creates a pull-secret in openshift-config namespace
// for testing telemetry/data collection features.
// If withToken is true, creates a secret with cloud.openshift.com auth.
// If withToken is false, creates a secret without telemetry token (for negative tests).
// This function is idempotent - ignores "already exists" errors.
func CreateTelemetryPullSecret(ctx context.Context, k8sClient client.Client, withToken bool) {
	const telemetryToken = // #nosec G101 -- test fixture, not a real
	`
		{
			"auths": {
				"cloud.openshift.com": {
					"auth": "testkey",
					"email": "testm@test.test"
				}
			}
		}
	`

	const telemetryNoToken = // #nosec G101 -- test fixture, not a real
	`
		{
			"auths": {
				"other.token": {
					"auth": "testkey",
					"email": "testm@test.test"
				}
			}
		}
	`

	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: TelemetryPullSecretNamespace,
		},
	}

	if withToken {
		pullSecret.Data = map[string][]byte{
			".dockerconfigjson": []byte(telemetryToken),
		}
	} else {
		pullSecret.Data = map[string][]byte{
			".dockerconfigjson": []byte(telemetryNoToken),
		}
	}

	err := k8sClient.Create(ctx, pullSecret)
	// Ignore "already exists" errors since the secret may have been created by another test
	if err != nil && !apierrors.IsAlreadyExists(err) {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

// DeleteTelemetryPullSecret removes the pull-secret from openshift-config namespace.
// This function is idempotent - ignores "not found" errors.
func DeleteTelemetryPullSecret(ctx context.Context, k8sClient client.Client) {
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: TelemetryPullSecretNamespace,
		},
	}
	err := k8sClient.Delete(ctx, pullSecret)
	// Ignore "not found" errors since the secret may have been deleted already
	if err != nil && !apierrors.IsNotFound(err) {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

// CreateMCPHeaderSecret creates a secret for MCP server header configuration.
// If withValidHeader is true, creates a secret with the correct header key.
// If withValidHeader is false, creates a secret with incorrect/garbage key (for negative tests).
// This function is idempotent - ignores "already exists" errors.
func CreateMCPHeaderSecret(ctx context.Context, k8sClient client.Client, name string, withValidHeader bool) {
	headerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: OLSNamespaceDefault,
		},
	}

	if withValidHeader {
		headerSecret.Data = map[string][]byte{
			MCPSECRETDATAPATH: []byte(name),
		}
	} else {
		headerSecret.Data = map[string][]byte{
			"garbage": []byte(name),
		}
	}

	err := k8sClient.Create(ctx, headerSecret)
	// Ignore "already exists" errors since the secret may have been created by another test
	if err != nil && !apierrors.IsAlreadyExists(err) {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

// ========================================
// Kubernetes Resource Builders
// ========================================

// BuildDefaultStorageClass creates a test StorageClass with standard configuration.
// This is useful for testing PVC-related functionality.
func BuildDefaultStorageClass() *storagev1.StorageClass {
	trueVal := true
	immediate := storagev1.VolumeBindingImmediate

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard",
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
		Provisioner:          "kubernetes.io/no-provisioner",
		AllowVolumeExpansion: &trueVal,
		VolumeBindingMode:    &immediate,
	}
}

// GetTestPostgresCacheConfig creates a PostgresCacheConfig with default test values.
// This is useful for creating test OLSConfig CRs with Postgres conversation cache.
func GetTestPostgresCacheConfig() PostgresCacheConfig {
	return PostgresCacheConfig{
		Host:         strings.Join([]string{PostgresServiceName, OLSNamespaceDefault, "svc"}, "."),
		Port:         PostgresServicePort,
		User:         PostgresDefaultUser,
		DbName:       PostgresDefaultDbName,
		PasswordPath: path.Join(CredentialsMountRoot, PostgresSecretName, OLSComponentPasswordFileName),
		SSLMode:      PostgresDefaultSSLMode,
		CACertPath:   path.Join(OLSAppCertsMountRoot, PostgresCertsSecretName, PostgresCAVolume, "service-ca.crt"),
	}
}

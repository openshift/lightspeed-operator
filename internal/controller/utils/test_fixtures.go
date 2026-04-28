package utils

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
									ToolBudgetRatio:      0.5,
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
						SharedBuffers:  PostgresSharedBuffers,
						MaxConnections: PostgresMaxConnections,
					},
				},
				DefaultModel:    "testModel",
				DefaultProvider: "testProvider",
				MaxIterations:   5,
				LogLevel:        olsv1alpha1.LogLevelInfo,
			},
		},
	}
}

// ========================================
// OLSConfig Modifier Functions (Builder Pattern)
// ========================================

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

// WithGoogleVertexProvider configures the first LLM provider as Google Vertex.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
func WithGoogleVertexProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "google_vertex"
	cr.Spec.LLMConfig.Providers[0].Type = "google_vertex"
	cr.Spec.LLMConfig.Providers[0].GoogleVertexConfig = &olsv1alpha1.VertexConfig{
		ProjectID: "testProjectID",
		Location:  "testLocation",
	}
	return cr
}

// WithGoogleVertexAnthropicProvider configures the first LLM provider as Google Vertex Anthropic.
// This modifies the CR in place and returns it for chaining.
// Requires that Providers[0] already exists.
func WithGoogleVertexAnthropicProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "google_vertex_anthropic"
	cr.Spec.LLMConfig.Providers[0].Type = "google_vertex_anthropic"
	cr.Spec.LLMConfig.Providers[0].GoogleVertexAnthropicConfig = &olsv1alpha1.VertexConfig{
		ProjectID: "testProjectID",
		Location:  "testLocation",
	}
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
			DefaultCredentialKey: []byte(token),
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

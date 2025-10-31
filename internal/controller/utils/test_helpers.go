package utils

import (
	"crypto/rand"
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func StatusHasCondition(status olsv1alpha1.OLSConfigStatus, condition metav1.Condition) bool {
	// ignore ObservedGeneration and LastTransitionTime
	for _, c := range status.Conditions {
		if c.Type == condition.Type &&
			c.Status == condition.Status &&
			c.Reason == condition.Reason &&
			c.Message == condition.Message {
			return true
		}
	}
	return false
}

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
				LogLevel:        "INFO",
			},
		},
	}
}

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

func GetNoCacheCR() *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: OLSNamespaceDefault,
			UID:       "OLSConfig_created_in_getNoCacheCR", // avoid the "uid must not be empty" error
		},
	}
}

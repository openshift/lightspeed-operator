package e2e

import (
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func generateLLMTokenSecret(name string) (*corev1.Secret, error) { // nolint:unused
	token := os.Getenv(LLMTokenEnvVar)
	var tenantID = os.Getenv(AzureTenantID)
	var clientID = os.Getenv(AzureClientID)
	var clientSecret = os.Getenv(AzureClientSecret)
	if token == "" {
		return nil, fmt.Errorf("LLM token not found in $%s", LLMTokenEnvVar)
	}
	if tenantID == "" {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: OLSNameSpace,
			},
			StringData: map[string]string{
				LLMApiTokenFileName: token,
			},
		}, nil
	} else {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: OLSNameSpace,
			},
			StringData: map[string]string{
				AzureOpenaiClientID:     clientID,
				AzureOpenaiTenantID:     tenantID,
				AzureOpenaiClientSecret: clientSecret,
			},
		}, nil
	}
}

func generateOLSConfig() (*olsv1alpha1.OLSConfig, error) { // nolint:unused
	llmProvider := os.Getenv(LLMProviderEnvVar)
	if llmProvider == "" {
		llmProvider = LLMDefaultProvider
	}
	llmModel := os.Getenv(LLMModelEnvVar)
	if llmModel == "" {
		llmModel = OpenAIDefaultModel
	}
	replicas := int32(1)
	if llmProvider == "azure_openai" {
		return &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: OLSCRName,
			},
			Spec: olsv1alpha1.OLSConfigSpec{
				LLMConfig: olsv1alpha1.LLMSpec{
					Providers: []olsv1alpha1.ProviderSpec{
						{
							Name: llmProvider,
							Models: []olsv1alpha1.ModelSpec{
								{
									Name: llmModel,
								},
							},
							CredentialsSecretRef: corev1.LocalObjectReference{
								Name: LLMTokenFirstSecretName,
							},
							Type:                llmProvider,
							AzureDeploymentName: llmModel,
							URL:                 AzureURL,
						},
					},
				},
				OLSConfig: olsv1alpha1.OLSSpec{
					ConversationCache: olsv1alpha1.ConversationCacheSpec{
						Type: olsv1alpha1.Postgres,
						Postgres: olsv1alpha1.PostgresSpec{
							SharedBuffers:  "256MB",
							MaxConnections: 2000,
						},
					},
					DefaultModel:    llmModel,
					DefaultProvider: llmProvider,
					LogLevel:        olsv1alpha1.LogLevelInfo,
					DeploymentConfig: olsv1alpha1.DeploymentConfig{
						APIContainer: olsv1alpha1.Config{
							Replicas: &replicas,
						},
					},
				},
			},
		}, nil
	}
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: OLSCRName,
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: llmProvider,
						Models: []olsv1alpha1.ModelSpec{
							{
								Name: llmModel,
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: LLMTokenFirstSecretName,
						},
						Type: llmProvider,
					},
				},
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Postgres,
					Postgres: olsv1alpha1.PostgresSpec{
						SharedBuffers:  "256MB",
						MaxConnections: 2000,
					},
				},
				DefaultModel:    llmModel,
				DefaultProvider: llmProvider,
				LogLevel:        olsv1alpha1.LogLevelInfo,
				DeploymentConfig: olsv1alpha1.DeploymentConfig{
					APIContainer: olsv1alpha1.Config{
						Replicas: &replicas,
					},
				},
			},
		},
	}, nil

}

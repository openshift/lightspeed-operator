package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// todo: implement LSC config map generation
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) generateLSCConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) { //lint:ignore U1000 Ignore unused lint error before implementation of reconciliation functions
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerConfigCmName,
			Namespace: r.Options.Namespace,
		},
	}
	return configMap, nil
}

// todo: implement LSC deployment generation
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) generateLSCDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) { //lint:ignore U1000 Ignore unused lint error before implementation of reconciliation functions

	// todo: mount LLM token secret as the environment variable for the llama stack configuration file
	// refer to generateLlamaStackConfigFile for the environment variable names and its corresponding provider type
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: r.Options.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: cr.Spec.OLSConfig.DeploymentConfig.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: generateAppServerSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: generateAppServerSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "lsc-app-server",
							Image: r.Options.LightspeedServiceImage,
						},
					},
				},
			},
		},
	}
	return deployment, nil
}

// todo: implement LSC deployment update
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) updateLSCDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {

	return nil
}

// todo: implement other fields for the llama stack configuration map
// generateLlamaStackConfigMap generates the llama stack configuration map for the LSC app server
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) generateLlamaStackConfigFile(ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	llamaStackConfig := &LlamaStackConfig{
		Version: LlamaStackConfigVersion,
	}

	// inference providers
	for _, provider := range cr.Spec.LLMConfig.Providers {

		providerConfig := InferenceProviderConfig{
			ProviderID: provider.Name,
		}
		switch provider.Type {
		case AzureOpenAIType:
			providerConfig.ProviderType = LlamaStackAzureOpenAIType
			providerConfig.Config = &InferenceProviderAzureOpenAI{
				// todo: add these environment variables to the podspec in generateLSCDeployment
				APIKey:     fmt.Sprintf("${env.%s}", AzureOpenAIAPIKeyEnvVar),
				APIBase:    provider.URL,
				APIVersion: provider.APIVersion,
				APIType:    "", // default api type is "azure" for Azure OpenAI
			}
			// warning: AzureDeploymentName is not supported by Llama stack yet
		case OpenAIType:
			providerConfig.ProviderType = LlamaStackOpenAIType
			providerConfig.Config = &InferenceProviderOpenAI{
				// todo: add these environment variables to the podspec in generateLSCDeployment
				APIKey:  fmt.Sprintf("${env.%s}", OpenAIAPIKeyEnvVar),
				BaseURL: provider.URL,
			}
		case WatsonXType:
			providerConfig.ProviderType = LlamaStackWatsonXType
			providerConfig.Config = &InferenceProviderWatsonX{
				// todo: add these environment variables to the podspec in generateLSCDeployment
				APIKey:    fmt.Sprintf("${env.%s}", WatsonXAPIKeyEnvVar),
				URL:       provider.URL,
				ProjectID: provider.WatsonProjectID,
			}
		case RHELAIType:

			providerConfig.ProviderType = LlamaStackVLLMType
			providerConfig.Config = &InferenceProviderVLLM{
				// todo: add these environment variables to the podspec in generateLSCDeployment
				APIToken: fmt.Sprintf("${env.%s}", RHELAIAPITokenEnvVar),
				URL:      provider.URL,
				// to be noted: max_tokens, tls_verify, refresh_models are available for both RHELAI and RHOAI but not specified in OLSConfig CR
			}
		case RHOAIType:
			providerConfig.ProviderType = LlamaStackVLLMType
			providerConfig.Config = &InferenceProviderVLLM{
				// todo: add these environment variables to the podspec in generateLSCDeployment
				APIToken: fmt.Sprintf("${env.%s}", RHOAIAPITokenEnvVar),
				URL:      provider.URL,
				// to be noted: max_tokens, tls_verify, refresh_models are available for both RHELAI and RHOAI but not specified in OLSConfig CR
			}
		default:
			return "", fmt.Errorf("unsupported provider type: %s", provider.Type)
		}
		llamaStackConfig.Providers.Inference = append(llamaStackConfig.Providers.Inference, providerConfig)
	}

	llamaStackConfigBytes, err := yaml.Marshal(llamaStackConfig)
	if err != nil {
		return "", fmt.Errorf("failed to generate llama stack configuration file %w", err)
	}
	return string(llamaStackConfigBytes), nil
}

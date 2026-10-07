package e2e

import "testing"

func TestGenerateBaseOLSConfigReasoningConfig(t *testing.T) {
	for _, provider := range []string{"openai", "azure_openai", "watsonx"} {
		for _, multiProvider := range []bool{false, true} {
			name := provider + "/single-provider"
			if multiProvider {
				name = provider + "/multi-provider"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv(LLMProviderEnvVar, provider)
				t.Setenv(LLMModelEnvVar, "primary-model")
				config, err := generateBaseOLSConfig(olsConfigOptions{multiProvider: multiProvider}, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range config.Spec.LLMConfig.Providers {
					for _, model := range p.Models {
						reasoning := model.Parameters.ReasoningConfig
						if (provider == "openai" || provider == "azure_openai") && model.Name == "primary-model" {
							if len(reasoning) != 1 || string(reasoning["effort"].Raw) != `"medium"` {
								t.Errorf("provider %s model %s: expected medium reasoning effort, got %v", p.Name, model.Name, reasoning)
							}
						} else if len(reasoning) != 0 {
							t.Errorf("provider %s model %s: unexpected reasoning config %v", p.Name, model.Name, reasoning)
						}
					}
				}
			})
		}
	}
}

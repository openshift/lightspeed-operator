package lcore

import (
	"context"
	"fmt"
	"strings"
	"testing"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

func TestBuildLlamaStackYAML_SupportedProvider(t *testing.T) {
	// Create a test CR with supported provider (OpenAI)
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: "openai",
						Type: "openai",
						Models: []olsv1alpha1.ModelSpec{
							{Name: "gpt-4o"},
						},
					},
				},
			},
		},
	}

	// Build the YAML
	ctx := context.Background()
	yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML returned error for supported provider: %v", err)
	}

	// Verify it's not empty
	if len(yamlOutput) == 0 {
		t.Fatal("buildLlamaStackYAML returned empty string")
	}

	// Verify it's valid YAML by unmarshaling
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML produced invalid YAML: %v\nYAML output:\n%s", err, yamlOutput)
	}

	// Verify key sections exist
	expectedKeys := []string{"version", "apis", "providers", "server", "models"}
	for _, key := range expectedKeys {
		if _, exists := result[key]; !exists {
			t.Errorf("Expected key '%s' not found in YAML output", key)
		}
	}

	t.Logf("Successfully validated Llama Stack YAML (%d bytes)", len(yamlOutput))
}

func TestBuildLlamaStackYAML_UnsupportedProvider(t *testing.T) {
	// Test unsupported providers (watsonx, bam are not supported)
	// Note: rhoai_vllm and rhelai_vllm are now supported via OpenAI compatibility
	unsupportedProviders := []string{"watsonx", "bam"}

	for _, providerType := range unsupportedProviders {
		t.Run(providerType, func(t *testing.T) {
			// Create a test CR with unsupported provider
			cr := &olsv1alpha1.OLSConfig{
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{
							{
								Name: "test-provider",
								Type: providerType,
								Models: []olsv1alpha1.ModelSpec{
									{Name: "test-model"},
								},
							},
						},
					},
				},
			}

			// Build the YAML - should return error
			ctx := context.Background()
			yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)

			// Verify error is returned
			if err == nil {
				t.Fatalf("Expected error for unsupported provider '%s', but got none. Output: %s", providerType, yamlOutput)
			}

			// Verify error message mentions the provider
			expectedErrMsg := "not currently supported by Llama Stack"
			if err.Error() == "" || len(err.Error()) == 0 {
				t.Errorf("Error message is empty for unsupported provider '%s'", providerType)
			}
			if err.Error() != "" && len(err.Error()) > 0 {
				// Check if error message contains expected text
				if !contains(err.Error(), expectedErrMsg) {
					t.Errorf("Error message '%s' doesn't contain expected text '%s'", err.Error(), expectedErrMsg)
				}
				if !contains(err.Error(), providerType) {
					t.Errorf("Error message '%s' doesn't mention provider type '%s'", err.Error(), providerType)
				}
			}

			t.Logf("Correctly rejected unsupported provider '%s' with error: %v", providerType, err)
		})
	}
}

func TestBuildLlamaStackYAML_OpenAICompatibleProviders(t *testing.T) {
	// Test that vLLM providers (rhoai_vllm, rhelai_vllm) use remote::vllm provider type
	vllmProviders := []string{"rhoai_vllm", "rhelai_vllm"}

	for _, providerType := range vllmProviders {
		t.Run(providerType, func(t *testing.T) {
			// Create a test CR with vLLM provider
			cr := &olsv1alpha1.OLSConfig{
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{
							{
								Name: "test-provider",
								Type: providerType,
								URL:  "https://test-vllm-endpoint.com/v1",
								Models: []olsv1alpha1.ModelSpec{
									{Name: "test-model"},
								},
							},
						},
					},
				},
			}

			// Build the YAML - should succeed
			ctx := context.Background()
			yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)

			// Verify no error is returned
			if err != nil {
				t.Fatalf("Unexpected error for supported provider '%s': %v", providerType, err)
			}

			// Verify it's valid YAML
			var result map[string]interface{}
			err = yaml.Unmarshal([]byte(yamlOutput), &result)
			if err != nil {
				t.Fatalf("buildLlamaStackYAML produced invalid YAML for '%s': %v", providerType, err)
			}

			// Verify provider is configured as remote::vllm
			providers, ok := result["providers"].(map[string]interface{})
			if !ok {
				t.Fatalf("providers section not found or invalid type")
			}

			inference, ok := providers["inference"].([]interface{})
			if !ok || len(inference) == 0 {
				t.Fatalf("inference providers not found or empty")
			}

			// Find the test provider (not the sentence-transformers one)
			var testProvider map[string]interface{}
			for _, provider := range inference {
				p, ok := provider.(map[string]interface{})
				if !ok {
					continue
				}
				if p["provider_id"] == "test-provider" {
					testProvider = p
					break
				}
			}

			if testProvider == nil {
				t.Fatalf("Test provider not found in inference providers")
			}

			// Verify it's configured as vLLM (not OpenAI)
			if testProvider["provider_type"] != "remote::vllm" {
				t.Errorf("Expected provider_type 'remote::vllm' for %s, got '%v'", providerType, testProvider["provider_type"])
			}

			// Verify URL is present in config
			config, ok := testProvider["config"].(map[string]interface{})
			if !ok {
				t.Fatalf("provider config not found or invalid type")
			}

			if url, ok := config["url"].(string); !ok || url == "" {
				t.Errorf("Expected URL to be configured for %s provider", providerType)
			}

			t.Logf("Successfully validated '%s' provider uses remote::vllm", providerType)
		})
	}
}

func TestBuildLlamaStackYAML_AzureProvider(t *testing.T) {
	// Create a fake secret with API token for Azure provider
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "azure-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"apitoken": []byte("test-api-key"),
		},
	}

	// Create a fake client with the secret
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = olsv1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	// Create a test reconciler
	logger := zap.New(zap.UseDevMode(true))
	testReconciler := utils.NewTestReconciler(
		fakeClient,
		logger,
		scheme,
		"test-namespace",
	)

	// Create a test CR with Azure OpenAI provider
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name:                "azure-openai",
						Type:                "azure_openai",
						URL:                 "https://my-azure.openai.azure.com",
						AzureDeploymentName: "gpt-4-deployment",
						APIVersion:          "2024-02-15-preview",
						Models: []olsv1alpha1.ModelSpec{
							{
								Name:              "gpt-4",
								ContextWindowSize: 128000,
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "azure-secret",
						},
					},
				},
			},
		},
	}

	// Build the YAML
	ctx := context.Background()
	yamlOutput, err := buildLlamaStackYAML(testReconciler, ctx, cr)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML returned error for Azure provider: %v", err)
	}

	// Verify it's valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML produced invalid YAML: %v", err)
	}

	// Verify Azure provider configuration
	providers, ok := result["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("providers section not found or invalid type")
	}

	inference, ok := providers["inference"].([]interface{})
	if !ok || len(inference) == 0 {
		t.Fatalf("inference providers not found or empty")
	}

	// Find the Azure provider (not the sentence-transformers one)
	var azureProvider map[string]interface{}
	for _, provider := range inference {
		p, ok := provider.(map[string]interface{})
		if !ok {
			continue
		}
		if p["provider_type"] == "remote::azure" {
			azureProvider = p
			break
		}
	}

	if azureProvider == nil {
		t.Fatalf("Azure provider not found in inference providers")
	}

	// Check provider_type
	if azureProvider["provider_type"] != "remote::azure" {
		t.Errorf("Expected provider_type 'remote::azure', got '%v'", azureProvider["provider_type"])
	}

	// Check config fields
	config, ok := azureProvider["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("provider config not found or invalid type")
	}

	// Verify Azure-specific fields are present
	// Note: Config always includes api_key (required by LiteLLM) plus client credentials fields
	// The client credentials fields will have empty defaults if not used
	requiredFields := []string{
		"api_key",         // Always present (required by LiteLLM's Pydantic validation)
		"client_id",       // Always present (with empty default if not using client credentials)
		"tenant_id",       // Always present (with empty default if not using client credentials)
		"client_secret",   // Always present (with empty default if not using client credentials)
		"api_base",        // Azure endpoint
		"api_version",     // Azure API version
		"deployment_name", // Azure deployment
	}
	for _, field := range requiredFields {
		if _, exists := config[field]; !exists {
			t.Errorf("Expected field '%s' not found in Azure provider config", field)
		}
	}

	// Verify api_key has the correct env var format
	if apiKey, ok := config["api_key"].(string); ok && apiKey != "" {
		if !strings.HasPrefix(apiKey, "${env.") || !strings.HasSuffix(apiKey, "_API_KEY}") {
			t.Errorf("api_key doesn't have correct env var format, got: %s", apiKey)
		}
	} else {
		t.Errorf("api_key field is missing or empty")
	}

	t.Logf("Successfully validated Llama Stack YAML with Azure provider (%d bytes)", len(yamlOutput))
}

func TestBuildLlamaStackYAML_GenericProvider(t *testing.T) {
	// Test generic provider with providerType and config
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name:         "fireworks",
						Type:         "generic",
						ProviderType: "remote::fireworks-ai",
						Config: &runtime.RawExtension{
							Raw: []byte(`{"url": "https://api.fireworks.ai/inference/v1", "custom_field": "test_value"}`),
						},
						Models: []olsv1alpha1.ModelSpec{
							{Name: "accounts/fireworks/models/llama-v3-70b-instruct"},
						},
					},
				},
			},
		},
	}

	// Build the YAML
	ctx := context.Background()
	yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML failed for generic provider: %v", err)
	}

	// Verify output is not empty
	if len(yamlOutput) == 0 {
		t.Fatal("buildLlamaStackYAML returned empty string for generic provider")
	}

	// Parse YAML output
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML produced invalid YAML: %v\nOutput:\n%s", err, yamlOutput)
	}

	// Verify providers section exists
	providers, ok := result["providers"].(map[string]interface{})
	if !ok {
		t.Fatal("providers section missing or invalid type")
	}

	// Verify inference providers exist
	inference, ok := providers["inference"].([]interface{})
	if !ok || len(inference) == 0 {
		t.Fatal("inference providers missing or empty")
	}

	// Find the fireworks provider
	var fireworksProvider map[string]interface{}
	for _, provider := range inference {
		p, ok := provider.(map[string]interface{})
		if !ok {
			continue
		}
		if p["provider_id"] == "fireworks" {
			fireworksProvider = p
			break
		}
	}

	if fireworksProvider == nil {
		t.Fatal("Fireworks provider not found in inference providers")
	}

	// CRITICAL: Verify provider_type is set correctly from ProviderType field
	if fireworksProvider["provider_type"] != "remote::fireworks-ai" {
		t.Errorf("Expected provider_type='remote::fireworks-ai', got '%v'", fireworksProvider["provider_type"])
	}

	// CRITICAL: Verify config section exists and has correct fields
	config, ok := fireworksProvider["config"].(map[string]interface{})
	if !ok {
		t.Fatal("provider config missing or invalid type")
	}

	// CRITICAL: Verify custom config fields are passed through
	if config["url"] != "https://api.fireworks.ai/inference/v1" {
		t.Errorf("Config URL not passed through correctly, got: %v", config["url"])
	}

	if config["custom_field"] != "test_value" {
		t.Errorf("Custom config field not preserved, got: %v", config["custom_field"])
	}

	// CRITICAL: Verify credential auto-injection (api_key should be added)
	apiKey, ok := config["api_key"]
	if !ok {
		t.Error("api_key not auto-injected into config")
	} else {
		expectedKey := "${env.FIREWORKS_API_KEY}"
		if apiKey != expectedKey {
			t.Errorf("Expected api_key='%s', got '%v'", expectedKey, apiKey)
		}
	}

	// Verify models section
	models, ok := result["models"].([]interface{})
	if !ok || len(models) == 0 {
		t.Fatal("models section missing or empty")
	}

	// Find the llama model
	foundModel := false
	for _, model := range models {
		m, ok := model.(map[string]interface{})
		if !ok {
			continue
		}
		if m["model_id"] == "accounts/fireworks/models/llama-v3-70b-instruct" {
			foundModel = true
			// Verify it's mapped to the fireworks provider
			if m["provider_id"] != "fireworks" {
				t.Errorf("Model not mapped to correct provider, got: %v", m["provider_id"])
			}
			break
		}
	}

	if !foundModel {
		t.Error("Model not found in models section")
	}

	t.Logf("✓ Generic provider config generated correctly (%d bytes)", len(yamlOutput))
}

func TestBuildLlamaStackYAML_GenericProvider_InvalidValues(t *testing.T) {
	// Test multiple invalid configurations
	testCases := []struct {
		name          string
		provider      olsv1alpha1.ProviderSpec
		expectedError string
	}{
		{
			name: "invalid JSON in config",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "broken-json",
				Type:         "generic",
				ProviderType: "remote::custom",
				Config: &runtime.RawExtension{
					Raw: []byte(`{invalid json syntax`),
				},
				Models: []olsv1alpha1.ModelSpec{{Name: "test-model"}},
			},
			expectedError: "failed to unmarshal config",
		},
		{
			name: "type generic without providerType",
			provider: olsv1alpha1.ProviderSpec{
				Name: "missing-provider-type",
				Type: "generic",
				// ProviderType not set - this should fail
				Models: []olsv1alpha1.ModelSpec{{Name: "test-model"}},
			},
			expectedError: "requires providerType and config fields to be set",
		},
		{
			name: "empty config Raw bytes",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "empty-config",
				Type:         "generic",
				ProviderType: "remote::empty",
				Config: &runtime.RawExtension{
					Raw: []byte(""),
				},
				Models: []olsv1alpha1.ModelSpec{{Name: "test-model"}},
			},
			expectedError: "failed to unmarshal config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cr := &olsv1alpha1.OLSConfig{
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{tc.provider},
					},
				},
			}

			ctx := context.Background()
			yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)

			// Should return error
			if err == nil {
				t.Fatalf("Expected error for %s, got none. Output: %s", tc.name, yamlOutput)
			}

			// Verify error message contains expected substring
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s', got: %v", tc.expectedError, err)
			}

			// Verify error message mentions provider name (for better debugging)
			if !strings.Contains(err.Error(), tc.provider.Name) {
				t.Errorf("Error message should mention provider name '%s', got: %v", tc.provider.Name, err)
			}

			t.Logf("✓ Correctly rejected %s with error: %v", tc.name, err)
		})
	}
}

func TestBuildLlamaStackYAML_GenericProvider_CustomCredentialKey(t *testing.T) {
	// Test generic provider with custom credentialKey field
	// This validates that users can specify a different secret key name
	// instead of the default "apitoken"
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name:          "together",
						Type:          "generic",
						ProviderType:  "remote::together",
						CredentialKey: "bearer_token", // Custom key instead of default "apitoken"
						Config: &runtime.RawExtension{
							Raw: []byte(`{"url": "https://api.together.xyz/v1"}`),
						},
						Models: []olsv1alpha1.ModelSpec{
							{Name: "meta-llama/Llama-3-70b-chat-hf"},
						},
					},
				},
			},
		},
	}

	// Build the YAML
	ctx := context.Background()
	yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML failed for generic provider with custom credentialKey: %v", err)
	}

	// Parse YAML output
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("Invalid YAML: %v\nOutput:\n%s", err, yamlOutput)
	}

	// Find the together provider
	providers := result["providers"].(map[string]interface{})
	inference := providers["inference"].([]interface{})
	var togetherProvider map[string]interface{}
	for _, provider := range inference {
		p := provider.(map[string]interface{})
		if p["provider_id"] == "together" {
			togetherProvider = p
			break
		}
	}

	if togetherProvider == nil {
		t.Fatal("Together provider not found in inference providers")
	}

	// Verify provider_type
	if togetherProvider["provider_type"] != "remote::together" {
		t.Errorf("Expected provider_type='remote::together', got '%v'", togetherProvider["provider_type"])
	}

	// Get config
	config := togetherProvider["config"].(map[string]interface{})

	// CRITICAL: Verify URL is preserved
	if config["url"] != "https://api.together.xyz/v1" {
		t.Errorf("Config URL not preserved, got: %v", config["url"])
	}

	// CRITICAL: Verify credential is still auto-injected
	// Even with custom credentialKey, the config should have api_key injected
	// (The credentialKey affects which secret key is read, not the config field name)
	apiKey, ok := config["api_key"]
	if !ok {
		t.Error("api_key not auto-injected even with custom credentialKey")
	} else {
		// Environment variable name should still be based on provider name
		expectedKey := "${env.TOGETHER_API_KEY}"
		if apiKey != expectedKey {
			t.Errorf("Expected api_key='%s', got '%v'", expectedKey, apiKey)
		}
	}

	t.Logf("✓ Generic provider with custom credentialKey='bearer_token' generated correctly")
}

func TestBuildLlamaStackYAML_GenericProvider_ConfigWithExistingCredential(t *testing.T) {
	// Test generic provider where config already has a credential field
	// The hasCredentialField() function should detect it and not inject duplicate
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name:         "custom",
						Type:         "generic",
						ProviderType: "remote::custom",
						Config: &runtime.RawExtension{
							// Config already includes api_token field
							Raw: []byte(`{
								"url": "https://api.custom.com/v1",
								"api_token": "${env.CUSTOM_SECRET_TOKEN}"
							}`),
						},
						Models: []olsv1alpha1.ModelSpec{
							{Name: "custom-model"},
						},
					},
				},
			},
		},
	}

	// Build the YAML
	ctx := context.Background()
	yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML failed: %v", err)
	}

	// Parse YAML output
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("Invalid YAML: %v", err)
	}

	// Find the custom provider
	providers := result["providers"].(map[string]interface{})
	inference := providers["inference"].([]interface{})
	var customProvider map[string]interface{}
	for _, provider := range inference {
		p := provider.(map[string]interface{})
		if p["provider_id"] == "custom" {
			customProvider = p
			break
		}
	}

	if customProvider == nil {
		t.Fatal("Custom provider not found")
	}

	// Get config
	config := customProvider["config"].(map[string]interface{})

	// CRITICAL: Verify original api_token is preserved
	apiToken, ok := config["api_token"]
	if !ok {
		t.Error("Original api_token field was lost")
	} else {
		if apiToken != "${env.CUSTOM_SECRET_TOKEN}" {
			t.Errorf("api_token was modified, expected '${env.CUSTOM_SECRET_TOKEN}', got '%v'", apiToken)
		}
	}

	// CRITICAL: Verify api_key was NOT auto-injected (because api_token already exists)
	// The hasCredentialField() function checks for api_key, api_token, apikey, token, access_token
	// Since api_token exists, api_key should NOT be injected
	if apiKey, ok := config["api_key"]; ok {
		t.Errorf("api_key should NOT be auto-injected when api_token already exists, but got: %v", apiKey)
	}

	// Verify URL is still present
	if config["url"] != "https://api.custom.com/v1" {
		t.Errorf("URL not preserved, got: %v", config["url"])
	}

	t.Logf("✓ Config with existing credential field preserved correctly (no duplicate injection)")
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBuildLCoreConfigYAML(t *testing.T) {
	// Use a proper CR from test fixtures instead of nil
	cr := utils.GetDefaultOLSConfigCR()

	// Create a fake client and reconciler for the test
	scheme := runtime.NewScheme()
	_ = olsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	logger := zap.New(zap.UseDevMode(true))
	testReconciler := utils.NewTestReconciler(
		fakeClient,
		logger,
		scheme,
		"test-namespace",
	)

	yamlOutput, err := buildLCoreConfigYAML(testReconciler, cr)
	if err != nil {
		t.Fatalf("buildLCoreConfigYAML returned error: %v", err)
	}

	// Verify it's not empty
	if len(yamlOutput) == 0 {
		t.Fatal("buildLCoreConfigYAML returned empty string")
	}

	// Verify it's valid YAML by unmarshaling
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("buildLCoreConfigYAML produced invalid YAML: %v\nYAML output:\n%s", err, yamlOutput)
	}

	// Verify key sections exist for LCore config
	expectedKeys := []string{"name", "service", "llama_stack", "user_data_collection", "authentication"}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("buildLCoreConfigYAML missing expected key: %s", key)
		}
	}

	t.Logf("Successfully validated LCore Config YAML (%d bytes)", len(yamlOutput))
}

func TestLCoreConfigYAMLComponentFunctions(t *testing.T) {
	// Use a proper CR from test fixtures instead of nil
	cr := utils.GetDefaultOLSConfigCR()

	// Test that all component functions return non-empty maps
	components := map[string]func(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) map[string]interface{}{
		"Service":            buildLCoreServiceConfig,
		"LlamaStack":         buildLCoreLlamaStackConfig,
		"UserDataCollection": buildLCoreUserDataCollectionConfig,
		"Authentication":     buildLCoreAuthenticationConfig,
		// "MCPServers": buildLCoreMCPServersConfig, // Commented out - function is unused
	}

	for name, fn := range components {
		t.Run(name, func(t *testing.T) {
			result := fn(nil, cr)
			if len(result) == 0 {
				t.Errorf("build%sConfig returned empty map", name)
			}
			// Verify the map can be marshaled to YAML
			yamlBytes, err := yaml.Marshal(result)
			if err != nil {
				t.Errorf("build%sConfig produced invalid map that can't be marshaled: %v", name, err)
			}
			if len(yamlBytes) == 0 {
				t.Errorf("build%sConfig marshaled to empty YAML", name)
			}
		})
	}
}

// TestLlamaStackYAMLComponentFunctions is commented out - functions now return maps instead of strings
// The individual component functions are tested implicitly through TestBuildLlamaStackYAML
//func TestLlamaStackYAMLComponentFunctions(t *testing.T) {
//	// Component functions now return maps/slices, not strings
//	// They are tested implicitly through the full YAML generation test
//}

func TestGenerateExporterConfigMap_DefaultServiceID(t *testing.T) {
	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}

	// Use a minimal mock reconciler with OLSConfig registered in scheme
	scheme := runtime.NewScheme()
	_ = olsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	r := &mockReconcilerForAssets{
		namespace: utils.OLSNamespaceDefault,
		scheme:    scheme,
	}

	cm, err := generateExporterConfigMap(r, cr)
	if err != nil {
		t.Fatalf("generateExporterConfigMap returned error: %v", err)
	}

	if cm.Name != utils.ExporterConfigCmName {
		t.Errorf("Expected ConfigMap name %s, got %s", utils.ExporterConfigCmName, cm.Name)
	}

	if cm.Namespace != utils.OLSNamespaceDefault {
		t.Errorf("Expected namespace %s, got %s", utils.OLSNamespaceDefault, cm.Namespace)
	}

	configContent := cm.Data[utils.ExporterConfigFilename]
	if !strings.Contains(configContent, `service_id: "`+utils.ServiceIDOLS+`"`) {
		t.Errorf("Expected service_id '%s' in config, got: %s", utils.ServiceIDOLS, configContent)
	}

	// Verify other required fields
	requiredFields := []string{"ingress_server_url", "allowed_subdirs", "collection_interval"}
	for _, field := range requiredFields {
		if !strings.Contains(configContent, field) {
			t.Errorf("Expected field '%s' in exporter config", field)
		}
	}
}

func TestGenerateExporterConfigMap_RHOSLightspeedServiceID(t *testing.T) {
	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
			Labels: map[string]string{
				utils.RHOSOLightspeedOwnerIDLabel: "test-owner-id",
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = olsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	r := &mockReconcilerForAssets{
		namespace: utils.OLSNamespaceDefault,
		scheme:    scheme,
	}

	cm, err := generateExporterConfigMap(r, cr)
	if err != nil {
		t.Fatalf("generateExporterConfigMap returned error: %v", err)
	}

	configContent := cm.Data[utils.ExporterConfigFilename]
	if !strings.Contains(configContent, `service_id: "`+utils.ServiceIDRHOSO+`"`) {
		t.Errorf("Expected service_id '%s' when RHOSO label present, got: %s", utils.ServiceIDRHOSO, configContent)
	}
}

// mockReconcilerForAssets is a minimal mock for testing asset generation
type mockReconcilerForAssets struct {
	reconciler.Reconciler
	namespace string
	scheme    *runtime.Scheme
}

func (m *mockReconcilerForAssets) GetNamespace() string {
	return m.namespace
}

func (m *mockReconcilerForAssets) GetScheme() *runtime.Scheme {
	return m.scheme
}

// TestBuildLlamaStackYAML_EdgeCases tests edge cases and error conditions
func TestBuildLlamaStackYAML_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		provider      olsv1alpha1.ProviderSpec
		expectError   bool
		errorContains string
	}{
		{
			name: "MalformedJSON_Config",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "bad-json",
				Type:         "generic",
				ProviderType: "remote::custom",
				Config: &runtime.RawExtension{
					Raw: []byte(`{invalid json}`),
				},
				Models: []olsv1alpha1.ModelSpec{{Name: "test-model"}},
			},
			expectError:   true,
			errorContains: "invalid character",
		},
		{
			name: "EmptyProviderName",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "",
				Type:         "openai",
				ProviderType: "",
				Models:       []olsv1alpha1.ModelSpec{{Name: "gpt-4"}},
			},
			expectError: false, // Name can be empty in provider spec
		},
		{
			name: "LongProviderName",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "provider-" + strings.Repeat("a", 200),
				Type:         "openai",
				ProviderType: "",
				Models:       []olsv1alpha1.ModelSpec{{Name: "gpt-4"}},
			},
			expectError: false, // Should handle long names
		},
		{
			name: "SpecialCharactersInProviderName",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "my-provider@#$%",
				Type:         "openai",
				ProviderType: "",
				Models:       []olsv1alpha1.ModelSpec{{Name: "gpt-4"}},
			},
			expectError: false, // Should not fail on special chars in name
		},
		{
			name: "VeryLargeConfig",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "large-config",
				Type:         "generic",
				ProviderType: "remote::custom",
				Config: &runtime.RawExtension{
					Raw: []byte(`{"data": "` + strings.Repeat("x", 10000) + `"}`),
				},
				Models: []olsv1alpha1.ModelSpec{{Name: "test"}},
			},
			expectError: false,
		},
		{
			name: "NestedConfig",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "nested",
				Type:         "generic",
				ProviderType: "remote::custom",
				Config: &runtime.RawExtension{
					Raw: []byte(`{"outer": {"inner": {"deep": {"value": 123}}}}`),
				},
				Models: []olsv1alpha1.ModelSpec{{Name: "test"}},
			},
			expectError: false,
		},
		{
			name: "ConfigWithArrays",
			provider: olsv1alpha1.ProviderSpec{
				Name:         "arrays",
				Type:         "generic",
				ProviderType: "remote::custom",
				Config: &runtime.RawExtension{
					Raw: []byte(`{"models": ["model1", "model2"], "endpoints": [{"url": "https://test"}]}`),
				},
				Models: []olsv1alpha1.ModelSpec{{Name: "test"}},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &olsv1alpha1.OLSConfig{
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{tt.provider},
					},
				},
			}

			ctx := context.Background()
			yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none. Output: %s", yamlOutput)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// Verify output is valid YAML
				var result map[string]interface{}
				if err := yaml.Unmarshal([]byte(yamlOutput), &result); err != nil {
					t.Errorf("buildLlamaStackYAML produced invalid YAML: %v", err)
				}
			}
		})
	}
}

// TestProviderNameToEnvVarConversion tests environment variable name conversion edge cases
func TestProviderNameToEnvVarConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "SIMPLE"},
		{"with-hyphens", "WITH_HYPHENS"},
		{"multiple-hyphens-here", "MULTIPLE_HYPHENS_HERE"},
		{"MixedCase", "MIXEDCASE"},
		{"trailing-", "TRAILING_"},
		{"-leading", "_LEADING"},
		{"multiple--hyphens", "MULTIPLE__HYPHENS"},
		{"already_underscore", "ALREADY_UNDERSCORE"},
		{"123numeric", "123NUMERIC"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("convert_%s", tt.input), func(t *testing.T) {
			result := utils.ProviderNameToEnvVarName(tt.input)
			if result != tt.expected {
				t.Errorf("ProviderNameToEnvVarName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGenericProviderCredentialInjection tests that credentials are properly injected
func TestGenericProviderCredentialInjection(t *testing.T) {
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name:          "test-provider",
						Type:          "generic",
						ProviderType:  "remote::test-backend",
						CredentialKey: "secret_key",
						Config: &runtime.RawExtension{
							Raw: []byte(`{"api_endpoint": "https://api.example.com"}`),
						},
						Models: []olsv1alpha1.ModelSpec{{Name: "test-model"}},
					},
				},
			},
		},
	}

	ctx := context.Background()
	yamlOutput, err := buildLlamaStackYAML(nil, ctx, cr)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML failed: %v", err)
	}

	// Verify that the custom credential key is referenced in environment variables
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlOutput), &result); err != nil {
		t.Fatalf("Invalid YAML produced: %v", err)
	}

	// Check that environment variable substitution is present
	if !strings.Contains(yamlOutput, "TEST_PROVIDER_API_KEY") {
		t.Errorf("Expected environment variable reference 'TEST_PROVIDER_API_KEY' in output")
	}
}

package lcore

import (
	"context"
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
	// Test unsupported providers
	unsupportedProviders := []string{"rhoai_vllm", "rhelai_vllm"}

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

func TestBuildLlamaStackYAML_WatsonxProvider(t *testing.T) {
	// Create a fake secret with API token for Watsonx provider
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "watsonx-secret",
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

	// Create a test CR with Watsonx provider
	cr := &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name:            "watsonx",
						Type:            "watsonx",
						URL:             "https://us-south.ml.cloud.ibm.com",
						WatsonProjectID: "my-project-id",
						Models: []olsv1alpha1.ModelSpec{
							{
								Name:              "ibm/granite-13b-chat-v2",
								ContextWindowSize: 8192,
							},
						},
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "watsonx-secret",
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
		t.Fatalf("buildLlamaStackYAML returned error for Watsonx provider: %v", err)
	}

	// Verify it's valid YAML
	var result map[string]interface{}
	err = yaml.Unmarshal([]byte(yamlOutput), &result)
	if err != nil {
		t.Fatalf("buildLlamaStackYAML produced invalid YAML: %v", err)
	}

	// Verify Watsonx provider configuration
	providers, ok := result["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("providers section not found or invalid type")
	}

	inference, ok := providers["inference"].([]interface{})
	if !ok || len(inference) == 0 {
		t.Fatalf("inference providers not found or empty")
	}

	// Find the Watsonx provider (not the sentence-transformers one)
	var watsonxProvider map[string]interface{}
	for _, provider := range inference {
		p, ok := provider.(map[string]interface{})
		if !ok {
			continue
		}
		if p["provider_type"] == "remote::watsonx" {
			watsonxProvider = p
			break
		}
	}

	if watsonxProvider == nil {
		t.Fatalf("Watsonx provider not found in inference providers")
	}

	// Check provider_type
	if watsonxProvider["provider_type"] != "remote::watsonx" {
		t.Errorf("Expected provider_type 'remote::watsonx', got '%v'", watsonxProvider["provider_type"])
	}

	// Check config fields
	config, ok := watsonxProvider["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("provider config not found or invalid type")
	}

	// Verify Watsonx-specific fields are present
	requiredFields := []string{
		"api_key",    // API key
		"project_id", // Watsonx project ID
		"base_url",   // Watsonx endpoint (Llama Stack uses base_url not url)
	}
	for _, field := range requiredFields {
		if _, exists := config[field]; !exists {
			t.Errorf("Expected field '%s' not found in Watsonx provider config", field)
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

	// Verify project_id is set correctly
	if projectID, ok := config["project_id"].(string); ok {
		if projectID != "my-project-id" {
			t.Errorf("Expected project_id 'my-project-id', got '%s'", projectID)
		}
	} else {
		t.Errorf("project_id field is missing or invalid type")
	}

	t.Logf("Successfully validated Llama Stack YAML with Watsonx provider (%d bytes)", len(yamlOutput))
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

	ctx := context.Background()
	yamlOutput, err := buildLCoreConfigYAML(testReconciler, ctx, cr)
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

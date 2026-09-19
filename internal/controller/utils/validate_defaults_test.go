package utils

import (
	"testing"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func TestValidateDefaultProviderAndModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*olsv1alpha1.OLSConfig)
		wantErr string
	}{
		{
			name: "matching provider and model",
		},
		{
			name: "unknown defaultProvider",
			mutate: func(cr *olsv1alpha1.OLSConfig) {
				cr.Spec.OLSConfig.DefaultProvider = "missing"
			},
			wantErr: `defaultProvider "missing" does not match any spec.llm.providers[].name`,
		},
		{
			name: "unknown defaultModel on known provider",
			mutate: func(cr *olsv1alpha1.OLSConfig) {
				cr.Spec.OLSConfig.DefaultModel = "not-a-model"
			},
			wantErr: `defaultModel "not-a-model" is not a model on provider "testProvider"`,
		},
		{
			name: "model exists on a different provider",
			mutate: func(cr *olsv1alpha1.OLSConfig) {
				cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
					Name:   "other",
					Type:   "openai",
					Models: []olsv1alpha1.ModelSpec{{Name: "other-model"}},
				})
				cr.Spec.OLSConfig.DefaultProvider = "other"
				cr.Spec.OLSConfig.DefaultModel = "testModel"
			},
			wantErr: `defaultModel "testModel" is not a model on provider "other"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := GetDefaultOLSConfigCR()
			if tt.mutate != nil {
				tt.mutate(cr)
			}
			err := ValidateDefaultProviderAndModel(cr)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateDefaultProviderAndModel() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateDefaultProviderAndModel() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("ValidateDefaultProviderAndModel() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateDefaultProviderAndModelNilCR(t *testing.T) {
	t.Parallel()
	err := ValidateDefaultProviderAndModel(nil)
	if err == nil || err.Error() != "OLSConfig is nil" {
		t.Fatalf("ValidateDefaultProviderAndModel(nil) error = %v, want OLSConfig is nil", err)
	}
}

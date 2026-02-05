package lcore

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// genericProvider builds a llamaStackGeneric ProviderSpec.
// Pass secretName="" to simulate a public/unauthenticated endpoint.
// Pass configRaw=nil to omit the Config field entirely.
func genericProvider(name, providerType string, configRaw []byte, secretName string) olsv1alpha1.ProviderSpec {
	p := olsv1alpha1.ProviderSpec{
		Name:                 name,
		Type:                 utils.LlamaStackGenericType,
		ProviderType:         providerType,
		CredentialsSecretRef: corev1.LocalObjectReference{Name: secretName},
		Models:               []olsv1alpha1.ModelSpec{{Name: "test-model"}},
	}
	if configRaw != nil {
		p.Config = &runtime.RawExtension{Raw: configRaw}
	}
	return p
}

// crWith wraps providers into a minimal OLSConfig.
func crWith(providers ...olsv1alpha1.ProviderSpec) *olsv1alpha1.OLSConfig {
	return &olsv1alpha1.OLSConfig{
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{Providers: providers},
		},
	}
}

// providerConfig extracts the "config" map from providers[idx].
// [0] is always sentence-transformers; user providers start at [1].
func providerConfig(providers []interface{}, idx int) map[string]interface{} {
	return providers[idx].(map[string]interface{})["config"].(map[string]interface{})
}

// ─── tests ──────────────────────────────────────────────────────────────────

var _ = Describe("Generic provider", func() {

	// ── buildLlamaStackInferenceProviders ────────────────────────────────────
	Describe("buildLlamaStackInferenceProviders", func() {

		It("returns only sentence-transformers for a nil CR", func() {
			providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(providers).To(HaveLen(1))
			Expect(providers[0].(map[string]interface{})["provider_id"]).To(Equal("sentence-transformers"))
		})

		It("returns a clear error for invalid JSON in config", func() {
			cr := crWith(genericProvider("p", "remote::openai", []byte(`{invalid`), "s"))
			_, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmarshal config"))
		})

		It("auto-injects api_key when credentials are configured", func() {
			cr := crWith(genericProvider("my-provider", "remote::openai",
				[]byte(`{"url":"https://example.com"}`), "my-secret"))

			providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			// [0] sentence-transformers, [1] my-provider
			Expect(providers).To(HaveLen(2))

			p := providers[1].(map[string]interface{})
			Expect(p["provider_id"]).To(Equal("my-provider"))
			Expect(p["provider_type"]).To(Equal("remote::openai"))
			Expect(providerConfig(providers, 1)["api_key"]).To(Equal("${env.MY_PROVIDER_API_KEY}"))
		})

		It("does not overwrite an api_key the user already supplied", func() {
			cr := crWith(genericProvider("my-provider", "remote::openai",
				[]byte(`{"api_key":"${env.CUSTOM_KEY}"}`), "my-secret"))

			providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(providerConfig(providers, 1)["api_key"]).To(Equal("${env.CUSTOM_KEY}"))
		})

		It("does not inject api_key for a public/unauthenticated provider", func() {
			cr := crWith(genericProvider("pub", "remote::ollama",
				[]byte(`{"url":"http://localhost:11434"}`), "" /* no secret */))

			providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(providerConfig(providers, 1)).NotTo(HaveKey("api_key"))
		})

		It("injects api_key even for an empty config object {}", func() {
			cr := crWith(genericProvider("p", "remote::openai", []byte(`{}`), "my-secret"))

			providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(providerConfig(providers, 1)).To(HaveKey("api_key"))
		})

		It("configures multiple generic providers independently", func() {
			cr := crWith(
				genericProvider("alpha", "remote::openai",
					[]byte(`{"url":"https://alpha.example.com"}`), "s-alpha"),
				genericProvider("beta", "remote::vllm",
					[]byte(`{"url":"https://beta.example.com"}`), "s-beta"),
			)

			providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			// [0] sentence-transformers, [1] alpha, [2] beta
			Expect(providers).To(HaveLen(3))

			alpha := providers[1].(map[string]interface{})
			Expect(alpha["provider_id"]).To(Equal("alpha"))
			Expect(alpha["provider_type"]).To(Equal("remote::openai"))

			beta := providers[2].(map[string]interface{})
			Expect(beta["provider_id"]).To(Equal("beta"))
			Expect(beta["provider_type"]).To(Equal("remote::vllm"))

			Expect(providerConfig(providers, 1)["api_key"]).To(Equal("${env.ALPHA_API_KEY}"))
			Expect(providerConfig(providers, 2)["api_key"]).To(Equal("${env.BETA_API_KEY}"))
		})

		DescribeTable("produces the correct env-var substitution for various provider names",
			func(providerName, wantAPIKey string) {
				cr := crWith(genericProvider(providerName, "remote::openai", []byte(`{}`), "s"))
				providers, err := buildLlamaStackInferenceProviders(nil, context.Background(), cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(providerConfig(providers, 1)["api_key"]).To(Equal(wantAPIKey))
			},
			Entry("hyphenated name", "my-openai", "${env.MY_OPENAI_API_KEY}"),
			Entry("multiple hyphens", "provider-with-hyphens", "${env.PROVIDER_WITH_HYPHENS_API_KEY}"),
			Entry("plain lowercase", "simpleprovider", "${env.SIMPLEPROVIDER_API_KEY}"),
			Entry("leading digits are prefixed with underscore", "123provider", "${env._123PROVIDER_API_KEY}"),
		)
	})

	// ── buildLlamaStackYAML ──────────────────────────────────────────────────
	Describe("buildLlamaStackYAML", func() {

		It("propagates a generic-provider invalid-JSON error to the caller", func() {
			cr := crWith(genericProvider("p", "remote::openai", []byte(`{invalid`), "s"))
			_, err := buildLlamaStackYAML(nil, context.Background(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build inference providers"))
		})

		It("generates YAML containing the credential injection placeholder for a generic provider", func() {
			cr := crWith(genericProvider("my-provider", "remote::openai",
				[]byte(`{"url":"https://example.com"}`), "my-secret"))

			yamlStr, err := buildLlamaStackYAML(nil, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			// The auto-injected api_key must use the env-var substitution pattern.
			Expect(yamlStr).To(ContainSubstring("${env.MY_PROVIDER_API_KEY}"))
			// Provider metadata must appear in the YAML.
			Expect(yamlStr).To(ContainSubstring("remote::openai"))
			Expect(yamlStr).To(ContainSubstring("my-provider"))
		})
	})

	// ── buildLCoreConfigYAML ─────────────────────────────────────────────────
	Describe("buildLCoreConfigYAML", func() {

		It("uses the generic provider name as inference default_provider", func() {
			cr := crWith(genericProvider("my-generic-provider", "remote::openai", []byte(`{}`), ""))
			cr.Spec.OLSConfig.DefaultProvider = "my-generic-provider"
			cr.Spec.OLSConfig.DefaultModel = "test-model"

			yamlStr, err := buildLCoreConfigYAML(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(yamlStr).To(ContainSubstring("my-generic-provider"))
			Expect(yamlStr).To(ContainSubstring("test-model"))
		})
	})

	// ── getProviderType ──────────────────────────────────────────────────────
	Describe("getProviderType", func() {

		DescribeTable("returns an error for unsupported or misused provider types",
			func(providerType, expectedMsg string) {
				p := &olsv1alpha1.ProviderSpec{Name: "test", Type: providerType}
				_, err := getProviderType(p)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedMsg))
			},
			Entry("llamaStackGeneric without providerType field",
				utils.LlamaStackGenericType, "requires providerType and config fields"),
			Entry("watsonx is unsupported", "watsonx", "not currently supported by Llama Stack"),
			Entry("bam is unsupported", "bam", "not currently supported by Llama Stack"),
			Entry("completely unknown type", "totally-unknown", "unknown provider type"),
		)
	})

	// ── deepCopyMap ──────────────────────────────────────────────────────────
	Describe("deepCopyMap", func() {

		It("returns nil for a nil input", func() {
			Expect(deepCopyMap(nil)).To(BeNil())
		})

		It("returns an empty (non-nil) map for an empty input", func() {
			result := deepCopyMap(map[string]interface{}{})
			Expect(result).NotTo(BeNil())
			Expect(result).To(BeEmpty())
		})

		It("copies primitive values correctly", func() {
			src := map[string]interface{}{"s": "hello", "n": 42, "b": true}
			Expect(deepCopyMap(src)).To(Equal(src))
		})

		It("prevents mutation of nested maps", func() {
			src := map[string]interface{}{
				"nested": map[string]interface{}{"key": "original"},
			}
			result := deepCopyMap(src)
			result["nested"].(map[string]interface{})["key"] = "modified"

			Expect(src["nested"].(map[string]interface{})["key"]).To(Equal("original"),
				"modifying the copy must not affect the original")
		})

		It("prevents mutation of slice values", func() {
			src := map[string]interface{}{"items": []interface{}{"a", "b", "c"}}
			result := deepCopyMap(src)
			result["items"].([]interface{})[0] = "modified"

			Expect(src["items"].([]interface{})[0]).To(Equal("a"),
				"modifying the copy's slice must not affect the original")
		})

		It("deep-copies three levels of nesting", func() {
			src := map[string]interface{}{
				"l1": map[string]interface{}{
					"l2": map[string]interface{}{"value": "deep"},
				},
			}
			result := deepCopyMap(src)
			result["l1"].(map[string]interface{})["l2"].(map[string]interface{})["value"] = "changed"

			got := src["l1"].(map[string]interface{})["l2"].(map[string]interface{})["value"]
			Expect(got).To(Equal("deep"), "3-level deep value must not be mutated")
		})

		It("deep-copies mixed nested slices and maps", func() {
			src := map[string]interface{}{
				"config": map[string]interface{}{
					"tags": []interface{}{"tag1", "tag2"},
					"meta": map[string]interface{}{"version": "1.0"},
				},
			}
			result := deepCopyMap(src)
			result["config"].(map[string]interface{})["tags"].([]interface{})[0] = "mutated"
			result["config"].(map[string]interface{})["meta"].(map[string]interface{})["version"] = "2.0"

			Expect(src["config"].(map[string]interface{})["tags"].([]interface{})[0]).To(Equal("tag1"))
			Expect(src["config"].(map[string]interface{})["meta"].(map[string]interface{})["version"]).To(Equal("1.0"))
		})
	})

	// ── CRD validation – ProviderSpec CEL rules ──────────────────────────────
	Describe("CRD validation – ProviderSpec CEL rules", Ordered, func() {

		// validGenericProvider returns a fully valid llamaStackGeneric ProviderSpec
		// that satisfies all five CRD CEL rules.
		validGenericProvider := func() olsv1alpha1.ProviderSpec {
			return olsv1alpha1.ProviderSpec{
				Name:                 "test-provider",
				Type:                 utils.LlamaStackGenericType,
				ProviderType:         "remote::openai",
				Config:               &runtime.RawExtension{Raw: []byte(`{}`)},
				CredentialsSecretRef: corev1.LocalObjectReference{Name: "test-secret"},
				Models:               []olsv1alpha1.ModelSpec{{Name: "test-model"}},
			}
		}

		// withInvalidProvider attempts to update the singleton "cluster" CR with the
		// given provider spec.  Because the CRD enforces name == "cluster", all
		// tests use Update so the name constraint never interferes with the CEL
		// assertion under test.
		withInvalidProvider := func(provider olsv1alpha1.ProviderSpec) error {
			existing := &olsv1alpha1.OLSConfig{}
			Expect(k8sClient.Get(ctx, crNamespacedName, existing)).To(Succeed())
			updated := existing.DeepCopy()
			updated.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{provider}
			return k8sClient.Update(ctx, updated)
		}

		// Capture the original spec once before any test in this block runs and
		// restore it afterwards so the reconciler tests that follow are unaffected.
		var savedSpec olsv1alpha1.OLSConfigSpec

		BeforeAll(func() {
			existing := &olsv1alpha1.OLSConfig{}
			Expect(k8sClient.Get(ctx, crNamespacedName, existing)).To(Succeed())
			savedSpec = *existing.Spec.DeepCopy()
		})

		AfterAll(func() {
			existing := &olsv1alpha1.OLSConfig{}
			Expect(k8sClient.Get(ctx, crNamespacedName, existing)).To(Succeed())
			restored := existing.DeepCopy()
			restored.Spec = savedSpec
			Expect(k8sClient.Update(ctx, restored)).To(Succeed())
		})

		// Rule 1: !has(self.providerType) || has(self.config)
		It("rejects a provider that sets providerType without config (Rule 1)", func() {
			p := validGenericProvider()
			p.Config = nil // violates Rule 1
			Expect(withInvalidProvider(p)).To(HaveOccurred())
		})

		// Rule 2: !has(self.config) || has(self.providerType)
		It("rejects a provider that sets config without providerType (Rule 2)", func() {
			p := validGenericProvider()
			p.ProviderType = "" // violates Rule 2
			err := withInvalidProvider(p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config"))
		})

		// Rule 3: !has(self.providerType) || self.type == "llamaStackGeneric"
		It("rejects a provider that sets providerType with type != llamaStackGeneric (Rule 3)", func() {
			p := validGenericProvider()
			p.Type = "openai" // violates Rule 3
			err := withInvalidProvider(p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("llamaStackGeneric"))
		})

		// Rule 4: self.type != "llamaStackGeneric" || (!has(self.deploymentName) && !has(self.projectID) && !has(self.url) && !has(self.apiVersion))
		It("rejects a llamaStackGeneric provider that also sets deploymentName (Rule 4)", func() {
			p := validGenericProvider()
			p.AzureDeploymentName = "my-deployment" // violates Rule 4
			err := withInvalidProvider(p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("legacy"))
		})

		It("rejects a llamaStackGeneric provider that also sets url (Rule 4)", func() {
			p := validGenericProvider()
			p.URL = "https://my-endpoint.example.com" // violates Rule 4
			err := withInvalidProvider(p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("legacy"))
		})

		It("rejects a llamaStackGeneric provider that also sets apiVersion (Rule 4)", func() {
			p := validGenericProvider()
			p.APIVersion = "2024-01" // violates Rule 4
			err := withInvalidProvider(p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("legacy"))
		})

		// Rule 5: !has(self.credentialKey) || !self.credentialKey.matches('^[ \t\n\r\v\f]*$')
		It("rejects a provider with a whitespace-only credentialKey (Rule 5)", func() {
			p := validGenericProvider()
			p.CredentialKey = "   " // violates Rule 5
			err := withInvalidProvider(p)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialKey"))
		})

		// Positive: a fully valid generic provider must be accepted.
		// This is the only test that successfully mutates the CR; AfterAll restores it.
		It("accepts a fully valid llamaStackGeneric provider", func() {
			existing := &olsv1alpha1.OLSConfig{}
			Expect(k8sClient.Get(ctx, crNamespacedName, existing)).To(Succeed())
			good := existing.DeepCopy()
			good.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{validGenericProvider()}
			good.Spec.OLSConfig.DefaultProvider = "test-provider"
			good.Spec.OLSConfig.DefaultModel = "test-model"
			Expect(k8sClient.Update(ctx, good)).To(Succeed())
		})
	})
})

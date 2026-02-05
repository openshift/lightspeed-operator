package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// createTestSecret creates a Secret in the test namespace and registers a
// DeferCleanup to delete it at the end of the enclosing It block.
// This eliminates the need for a shared AfterEach + nil-check pattern.
func createTestSecret(name string, data map[string][]byte) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: OLSNamespaceDefault},
		Data:       data,
	}
	Expect(k8sClient.Create(testCtx, s)).To(Succeed())
	DeferCleanup(k8sClient.Delete, testCtx, s)
}

// newGenericReconciler returns a fresh TestReconciler with LCore enabled.
// Calling this at the start of each It block prevents state leaking between tests.
func newGenericReconciler() *TestReconciler {
	r := NewTestReconciler(k8sClient, logf.Log.WithName("test"), k8sClient.Scheme(), OLSNamespaceDefault)
	r.SetUseLCore(true)
	return r
}

// ─── ProviderNameToEnvVarName – edge cases ───────────────────────────────────

var _ = Describe("ProviderNameToEnvVarName", func() {

	DescribeTable("converts provider names to valid env-var identifiers",
		func(input, expected string) {
			Expect(ProviderNameToEnvVarName(input)).To(Equal(expected))
		},
		// Existing cases (canonical examples)
		Entry("hyphen becomes underscore", "my-provider", "MY_PROVIDER"),
		Entry("all uppercase passthrough", "PROVIDER", "PROVIDER"),
		Entry("lowercase to uppercase", "provider", "PROVIDER"),
		Entry("empty string gets underscore prefix (POSIX compliance)", "", "_"),
		Entry("mixed case with hyphen", "OpenAI-Provider", "OPENAI_PROVIDER"),
		// Edge cases with leading digits
		Entry("leading digit gets underscore prefix (POSIX compliance)", "123provider", "_123PROVIDER"),
		Entry("digit-only name gets underscore prefix (POSIX compliance)", "12345", "_12345"),
		// Special character stripping
		Entry("dot is stripped (not alphanumeric/hyphen/underscore)", "my.provider", "MYPROVIDER"),
		Entry("at-sign is stripped", "provider@test", "PROVIDERTEST"),
		// All special characters result in underscore (collision prevention)
		Entry("all special characters sanitize to empty, then prefix", "!@#$%", "_"),
		Entry("whitespace-only sanitizes to empty, then prefix", "   ", "_"),
	)
})

// ─── ValidateLLMCredentials – generic provider scenarios ─────────────────────

var _ = Describe("ValidateLLMCredentials – generic provider", func() {

	It("fails when the credential secret does not exist", func() {
		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialKey = DefaultCredentialKey
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "does-not-exist"

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does-not-exist not found"))
	})

	It("succeeds when no credentials are configured (public/unauthenticated endpoint)", func() {
		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::ollama"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = ""

		Expect(ValidateLLMCredentials(r, testCtx, cr)).To(Succeed())
	})

	It("fails when public generic provider has malformed JSON config", func() {
		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::ollama"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = ""
		cr.Spec.LLMConfig.Providers[0].Config = &runtime.RawExtension{Raw: []byte(`{invalid json`)}

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not valid JSON"))
	})

	It("succeeds when public generic provider has valid JSON config", func() {
		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::ollama"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = ""
		cr.Spec.LLMConfig.Providers[0].Config = &runtime.RawExtension{Raw: []byte(`{"url":"http://localhost:11434"}`)}

		Expect(ValidateLLMCredentials(r, testCtx, cr)).To(Succeed())
	})

	It("succeeds when public generic provider has no config at all", func() {
		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::ollama"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = ""
		cr.Spec.LLMConfig.Providers[0].Config = nil

		Expect(ValidateLLMCredentials(r, testCtx, cr)).To(Succeed())
	})

	It("succeeds with a custom credentialKey that exists in the secret", func() {
		createTestSecret("gen-custom-key-secret", map[string][]byte{
			"bearer_token": []byte("tok"),
		})

		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialKey = "bearer_token"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "gen-custom-key-secret"

		Expect(ValidateLLMCredentials(r, testCtx, cr)).To(Succeed())
	})

	It("succeeds with the default credentialKey (apitoken) when credentialKey is omitted", func() {
		createTestSecret("gen-default-key-secret", map[string][]byte{
			DefaultCredentialKey: []byte("tok"),
		})

		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialKey = "" // omitted → defaults to "apitoken"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "gen-default-key-secret"

		Expect(ValidateLLMCredentials(r, testCtx, cr)).To(Succeed())
	})

	It("fails when the secret exists but lacks the specified credentialKey", func() {
		createTestSecret("gen-missing-key-secret", map[string][]byte{
			"wrong_key": []byte("tok"),
		})

		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialKey = "expected_key"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "gen-missing-key-secret"

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing key 'expected_key'"))
	})

	It("fails at runtime when credentialKey is whitespace-only", func() {
		createTestSecret("gen-whitespace-key-secret", map[string][]byte{
			DefaultCredentialKey: []byte("tok"),
		})

		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialKey = "   "
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "gen-whitespace-key-secret"

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("credentialKey must not be empty or whitespace"))
	})

	It("fails when config contains malformed JSON", func() {
		createTestSecret("gen-invalid-json-secret", map[string][]byte{
			DefaultCredentialKey: []byte("tok"),
		})

		r := newGenericReconciler()
		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialKey = DefaultCredentialKey
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "gen-invalid-json-secret"
		cr.Spec.LLMConfig.Providers[0].Config = &runtime.RawExtension{Raw: []byte(`{invalid`)}

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not valid JSON"))
	})

	It("fails when generic provider requires LCore backend but LCore is disabled", func() {
		createTestSecret("gen-lcore-required-secret", map[string][]byte{
			DefaultCredentialKey: []byte("tok"),
		})

		r := newGenericReconciler()
		r.SetUseLCore(false) // override to disable LCore

		cr := GetDefaultOLSConfigCR()
		cr.Spec.LLMConfig.Providers[0].Type = LlamaStackGenericType
		cr.Spec.LLMConfig.Providers[0].ProviderType = "remote::openai"
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "gen-lcore-required-secret"

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requires LCore backend"))
		Expect(err.Error()).To(ContainSubstring("--enable-lcore"))
	})

	It("fails on the generic provider even when a valid legacy provider precedes it", func() {
		// Scenario: [legacy openai, generic llamaStackGeneric] with LCore disabled.
		// The legacy provider passes secret validation; the generic provider must
		// still be rejected because LCore is disabled.
		createTestSecret("mix-legacy-secret", map[string][]byte{
			DefaultCredentialKey: []byte("tok"),
		})

		r := newGenericReconciler()
		r.SetUseLCore(false)

		cr := GetDefaultOLSConfigCR()
		// Make Provider[0] a valid legacy openai provider so it passes validation.
		cr.Spec.LLMConfig.Providers[0].Type = "openai"
		cr.Spec.LLMConfig.Providers[0].ProviderType = ""
		cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "mix-legacy-secret"

		// Append Provider[1] as a llamaStackGeneric provider.
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name:                 "generic-mixed",
			Type:                 LlamaStackGenericType,
			ProviderType:         "remote::openai",
			CredentialsSecretRef: corev1.LocalObjectReference{Name: "mix-legacy-secret"},
			Models:               []olsv1alpha1.ModelSpec{{Name: "test-model"}},
		})

		err := ValidateLLMCredentials(r, testCtx, cr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requires LCore backend"))
		Expect(err.Error()).To(ContainSubstring("generic-mixed"))
	})
})

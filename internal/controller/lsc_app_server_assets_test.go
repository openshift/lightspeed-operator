package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("LSC App server assets", Label("LSCBackend"), Ordered, func() {

	const (
		testURL = "https://testURL"
	)
	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions
	var ctx context.Context

	addAzureOpenAIProvider := func(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name:       "testProviderAzureOpenAI",
			URL:        testURL,
			Type:       AzureOpenAIType,
			APIVersion: "testAzureVersion",
		})
		return cr
	}

	addOpenAIProvider := func(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name: "testProviderOpenAI",
			URL:  testURL,
			Type: OpenAIType,
		})
		return cr
	}

	addWatsonxProvider := func(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name:            "testProviderWatsonX",
			URL:             testURL,
			Type:            WatsonXType,
			WatsonProjectID: "testProjectID",
		})
		return cr
	}

	addRHOAIProvider := func(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name: "testProviderRHOAI",
			URL:  testURL,
			Type: RHOAIType,
		})
		return cr
	}

	addRHELAIProvider := func(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name: "testProviderRHELAI",
			URL:  testURL,
			Type: RHELAIType,
		})
		return cr
	}

	addUnknownProvider := func(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
		cr.Spec.LLMConfig.Providers = append(cr.Spec.LLMConfig.Providers, olsv1alpha1.ProviderSpec{
			Name: "testProviderUnknown",
			URL:  testURL,
			Type: "unknown",
		})
		return cr
	}

	Context("LSC asset generation", func() {
		BeforeEach(func() {
			ctx = context.Background()
			rOptions = &OLSConfigReconcilerOptions{
				OpenShiftMajor:          "123",
				OpenshiftMinor:          "456",
				LightspeedServiceImage:  "lightspeed-service:latest",
				OpenShiftMCPServerImage: "openshift-mcp-server:latest",
				Namespace:               OLSNamespaceDefault,
			}
			cr = getDefaultOLSConfigCR()
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
		})

		Describe("generateLSCConfigMap", func() {
			It("should generate a valid configmap", func() {
				cm, err := r.generateLSCConfigMap(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(cm).NotTo(BeNil())
			})

			// TODO: Add more tests cases for once implementation is complete
		})

		Describe("generateLSCDeployment", func() {
			It("should generate a valid deployment", func() {
				deployment, err := r.generateLSCDeployment(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(deployment).NotTo(BeNil())
			})

			// TODO: Add more tests cases for once implementation is complete
		})

		Describe("updateLSCDeployment", func() {
			var existingDeployment *appsv1.Deployment
			var desiredDeployment *appsv1.Deployment

			BeforeEach(func() {
				existingDeployment, _ = r.generateLSCDeployment(ctx, cr)
			})

			It("should successfully update deployment", func() {
				desiredDeployment, _ = r.generateLSCDeployment(ctx, cr)
				err := r.updateLSCDeployment(ctx, existingDeployment, desiredDeployment)
				Expect(err).NotTo(HaveOccurred())
			})

			// TODO: Add more tests cases for once implementation is complete
		})
	})

	Context("Llama stack config file generation", func() {

		BeforeEach(func() {
			ctx = context.Background()
			rOptions = &OLSConfigReconcilerOptions{
				OpenShiftMajor:          "123",
				OpenshiftMinor:          "456",
				LightspeedServiceImage:  "lightspeed-service:latest",
				OpenShiftMCPServerImage: "openshift-mcp-server:latest",
				Namespace:               OLSNamespaceDefault,
			}
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
			cr = getEmptyOLSConfigCR()
		})

		Describe("Inference Provider", func() {
			It("should generate a valid llama stack config file for OpenAI provider", func() {
				cr = addOpenAIProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				ExpectedConfigFile := `providers:
  inference:
  - config:
      api_key: ${env.OPENAI_API_KEY}
      base_url: https://testURL
    provider_id: testProviderOpenAI
    provider_type: remote::openai
version: "2"
`
				Expect(llamaStackConfigFile).To(Equal(ExpectedConfigFile))
			})

			It("should generate a valid llama stack config file for Azure OpenAI provider", func() {
				cr = addAzureOpenAIProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(llamaStackConfigFile).NotTo(BeEmpty())
				ExpectedConfigFile := `providers:
  inference:
  - config:
      api_base: https://testURL
      api_key: ${env.AZURE_OPENAI_API_KEY}
      api_version: testAzureVersion
    provider_id: testProviderAzureOpenAI
    provider_type: remote::azure_openai
version: "2"
`
				Expect(llamaStackConfigFile).To(Equal(ExpectedConfigFile))
			})

			It("should generate a valid llama stack config file for Watson X provider", func() {
				cr = addWatsonxProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				ExpectedConfigFile := `providers:
  inference:
  - config:
      api_key: ${env.WATSONX_API_KEY}
      project_id: testProjectID
      url: https://testURL
    provider_id: testProviderWatsonX
    provider_type: remote::watsonx
version: "2"
`
				Expect(llamaStackConfigFile).To(Equal(ExpectedConfigFile))
			})

			It("should generate a valid llama stack config file for RHOAI provider", func() {
				cr = addRHOAIProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				ExpectedConfigFile := `providers:
  inference:
  - config:
      api_token: ${env.RHOAI_API_TOKEN}
      url: https://testURL
    provider_id: testProviderRHOAI
    provider_type: remote::vllm
version: "2"
`
				Expect(llamaStackConfigFile).To(Equal(ExpectedConfigFile))
			})

			It("should generate a valid llama stack config file for RHELAI provider", func() {
				cr = addRHELAIProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				ExpectedConfigFile := `providers:
  inference:
  - config:
      api_token: ${env.RHELAI_API_TOKEN}
      url: https://testURL
    provider_id: testProviderRHELAI
    provider_type: remote::vllm
version: "2"
`
				Expect(llamaStackConfigFile).To(Equal(ExpectedConfigFile))
			})

			It("should return an error for an unsupported provider type", func() {
				cr = addUnknownProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).To(MatchError(ContainSubstring("unsupported provider type")))
				Expect(llamaStackConfigFile).To(BeEmpty())
			})

			It("should generate a valid llama stack config file for multiple providers", func() {
				cr = addOpenAIProvider(cr)
				cr = addWatsonxProvider(cr)
				llamaStackConfigFile, err := r.generateLlamaStackConfigFile(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				ExpectedConfigFile := `providers:
  inference:
  - config:
      api_key: ${env.OPENAI_API_KEY}
      base_url: https://testURL
    provider_id: testProviderOpenAI
    provider_type: remote::openai
  - config:
      api_key: ${env.WATSONX_API_KEY}
      project_id: testProjectID
      url: https://testURL
    provider_id: testProviderWatsonX
    provider_type: remote::watsonx
version: "2"
`
				Expect(llamaStackConfigFile).To(Equal(ExpectedConfigFile))
			})
		})

		// TODO: Add more tests cases for once implementation is complete
	})
})

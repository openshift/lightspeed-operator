package appserver

import (
	"context"
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	networkingv1 "k8s.io/api/networking/v1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var testURL = "https://testURL"
var defaultVolumeMode = utils.VolumeDefaultMode

var _ = Describe("App server assets", func() {
	var cr *olsv1alpha1.OLSConfig
	var secret *corev1.Secret
	var configmap *corev1.ConfigMap

	Context("complete custom resource", func() {
		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
			configmap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Configmap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.DefaultOpenShiftCerts,
				},
			})
			configMapCreationErr := testReconcilerInstance.Create(ctx, configmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate a service account", func() {
			sa, err := GenerateServiceAccount(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(sa.Name).To(Equal(utils.OLSAppServerServiceAccountName))
			Expect(sa.Namespace).To(Equal(utils.OLSNamespaceDefault))
		})

		It("should generate the olsconfig config map", func() {
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			major, minor, err := utils.GetOpenshiftVersion(k8sClient, ctx)
			Expect(err).NotTo(HaveOccurred())

			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			olsconfigGenerated := utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			olsConfigExpected := utils.AppSrvConfigFile{
				OLSConfig: utils.OLSConfig{
					DefaultModel:    "testModel",
					DefaultProvider: "testProvider",
					Logging: utils.LoggingConfig{
						AppLogLevel:     utils.LogLevelInfo,
						LibLogLevel:     utils.LogLevelInfo,
						UvicornLogLevel: utils.LogLevelInfo,
					},
					ConversationCache: utils.ConversationCacheConfig{
						Type:     utils.OLSDefaultCacheType,
						Postgres: utils.GetTestPostgresCacheConfig(),
					},
					TLSConfig: utils.TLSConfig{
						TLSCertificatePath: path.Join(utils.OLSAppCertsMountRoot, utils.OLSCertsSecretName, "tls.crt"),
						TLSKeyPath:         path.Join(utils.OLSAppCertsMountRoot, utils.OLSCertsSecretName, "tls.key"),
					},
					ReferenceContent: utils.ReferenceContent{
						EmbeddingsModelPath: "/app-root/embeddings_model",
						Indexes: []utils.ReferenceIndex{
							{
								ProductDocsIndexId:   "ocp-product-docs-" + major + "_" + minor,
								ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/" + major + "." + minor,
								ProductDocsOrigin:    "Red Hat OpenShift 123.456 documentation",
							},
						},
					},
					UserDataCollection: utils.UserDataCollectionConfig{
						FeedbackDisabled:    false,
						FeedbackStorage:     "/app-root/ols-user-data/feedback",
						TranscriptsDisabled: false,
						TranscriptsStorage:  "/app-root/ols-user-data/transcripts",
					},
					ExtraCAs: []string{
						"/etc/certs/ols-additional-ca/service-ca.crt",
					},
					CertificateDirectory: "/etc/certs/cert-bundle",
				},
				LLMProviders: []utils.ProviderConfig{
					{
						Name:            "testProvider",
						URL:             testURL,
						CredentialsPath: "/etc/apikeys/test-secret",
						Type:            "bam",
						Models: []utils.ModelConfig{
							{
								Name: "testModel",
								URL:  testURL,
								Parameters: utils.ModelParameters{
									MaxTokensForResponse: 20,
								},
								ContextWindowSize: 32768,
							},
						},
					},
				},
				UserDataCollectorConfig: utils.UserDataCollectorConfig{
					DataStorage: "/app-root/ols-user-data",
					LogLevel:    "",
				},
			}

			Expect(olsconfigGenerated).To(Equal(olsConfigExpected))

			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should generate configmap with queryFilters", func() {
			crWithFilters := utils.WithQueryFilters(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), crWithFilters)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("query_filters", ContainElement(MatchAllKeys(Keys{
				"name":         Equal("testFilter"),
				"pattern":      Equal("testPattern"),
				"replace_with": Equal("testReplace"),
			})))))
		})

		It("should generate configmap with token quota limiters", func() {
			crWithFilters := utils.WithQuotaLimiters(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), crWithFilters)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("quota_handlers", HaveKeyWithValue("limiters", ContainElements(
				MatchAllKeys(Keys{
					"name":           Equal("my_user_limiter"),
					"type":           Equal("user_limiter"),
					"initial_quota":  BeNumerically("==", 10000),
					"quota_increase": BeNumerically("==", 100),
					"period":         Equal("1d"),
				}),
				MatchAllKeys(Keys{
					"name":           Equal("my_cluster_limiter"),
					"type":           Equal("cluster_limiter"),
					"initial_quota":  BeNumerically("==", 20000),
					"quota_increase": BeNumerically("==", 200),
					"period":         Equal("30d"),
				}),
			)))))
		})

		It("should generate configmap with Azure OpenAI provider", func() {
			azureOpenAI := utils.WithAzureOpenAIProvider(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), azureOpenAI)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name":        Equal("openai"),
				"type":        Equal("azure_openai"),
				"api_version": Equal("2021-09-01"),
				"azure_openai_config": MatchKeys(Options(IgnoreExtras), Keys{
					"url":              Equal(testURL),
					"credentials_path": Equal("/etc/apikeys/test-secret"),
					"deployment_name":  Equal("testDeployment"),
				}),
			}))))
		})

		It("should generate configmap with IBM watsonx provider", func() {
			watsonx := utils.WithWatsonxProvider(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), watsonx)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name":       Equal("watsonx"),
				"type":       Equal("watsonx"),
				"project_id": Equal("testProjectID"),
			}))))
		})

		It("should generate configmap with rhoai_vllm provider", func() {
			provider := utils.WithRHOAIProvider(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), provider)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name": Equal("rhoai_vllm"),
				"type": Equal("rhoai_vllm"),
			}))))
		})

		It("should generate configmap with rhelia_vllm provider", func() {
			provider := utils.WithRHELAIProvider(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), provider)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name": Equal("rhelai_vllm"),
				"type": Equal("rhelai_vllm"),
			}))))
		})

		It("should generate configmap with introspectionEnabled", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())

			var appSrvConfigFile utils.AppSrvConfigFile
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &appSrvConfigFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(appSrvConfigFile.MCPServers).NotTo(BeEmpty())
			Expect(appSrvConfigFile.MCPServers).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("openshift"),
				"Transport": Equal(utils.StreamableHTTP),
				"StreamableHTTP": PointTo(MatchFields(IgnoreExtras, Fields{
					"URL":            Equal(fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort)),
					"Timeout":        Equal(utils.OpenShiftMCPServerTimeout),
					"SSEReadTimeout": Equal(utils.OpenShiftMCPServerHTTPReadTimeout),
					"Headers":        Equal(map[string]string{utils.K8S_AUTH_HEADER: utils.KUBERNETES_PLACEHOLDER}),
				})),
			})))
		})

		It("should fail to generate configmap with additional MCP server if the headers are not configured correctly", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			utils.CreateMCPHeaderSecret(ctx, k8sClient, "garbage", false)
			cr.Spec.MCPServers = []olsv1alpha1.MCPServer{
				{
					Name: "testMCP",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header1": "value3",
						},
					},
				},
			}
			_, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP testMCP header secret value3 is not found"))

			cr.Spec.MCPServers = []olsv1alpha1.MCPServer{
				{
					Name: "testMCP",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header1": "garbage",
						},
					},
				},
			}
			_, err = GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("header garbage for MCP server testMCP is missing key 'header'"))
		})

		It("should generate configmap with additional MCP server if feature gate is enabled", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			utils.CreateMCPHeaderSecret(ctx, k8sClient, "value1", true)
			utils.CreateMCPHeaderSecret(ctx, k8sClient, "value2", true)
			cr.Spec.MCPServers = []olsv1alpha1.MCPServer{
				{
					Name: "testMCP",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header1": "value1",
						},
					},
				},
				{
					Name: "testMCP2",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP2.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header2": "value2",
						},
						EnableSSE: true,
					},
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			var appSrvConfigFile utils.AppSrvConfigFile
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &appSrvConfigFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(appSrvConfigFile.MCPServers).To(HaveLen(2))
			Expect(appSrvConfigFile.MCPServers[0].Name).To(Equal("testMCP"))
			Expect(appSrvConfigFile.MCPServers[0].Transport).To(Equal(utils.StreamableHTTP))
			Expect(appSrvConfigFile.MCPServers[0].StreamableHTTP).To(Equal(&utils.StreamableHTTPTransportConfig{
				URL:            "https://testMCP.com",
				Timeout:        10,
				SSEReadTimeout: 10,
				Headers: map[string]string{
					"header1": utils.MCPHeadersMountRoot + "/value1/" + utils.MCPSECRETDATAPATH,
				},
			}))
			Expect(appSrvConfigFile.MCPServers[0].SSE).To(BeNil())

			Expect(appSrvConfigFile.MCPServers[1].Name).To(Equal("testMCP2"))
			Expect(appSrvConfigFile.MCPServers[1].Transport).To(Equal(utils.SSE))
			Expect(appSrvConfigFile.MCPServers[1].SSE).To(Equal(&utils.StreamableHTTPTransportConfig{
				URL:            "https://testMCP2.com",
				Timeout:        10,
				SSEReadTimeout: 10,
				Headers: map[string]string{
					"header2": utils.MCPHeadersMountRoot + "/value2/" + utils.MCPSECRETDATAPATH,
				},
			}))
			Expect(appSrvConfigFile.MCPServers[1].StreamableHTTP).To(BeNil())
		})

		It("should not generate configmap with additional MCP server if feature gate is missing", func() {
			Expect(cr.Spec.FeatureGates).To(BeNil())
			utils.CreateMCPHeaderSecret(ctx, k8sClient, "value1", true)
			cr.Spec.MCPServers = []olsv1alpha1.MCPServer{
				{
					Name: "testMCP",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header1": "value1",
						},
					},
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			var appSrvConfigFile utils.AppSrvConfigFile
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &appSrvConfigFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(appSrvConfigFile.MCPServers).To(BeNil())
		})

		It("should generate configmap with additional MCP server along side the default MCP server", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServer{
				{
					Name: "testMCP",
					StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
						URL:            "https://testMCP.com",
						Timeout:        10,
						SSEReadTimeout: 10,
						Headers: map[string]string{
							"header1": "value1",
						},
					},
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			var appSrvConfigFile utils.AppSrvConfigFile
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &appSrvConfigFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(appSrvConfigFile.MCPServers).To(HaveLen(2))
			Expect(appSrvConfigFile.MCPServers).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("openshift"),
				"Transport": Equal(utils.StreamableHTTP),
				"StreamableHTTP": PointTo(MatchFields(IgnoreExtras, Fields{
					"URL":            Equal(fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort)),
					"Timeout":        Equal(utils.OpenShiftMCPServerTimeout),
					"SSEReadTimeout": Equal(utils.OpenShiftMCPServerHTTPReadTimeout),
				})),
			})))
			Expect(appSrvConfigFile.MCPServers).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("testMCP"),
				"Transport": Equal(utils.StreamableHTTP),
				"StreamableHTTP": PointTo(MatchFields(IgnoreExtras, Fields{
					"URL":            Equal("https://testMCP.com"),
					"Timeout":        BeNumerically("==", 10),
					"SSEReadTimeout": BeNumerically("==", 10),
					"Headers":        Equal(map[string]string{"header1": utils.MCPHeadersMountRoot + "/value1/" + utils.MCPSECRETDATAPATH}),
				})),
			})))
			Expect(appSrvConfigFile.MCPServers).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":      Equal("openshift"),
				"Transport": Equal(utils.StreamableHTTP),
				"StreamableHTTP": PointTo(MatchFields(IgnoreExtras, Fields{
					"URL":            Equal(fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort)),
					"Timeout":        Equal(utils.OpenShiftMCPServerTimeout),
					"SSEReadTimeout": Equal(utils.OpenShiftMCPServerHTTPReadTimeout),
					"Headers":        Equal(map[string]string{utils.K8S_AUTH_HEADER: utils.KUBERNETES_PLACEHOLDER}),
				})),
			})))
		})
		It("should place APIVersion in ProviderConfig for Azure OpenAI provider", func() {
			// Configure CR with Azure OpenAI provider including APIVersion
			cr = utils.WithAzureOpenAIProvider(cr)

			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())

			var appSrvConfigFile utils.AppSrvConfigFile
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &appSrvConfigFile)
			Expect(err).NotTo(HaveOccurred())

			// Verify that there is exactly one provider
			Expect(appSrvConfigFile.LLMProviders).To(HaveLen(1))
			provider := appSrvConfigFile.LLMProviders[0]

			// Verify APIVersion is set at the ProviderConfig level
			Expect(provider.APIVersion).To(Equal("2021-09-01"))

		})

		It("should generate the OLS deployment", func() {
			By("generate full deployment when telemetry pull secret exists")
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
			// application container
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OLSAppServerImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: utils.OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", utils.OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf(get10RequiredVolumeMounts()))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("1Gi")},
				Claims:   []corev1.ResourceClaim{},
			}))
			// dataverse exporter container
			Expect(dep.Spec.Template.Spec.Containers[1].Image).To(Equal(utils.DataverseExporterImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(Equal([]string{
				"--mode",
				"openshift",
				"--config",
				path.Join(utils.ExporterConfigMountPath, utils.ExporterConfigFilename),
				"--log-level",
				utils.LogLevelInfo,
				"--data-dir",
				utils.OLSUserDataMountPath,
			}))
			Expect(dep.Spec.Template.Spec.Containers[1].VolumeMounts).To(ConsistOf(get10RequiredVolumeMounts()))
			Expect(dep.Spec.Template.Spec.Containers[1].Resources).To(Equal(corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
				Claims:   []corev1.ResourceClaim{},
			}))
			Expect(len(dep.Spec.Template.Spec.Volumes)).To(Equal(10))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(utils.GenerateAppServerSelectorLabels()))

			By("generate deployment without data collector when telemetry pull secret does not exist")
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			// application container
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OLSAppServerImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: utils.OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", utils.OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf(get8RequiredVolumeMounts()))
			Expect(len(dep.Spec.Template.Spec.Volumes)).To(Equal(8))

			By("generate deployment without data collector when telemetry pull secret does not contain telemetry token")
			utils.CreateTelemetryPullSecret(ctx, k8sClient, false)
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)

			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			// application container
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OLSAppServerImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: utils.OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", utils.OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf(get8RequiredVolumeMounts()))
			Expect(len(dep.Spec.Template.Spec.Volumes)).To(Equal(8))
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should use configured log level for data collector container", func() {
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)

			By("using default INFO log level when not specified")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{}
			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			// data collector container should be the second container
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(ContainElement(utils.LogLevelInfo))

			By("using DEBUG log level when configured")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{
				LogLevel: utils.LogLevelDebug,
			}
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(Equal([]string{
				"--mode",
				"openshift",
				"--config",
				path.Join(utils.ExporterConfigMountPath, utils.ExporterConfigFilename),
				"--log-level",
				utils.LogLevelDebug,
				"--data-dir",
				utils.OLSUserDataMountPath,
			}))

			By("using WARNING log level when configured")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{
				LogLevel: utils.LogLevelWarning,
			}
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(ContainElement(utils.LogLevelWarning))

			By("using ERROR log level when configured")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{
				LogLevel: utils.LogLevelError,
			}
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(ContainElement(utils.LogLevelError))

			By("using CRITICAL log level when configured")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{
				LogLevel: "CRITICAL",
			}
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(ContainElement("CRITICAL"))

			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should generate the OLS service", func() {
			service, err := GenerateService(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(utils.OLSAppServerServiceName))
			Expect(service.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(service.Spec.Selector).To(Equal(utils.GenerateAppServerSelectorLabels()))
			Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
				{
					Name:       "https",
					Port:       utils.OLSAppServerServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("https"),
				},
			}))
		})

		It("should generate the network policy", func() {
			np, err := GenerateAppServerNetworkPolicy(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(np.Name).To(Equal(utils.OLSAppServerNetworkPolicyName))
			Expect(np.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress}))
			Expect(np.Spec.Ingress).To(HaveLen(3))
			// allow prometheus to scrape metrics
			Expect(np.Spec.Ingress).To(ContainElement(networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "app.kubernetes.io/name",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"prometheus"},
								},
								{
									Key:      "prometheus",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"k8s"},
								},
							},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": utils.ClientCACmNamespace,
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
					},
				},
			}))
			// allow the console to access the API
			Expect(np.Spec.Ingress).To(ContainElement(networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "console",
							},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": "openshift-console",
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
					},
				},
			}))
			// allow the ingress controller to access the API
			Expect(np.Spec.Ingress).To(ContainElement(networkingv1.NetworkPolicyIngressRule{
				// allow ingress controller to access the API
				From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"network.openshift.io/policy-group": "ingress",
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
					},
				},
			}))

		})

		It("should switch data collection on and off as CR defines in .spec.ols_config.user_data_collection", func() {
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			defer utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			By("Switching data collection off")
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated := utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.FeedbackDisabled).To(BeTrue())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.TranscriptsDisabled).To(BeTrue())

			deployment, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Volumes).To(Not(ContainElement(
				corev1.Volume{
					Name: "ols-user-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			)))

			By("Switching data collection on")
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    false,
				TranscriptsDisabled: false,
			}
			cm, err = GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.FeedbackDisabled).To(BeFalse())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.TranscriptsDisabled).To(BeFalse())

			deployment, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deployment.Spec.Template.Spec.Containers[1].Image).To(Equal(utils.DataverseExporterImageDefault))
			Expect(deployment.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(deployment.Spec.Template.Spec.Containers[1].Resources).ToNot(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers[1].Args).To(Equal([]string{
				"--mode",
				"openshift",
				"--config",
				path.Join(utils.ExporterConfigMountPath, utils.ExporterConfigFilename),
				"--log-level",
				utils.LogLevelInfo,
				"--data-dir",
				utils.OLSUserDataMountPath,
			}))
			Expect(deployment.Spec.Template.Spec.Containers[1].VolumeMounts).To(ConsistOf(get10RequiredVolumeMounts()))
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: "ols-user-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))
		})

		It("should use user provided TLS settings if user provided one", func() {
			const tlsSecretName = "test-tls-secret"
			cr.Spec.OLSConfig.TLSConfig = &olsv1alpha1.TLSConfig{
				KeyCertSecretRef: corev1.LocalObjectReference{
					Name: tlsSecretName,
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated := utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.TLSConfig.TLSCertificatePath).To(Equal(path.Join(utils.OLSAppCertsMountRoot, tlsSecretName, "tls.crt")))
			Expect(olsconfigGenerated.OLSConfig.TLSConfig.TLSKeyPath).To(Equal(path.Join(utils.OLSAppCertsMountRoot, tlsSecretName, "tls.key")))

			deployment, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "secret-" + tlsSecretName,
					MountPath: path.Join(utils.OLSAppCertsMountRoot, tlsSecretName),
					ReadOnly:  true,
				},
			))
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: "secret-" + tlsSecretName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  tlsSecretName,
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
			))
		})

		It("should generate RAG volume and initContainers", func() {
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.19",
					IndexID:   "ocp-product-docs-4_19",
					Image:     "rag-ocp-product-docs:4.19",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			deployment, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: utils.RAGVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}))

			Expect(deployment.Spec.Template.Spec.InitContainers).To(ConsistOf(
				corev1.Container{
					Name:    "rag-0",
					Image:   "rag-ocp-product-docs:4.19",
					Command: []string{"sh", "-c", fmt.Sprintf("mkdir -p %s/rag-0 && cp -a /rag/vector_db/ocp_product_docs/4.19/. %s/rag-0", utils.RAGVolumeMountPath, utils.RAGVolumeMountPath)},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      utils.RAGVolumeName,
							MountPath: utils.RAGVolumeMountPath,
						},
					},
					ImagePullPolicy: corev1.PullAlways,
				},
				corev1.Container{
					Name:    "rag-1",
					Image:   "rag-ansible-docs:2.18",
					Command: []string{"sh", "-c", fmt.Sprintf("mkdir -p %s/rag-1 && cp -a /rag/vector_db/ansible_docs/2.18/. %s/rag-1", utils.RAGVolumeMountPath, utils.RAGVolumeMountPath)},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      utils.RAGVolumeName,
							MountPath: utils.RAGVolumeMountPath,
						},
					},
					ImagePullPolicy: corev1.PullAlways,
				},
			))
		})

		It("should fill app config with multiple RAG indexes and remove them when no additional RAG is defined", func() {
			By("additional RAG indexes are added")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.19",
					IndexID:   "ocp-product-docs-4_19",
					Image:     "rag-ocp-product-docs:4.19",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated := utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())

			major, minor, err := utils.GetOpenshiftVersion(k8sClient, ctx)
			Expect(err).NotTo(HaveOccurred())
			// OCP document is there unless byokRAGOnly is true
			ocpIndex := utils.ReferenceIndex{
				ProductDocsIndexId:   "ocp-product-docs-" + major + "_" + minor,
				ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/" + major + "." + minor,
				ProductDocsOrigin:    "Red Hat OpenShift 123.456 documentation",
			}

			// OLS-1823: prioritize BYOK content over OCP docs
			Expect(olsconfigGenerated.OLSConfig.ReferenceContent.Indexes).To(Equal([]utils.ReferenceIndex{
				{
					ProductDocsIndexId:   "ocp-product-docs-4_19",
					ProductDocsIndexPath: utils.RAGVolumeMountPath + "/rag-0",
					ProductDocsOrigin:    "rag-ocp-product-docs:4.19",
				},
				{
					ProductDocsIndexId:   "ansible-docs-2_18",
					ProductDocsIndexPath: utils.RAGVolumeMountPath + "/rag-1",
					ProductDocsOrigin:    "rag-ansible-docs:2.18",
				},
				ocpIndex,
			}))

			By("additional RAG indexes are removed")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{}
			cm, err = GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated = utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.ReferenceContent.Indexes).To(ConsistOf(ocpIndex))

		})

		// This test covers ByokRAGOnly == true. ByokRAGOnly == false is covered by the previous test.
		It("should not include the OCP docs RAG when byokRAGOnly is true", func() {
			cr.Spec.OLSConfig.ByokRAGOnly = true
			By("RAG index is added")
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated := utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())

			// Testing for equality means the only RAG source is the one specified via the BYOK
			// mechanism above. The OCP RAG database is not included.
			Expect(olsconfigGenerated.OLSConfig.ReferenceContent.Indexes).To(Equal([]utils.ReferenceIndex{
				{
					ProductDocsIndexId:   "ansible-docs-2_18",
					ProductDocsIndexPath: utils.RAGVolumeMountPath + "/rag-0",
					ProductDocsOrigin:    "rag-ansible-docs:2.18",
				},
			}))
		})

		It("should generate deployment with MCP server sidecar when introspectionEnabled is true", func() {
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			defer utils.DeleteTelemetryPullSecret(ctx, k8sClient)

			By("Enabling introspection")
			cr.Spec.OLSConfig.IntrospectionEnabled = true

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))

			// Should have 3 containers: main app, telemetry, and MCP server
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(3))

			// Verify OpenShift MCP server container (should be the third container)
			openshiftMCPServerContainer := dep.Spec.Template.Spec.Containers[2]
			Expect(openshiftMCPServerContainer.Name).To(Equal(utils.OpenShiftMCPServerContainerName))
			Expect(openshiftMCPServerContainer.Image).To(Equal(utils.OpenShiftMCPServerImageDefault))
			Expect(openshiftMCPServerContainer.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
			Expect(openshiftMCPServerContainer.Command).To(Equal([]string{"/openshift-mcp-server", "--read-only", "--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerPort)}))
			Expect(openshiftMCPServerContainer.SecurityContext).To(Equal(&corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
			}))
			Expect(openshiftMCPServerContainer.Resources).To(Equal(corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
				Claims:   []corev1.ResourceClaim{},
			}))

			// Verify MCP server has the same volume mounts as other containers
			Expect(openshiftMCPServerContainer.VolumeMounts).To(ConsistOf(get10RequiredVolumeMounts()))

			By("Disabling introspection")
			cr.Spec.OLSConfig.IntrospectionEnabled = false

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())

			// Should have only 2 containers: main app and dataverse exporter (no MCP server)
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
		})

		It("should deploy MCP container independently of data collection settings", func() {
			By("Test case 1: introspection enabled, data collection enabled - should have both MCP and dataverse exporter containers")
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    false,
				TranscriptsDisabled: false,
			}

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(3))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[2].Name).To(Equal(utils.OpenShiftMCPServerContainerName))

			By("Test case 2: introspection enabled, data collection disabled - should have only MCP container (no dataverse exporter)")
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.OpenShiftMCPServerContainerName))

			By("Test case 3: introspection disabled, data collection enabled - should have only dataverse exporter container (no MCP)")
			cr.Spec.OLSConfig.IntrospectionEnabled = false
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    false,
				TranscriptsDisabled: false,
			}

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))

			By("Test case 4: introspection disabled, data collection disabled - should have only main container")
			cr.Spec.OLSConfig.IntrospectionEnabled = false
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))

			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should deploy MCP container when introspection is enabled regardless of telemetry settings", func() {
			By("Test case: introspection enabled with no telemetry pull secret - MCP should still be deployed")
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.OpenShiftMCPServerContainerName))

			// Verify MCP container configuration
			mcpContainer := dep.Spec.Template.Spec.Containers[1]
			Expect(mcpContainer.Image).To(Equal(utils.OpenShiftMCPServerImageDefault))
			Expect(mcpContainer.Command).To(Equal([]string{"/openshift-mcp-server", "--read-only", "--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerPort)}))
			Expect(mcpContainer.Resources).To(Equal(corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
				Claims:   []corev1.ResourceClaim{},
			}))
		})

	})

	Context("empty custom resource", func() {
		BeforeEach(func() {
			cr = utils.GetEmptyOLSConfigCR()
			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
			configmap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Configmap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.DefaultOpenShiftCerts,
				},
			})
			configMapCreationErr := testReconcilerInstance.Create(ctx, configmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate a service account", func() {
			sa, err := GenerateServiceAccount(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(sa.Name).To(Equal(utils.OLSAppServerServiceAccountName))
			Expect(sa.Namespace).To(Equal(utils.OLSNamespaceDefault))
		})

		It("should generate the olsconfig config map", func() {
			// todo: this test is not complete
			// generateOLSConfigMap should return an error if the CR is missing required fields
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			major, minor, err := utils.GetOpenshiftVersion(k8sClient, ctx)
			Expect(err).NotTo(HaveOccurred())
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			expectedConfigStr := `llm_providers: []
ols_config:
  conversation_cache:
    postgres:
      ca_cert_path: /etc/certs/lightspeed-postgres-certs/cm-olspostgresca/service-ca.crt
      dbname: postgres
      host: lightspeed-postgres-server.openshift-lightspeed.svc
      password_path: /etc/credentials/lightspeed-postgres-secret/password
      port: 5432
      ssl_mode: require
      user: postgres
    type: postgres
  extra_ca:
    - /etc/certs/ols-additional-ca/service-ca.crt
  certificate_directory: /etc/certs/cert-bundle		
  logging_config:
    app_log_level: ""
    lib_log_level: ""
    uvicorn_log_level: ""
  reference_content:
    embeddings_model_path: /app-root/embeddings_model
    indexes:
    - product_docs_index_id: ocp-product-docs-` + major + `_` + minor + `
      product_docs_index_path: /app-root/vector_db/ocp_product_docs/` + major + `.` + minor + `
      product_docs_origin: Red Hat OpenShift 123.456 documentation
  tls_config:
    tls_certificate_path: /etc/certs/lightspeed-tls/tls.crt
    tls_key_path: /etc/certs/lightspeed-tls/tls.key
  user_data_collection:
    feedback_disabled: false
    feedback_storage: /app-root/ols-user-data/feedback
    transcripts_disabled: false
    transcripts_storage: /app-root/ols-user-data/transcripts
user_data_collector_config:
  data_storage: /app-root/ols-user-data

`
			// unmarshal to ensure the key order
			var actualConfig map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &actualConfig)
			Expect(err).NotTo(HaveOccurred())

			var expectedConfig map[string]interface{}
			err = yaml.Unmarshal([]byte(expectedConfigStr), &expectedConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(actualConfig).To(Equal(expectedConfig))
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should generate the olsconfig config map without user_data_collector_config", func() {
			// pull-secret without telemetry token should disable data collection
			// and user_data_collector_config should not be present in the config
			utils.CreateTelemetryPullSecret(ctx, k8sClient, false)
			major, minor, err := utils.GetOpenshiftVersion(k8sClient, ctx)
			Expect(err).NotTo(HaveOccurred())
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			expectedConfigStr := `llm_providers: []
ols_config:
  conversation_cache:
    postgres:
      ca_cert_path: /etc/certs/lightspeed-postgres-certs/cm-olspostgresca/service-ca.crt
      dbname: postgres
      host: lightspeed-postgres-server.openshift-lightspeed.svc
      password_path: /etc/credentials/lightspeed-postgres-secret/password
      port: 5432
      ssl_mode: require
      user: postgres
    type: postgres
  extra_ca:
    - /etc/certs/ols-additional-ca/service-ca.crt
  certificate_directory: /etc/certs/cert-bundle		
  logging_config:
    app_log_level: ""
    lib_log_level: ""
    uvicorn_log_level: ""
  reference_content:
    embeddings_model_path: /app-root/embeddings_model
    indexes:
    - product_docs_index_id: ocp-product-docs-` + major + `_` + minor + `
      product_docs_index_path: /app-root/vector_db/ocp_product_docs/` + major + `.` + minor + `
      product_docs_origin: Red Hat OpenShift 123.456 documentation
  tls_config:
    tls_certificate_path: /etc/certs/lightspeed-tls/tls.crt
    tls_key_path: /etc/certs/lightspeed-tls/tls.key
  user_data_collection:
    feedback_disabled: true
    feedback_storage: /app-root/ols-user-data/feedback
    transcripts_disabled: true
    transcripts_storage: /app-root/ols-user-data/transcripts
user_data_collector_config: {}

`
			// unmarshal to ensure the key order
			var actualConfig map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &actualConfig)
			Expect(err).NotTo(HaveOccurred())

			var expectedConfig map[string]interface{}
			err = yaml.Unmarshal([]byte(expectedConfigStr), &expectedConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(actualConfig).To(Equal(expectedConfig))
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should generate the OLS service", func() {
			service, err := GenerateService(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(utils.OLSAppServerServiceName))
			Expect(service.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(service.Spec.Selector).To(Equal(utils.GenerateAppServerSelectorLabels()))
			Expect(service.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OLSCertsSecretName))
			Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
				{
					Name:       "https",
					Port:       utils.OLSAppServerServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("https"),
				},
			}))
		})

		It("should generate the OLS deployment", func() {
			// todo: update this test after updating the test for generateOLSConfigMap
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			defer utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OLSAppServerImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: utils.OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", utils.OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf(
				append(get7RequiredVolumeMounts(),
					corev1.VolumeMount{
						Name:      "ols-user-data",
						ReadOnly:  false,
						MountPath: "/app-root/ols-user-data",
					},
					corev1.VolumeMount{
						Name:      utils.ExporterConfigVolumeName,
						ReadOnly:  true,
						MountPath: utils.ExporterConfigMountPath,
					}),
			))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf(
				append(get7RequiredVolumes(),
					corev1.Volume{
						Name: "ols-user-data",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					corev1.Volume{
						Name: utils.ExporterConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: utils.ExporterConfigCmName},
								DefaultMode:          &defaultVolumeMode,
							},
						},
					}),
			))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(utils.GenerateAppServerSelectorLabels()))
			Expect(dep.Spec.Template.Spec.Containers[0].LivenessProbe).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Port).To(Equal(intstr.FromString("https")))
			Expect(dep.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Path).To(Equal("/liveness"))
			Expect(dep.Spec.Template.Spec.Containers[0].ReadinessProbe).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Port).To(Equal(intstr.FromString("https")))
			Expect(dep.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Path).To(Equal("/readiness"))
			Expect(dep.Spec.Template.Spec.Tolerations).To(BeNil())
			Expect(dep.Spec.Template.Spec.NodeSelector).To(BeNil())
		})

		It("should generate the OLS service monitor", func() {
			serviceMonitor, err := GenerateServiceMonitor(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceMonitor.Name).To(Equal(utils.AppServerServiceMonitorName))
			Expect(serviceMonitor.Namespace).To(Equal(utils.OLSNamespaceDefault))
			valFalse := false
			serverName := fmt.Sprintf("%s.%s.svc", utils.OLSAppServerServiceName, utils.OLSNamespaceDefault)
			Expect(serviceMonitor.Spec.Endpoints).To(ConsistOf(
				monv1.Endpoint{
					Port:     "https",
					Path:     utils.AppServerMetricsPath,
					Interval: "30s",
					Scheme:   "https",
					TLSConfig: &monv1.TLSConfig{
						SafeTLSConfig: monv1.SafeTLSConfig{
							InsecureSkipVerify: &valFalse,
							ServerName:         &serverName,
						},
						CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
						CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
						KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
					},
					Authorization: &monv1.SafeAuthorization{
						Type: "Bearer",
						Credentials: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: utils.MetricsReaderServiceAccountTokenSecretName,
							},
							Key: "token",
						},
					},
				},
			))
			Expect(serviceMonitor.Spec.Selector.MatchLabels).To(Equal(utils.GenerateAppServerSelectorLabels()))
			Expect(serviceMonitor.ObjectMeta.Labels).To(HaveKeyWithValue("openshift.io/user-monitoring", "false"))
		})

		It("should generate the metrics reader secret", func() {
			secret, err := GenerateMetricsReaderSecret(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal(utils.MetricsReaderServiceAccountTokenSecretName))
			Expect(secret.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(secret.Type).To(Equal(corev1.SecretTypeServiceAccountToken))
		})

		It("should generate the OLS prometheus rules", func() {
			prometheusRule, err := GeneratePrometheusRule(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(prometheusRule.Name).To(Equal(utils.AppServerPrometheusRuleName))
			Expect(prometheusRule.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(len(prometheusRule.Spec.Groups[0].Rules)).To(Equal(4))
		})

		It("should generate the SAR cluster role", func() {
			clusterRole, err := GenerateSARClusterRole(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterRole.Name).To(Equal(utils.OLSAppServerSARRoleName))
			Expect(clusterRole.Rules).To(ConsistOf(
				rbacv1.PolicyRule{
					APIGroups: []string{"authorization.k8s.io"},
					Resources: []string{"subjectaccessreviews"},
					Verbs:     []string{"create"},
				},
				rbacv1.PolicyRule{
					APIGroups: []string{"authentication.k8s.io"},
					Resources: []string{"tokenreviews"},
					Verbs:     []string{"create"},
				},
				rbacv1.PolicyRule{
					APIGroups: []string{"config.openshift.io"},
					Resources: []string{"clusterversions"},
					Verbs:     []string{"get", "list"},
				},
				rbacv1.PolicyRule{
					APIGroups:     []string{""},
					Resources:     []string{"secrets"},
					ResourceNames: []string{"pull-secret"},
					Verbs:         []string{"get"},
				},
			))
		})
	})

	Context("Additional CA", func() {

		const caConfigMapName = "test-ca-configmap"
		const certFilename = "additional-ca.crt"
		var additionalCACm *corev1.ConfigMap

		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			err := testReconcilerInstance.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			By("create the additional CA configmap")
			additionalCACm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					certFilename: utils.TestCACert,
				},
			}
			err = testReconcilerInstance.Create(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
			configmap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Configmap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.DefaultOpenShiftCerts,
				},
			})
			configMapCreationErr := testReconcilerInstance.Create(ctx, configmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the provider secret")
			err := testReconcilerInstance.Delete(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			By("Delete the additional CA configmap")
			err = testReconcilerInstance.Delete(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should update OLS config and mount volumes for additional CA", func() {
			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Volumes).NotTo(ContainElement(
				corev1.Volume{
					Name: utils.AdditionalCAVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: caConfigMapName,
							},
							DefaultMode: &defaultVolumeMode,
						},
					},
				}))

			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: caConfigMapName,
			}

			olsCm, err := GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("extra_ca:\n  - /etc/certs/ols-additional-ca/service-ca.crt\n  - /etc/certs/ols-user-ca/additional-ca.crt"))
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("certificate_directory: /etc/certs/cert-bundle"))

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Volumes).To(ContainElements(
				corev1.Volume{
					Name: utils.AdditionalCAVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: caConfigMapName,
							},
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				corev1.Volume{
					Name: utils.CertBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))

		})

		It("should return error if the CA text is malformed", func() {
			additionalCACm.Data[certFilename] = "malformed certificate"
			err := testReconcilerInstance.Update(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())

			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: caConfigMapName,
			}
			_, err = GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to validate additional CA certificate"))
		})

	})

	Context("Proxy settings", func() {
		const caConfigMapName = "test-ca-configmap"
		var proxyCACm *corev1.ConfigMap

		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			By("create the provider secret")
			secret, _ = utils.GenerateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			err := testReconcilerInstance.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			By("create the additional CA configmap")
			proxyCACm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					utils.ProxyCACertFileName: utils.TestCACert,
				},
			}
			err = testReconcilerInstance.Create(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())

			By("create the OpenShift certificates config map")
			configmap, _ = utils.GenerateRandomConfigMap()
			configmap.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Configmap",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       utils.DefaultOpenShiftCerts,
				},
			})
			configMapCreationErr := testReconcilerInstance.Create(ctx, configmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the provider secret")
			err := testReconcilerInstance.Delete(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			By("Delete the additional CA configmap")
			err = testReconcilerInstance.Delete(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should update OLS config and mount volumes for proxy settings", func() {
			olsCm, err := GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[utils.OLSConfigFilename]).NotTo(ContainSubstring("proxy_config:"))

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Volumes).NotTo(ContainElement(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(utils.ProxyCACertVolumeName),
				}),
			))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).NotTo(ContainElement(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(utils.ProxyCACertVolumeName),
				}),
			))

			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: "https://proxy.example.com:8080",
				ProxyCACertificateRef: &corev1.LocalObjectReference{
					Name: caConfigMapName,
				},
			}

			olsCm, err = GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("proxy_config:\n    proxy_ca_cert_path: /etc/certs/proxy-ca/proxy-ca.crt\n    proxy_url: https://proxy.example.com:8080\n"))

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: utils.ProxyCACertVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: caConfigMapName,
							},
							DefaultMode: &defaultVolumeMode,
						},
					},
				}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      utils.ProxyCACertVolumeName,
					MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName),
					ReadOnly:  true,
				},
			))
		})

		It("should return error if the CA text is malformed", func() {
			proxyCACm.Data[utils.ProxyCACertFileName] = "malformed certificate"
			err := testReconcilerInstance.Update(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())

			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: "https://proxy.example.com:8080",
				ProxyCACertificateRef: &corev1.LocalObjectReference{
					Name: caConfigMapName,
				},
			}
			_, err = GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to validate proxy CA certificate"))
		})
	})
})

func get7RequiredVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "secret-lightspeed-tls",
			MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.OLSCertsSecretName),
			ReadOnly:  true,
		},
		{
			Name:      "cm-olsconfig",
			MountPath: "/etc/ols",
			ReadOnly:  true,
		},
		{
			Name:      "secret-lightspeed-postgres-secret",
			ReadOnly:  true,
			MountPath: "/etc/credentials/lightspeed-postgres-secret",
		},
		{
			Name:      utils.PostgresCAVolume,
			ReadOnly:  true,
			MountPath: "/etc/certs/lightspeed-postgres-certs/cm-olspostgresca",
		},
		{
			Name:      utils.TmpVolumeName,
			MountPath: utils.TmpVolumeMountPath,
		},
		{
			Name:      "openshift-ca",
			ReadOnly:  true,
			MountPath: "/etc/certs/ols-additional-ca",
		},
		{
			Name:      "cert-bundle",
			ReadOnly:  false,
			MountPath: "/etc/certs/cert-bundle",
		},
	}
}

func get8RequiredVolumeMounts() []corev1.VolumeMount {
	return append(get7RequiredVolumeMounts(),
		corev1.VolumeMount{
			Name:      "secret-test-secret",
			MountPath: path.Join(utils.APIKeyMountRoot, "test-secret"),
			ReadOnly:  true,
		})
}

func get9RequiredVolumeMounts() []corev1.VolumeMount {
	return append(get8RequiredVolumeMounts(),
		corev1.VolumeMount{
			Name:      "ols-user-data",
			ReadOnly:  false,
			MountPath: "/app-root/ols-user-data",
		})
}

func get10RequiredVolumeMounts() []corev1.VolumeMount {
	return append(get9RequiredVolumeMounts(),
		corev1.VolumeMount{
			Name:      utils.ExporterConfigVolumeName,
			ReadOnly:  true,
			MountPath: utils.ExporterConfigMountPath,
		})
}

func get7RequiredVolumes() []corev1.Volume {

	return []corev1.Volume{
		{
			Name: "secret-lightspeed-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.OLSCertsSecretName,
					DefaultMode: &defaultVolumeMode,
				},
			},
		},
		{
			Name: "cm-olsconfig",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: utils.OLSConfigCmName},
					DefaultMode:          &defaultVolumeMode,
				},
			},
		},
		{
			Name: "secret-lightspeed-postgres-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.PostgresSecretName,
					DefaultMode: &defaultVolumeMode,
				},
			},
		},
		{
			Name: utils.PostgresCAVolume,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: utils.OLSCAConfigMap},
					DefaultMode:          &defaultVolumeMode,
				},
			},
		},
		{
			Name: utils.TmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "openshift-ca",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"},
					DefaultMode:          &defaultVolumeMode,
				},
			},
		},
		{
			Name: "cert-bundle",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
}

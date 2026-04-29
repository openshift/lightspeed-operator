package appserver

import (
	"context"
	"fmt"
	"path"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	networkingv1 "k8s.io/api/networking/v1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
					MaxIterations:   5,
					Logging: utils.LoggingConfig{
						AppLogLevel:     string(olsv1alpha1.LogLevelInfo),
						LibLogLevel:     string(olsv1alpha1.LogLevelInfo),
						UvicornLogLevel: string(olsv1alpha1.LogLevelInfo),
					},
					ConversationCache: utils.ConversationCacheConfig{
						Type: utils.OLSDefaultCacheType,
						Postgres: utils.PostgresCacheConfig{
							Host: strings.Join([]string{utils.PostgresServiceName, utils.OLSNamespaceDefault, "svc"}, "."),
							Port: utils.PostgresServicePort, User: utils.PostgresDefaultUser,
							DbName:       utils.PostgresDefaultDbName,
							PasswordPath: path.Join(utils.CredentialsMountRoot, utils.PostgresSecretName, utils.OLSComponentPasswordFileName),
							SSLMode:      utils.PostgresDefaultSSLMode,
							CACertPath:   path.Join(utils.OLSAppCertsMountRoot, "postgres-ca", "service-ca.crt"),
						},
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
					ToolsApproval: &utils.ToolsApprovalConfig{
						ApprovalType:    "tool_annotations",
						ApprovalTimeout: utils.ToolsApprovalDefaultTimeout,
					},
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
									ToolBudgetRatio:      0.5,
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

		It("should apply default tool_budget_ratio when parameters are not specified", func() {
			crNoParams := &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: utils.OLSConfigName,
				},
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{
							{
								Name: "testProvider",
								Type: "bam",
								URL:  "https://testURL",
								Models: []olsv1alpha1.ModelSpec{
									{
										Name:              "testModel",
										URL:               "https://testURL",
										ContextWindowSize: 32768,
									},
								},
								CredentialsSecretRef: corev1.LocalObjectReference{
									Name: "test-secret",
								},
							},
						},
					},
					OLSConfig: olsv1alpha1.OLSSpec{
						DefaultModel:    "testModel",
						DefaultProvider: "testProvider",
					},
				},
			}

			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), crNoParams)
			Expect(err).NotTo(HaveOccurred())

			olsconfigGenerated := utils.AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())

			Expect(olsconfigGenerated.LLMProviders).To(HaveLen(1))
			Expect(olsconfigGenerated.LLMProviders[0].Models).To(HaveLen(1))
			Expect(olsconfigGenerated.LLMProviders[0].Models[0].Parameters.ToolBudgetRatio).To(Equal(0.5))
			Expect(olsconfigGenerated.LLMProviders[0].Models[0].Parameters.MaxTokensForResponse).To(Equal(0))
		})

		It("should generate configmap with queryFilters", func() {
			cr.Spec.OLSConfig.QueryFilters = []olsv1alpha1.QueryFiltersSpec{
				{Name: "testFilter", Pattern: "testPattern", ReplaceWith: "testReplace"},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
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

		It("should generate configmap with tools approval configuration", func() {
			By("with default values when empty config is specified")
			cr.Spec.OLSConfig.ToolsApprovalConfig = &olsv1alpha1.ToolsApprovalConfig{}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("tools_approval", MatchAllKeys(Keys{
				"approval_type":    Equal("tool_annotations"),
				"approval_timeout": BeNumerically("==", 600),
			}))))

			By("with custom values when specified")
			cr.Spec.OLSConfig.ToolsApprovalConfig = &olsv1alpha1.ToolsApprovalConfig{
				ApprovalType:    olsv1alpha1.ApprovalTypeAlways,
				ApprovalTimeout: 300,
			}
			cm, err = GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("tools_approval", MatchAllKeys(Keys{
				"approval_type":    Equal("always"),
				"approval_timeout": BeNumerically("==", 300),
			}))))

			By("with tool_annotations approval type")
			cr.Spec.OLSConfig.ToolsApprovalConfig = &olsv1alpha1.ToolsApprovalConfig{
				ApprovalType:    olsv1alpha1.ApprovalTypeToolAnnotations,
				ApprovalTimeout: 120,
			}
			cm, err = GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("tools_approval", MatchAllKeys(Keys{
				"approval_type":    Equal("tool_annotations"),
				"approval_timeout": BeNumerically("==", 120),
			}))))

			By("with default values when config is nil")
			cr.Spec.OLSConfig.ToolsApprovalConfig = nil
			cm, err = GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("tools_approval", MatchAllKeys(Keys{
				"approval_type":    Equal("tool_annotations"),
				"approval_timeout": BeNumerically("==", 600),
			}))))
		})

		It("should generate configmap with token quota limiters", func() {
			cr.Spec.OLSConfig.QuotaHandlersConfig = &olsv1alpha1.QuotaHandlersConfig{
				LimitersConfig: []olsv1alpha1.LimiterConfig{
					{Name: "my_user_limiter", Type: "user_limiter", InitialQuota: 10000, QuotaIncrease: 100, Period: "1d"},
					{Name: "my_cluster_limiter", Type: "cluster_limiter", InitialQuota: 20000, QuotaIncrease: 200, Period: "30d"},
				},
			}
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
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
			setFirstLLMProviderNameAndType(cr, "watsonx", "watsonx")
			cr.Spec.LLMConfig.Providers[0].WatsonProjectID = "testProjectID"
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
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
			setFirstLLMProviderNameAndType(cr, "rhoai_vllm", "rhoai_vllm")
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
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
			setFirstLLMProviderNameAndType(cr, "rhelai_vllm", "rhelai_vllm")
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name": Equal("rhelai_vllm"),
				"type": Equal("rhelai_vllm"),
			}))))
		})

		It("should generate configmap with googleVertex provider", func() {
			cr := utils.WithGoogleVertexProvider(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name":             Equal("google_vertex"),
				"type":             Equal("google_vertex"),
				"credentials_path": Equal("/etc/apikeys/test-secret/apitoken"),
				"google_vertex_config": MatchKeys(Options(IgnoreExtras), Keys{
					"project":  Equal("testProjectID"),
					"location": Equal("testLocation"),
				}),
			}))))
		})

		It("should return error when googleVertexConfig is not specified for google_vertex provider", func() {
			cr := utils.WithGoogleVertexProvider(cr)
			cr.Spec.LLMConfig.Providers[0].GoogleVertexConfig = nil
			_, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("googleVertexConfig is required for google_vertex provider"))
		})

		It("should generate configmap with googleVertexAnthropic provider", func() {
			cr := utils.WithGoogleVertexAnthropicProvider(cr)
			cm, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[utils.OLSConfigFilename]), &olsConfigMap)

			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name":             Equal("google_vertex_anthropic"),
				"type":             Equal("google_vertex_anthropic"),
				"credentials_path": Equal("/etc/apikeys/test-secret/apitoken"),
				"google_vertex_anthropic_config": MatchKeys(Options(IgnoreExtras), Keys{
					"project":  Equal("testProjectID"),
					"location": Equal("testLocation"),
				}),
			}))))
		})

		It("should return error when googleVertexAnthropicConfig is not specified for google_vertex_anthropic provider", func() {
			cr := utils.WithGoogleVertexAnthropicProvider(cr)
			cr.Spec.LLMConfig.Providers[0].GoogleVertexAnthropicConfig = nil
			_, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("googleVertexAnthropicConfig is required for google_vertex_anthropic provider"))
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
				"Name":    Equal("openshift"),
				"URL":     Equal(fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort)),
				"Timeout": Equal(utils.OpenShiftMCPServerTimeout),
				"Headers": Equal(map[string]string{
					utils.K8S_AUTH_HEADER: utils.KUBERNETES_PLACEHOLDER,
				}),
			})))
		})

		It("should skip MCP server with missing header secret during config generation", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			// Note: We don't create the secret - config generation doesn't validate secrets
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name:    "testMCP",
					URL:     "https://testMCP.com",
					Timeout: 10,
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "header1",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{Name: "value3"},
							},
						},
					},
				},
			}
			// Config generation should succeed - secret validation happens during deployment
			_, err := GenerateOLSConfigMap(testReconcilerInstance, context.TODO(), cr)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should generate configmap with additional MCP servers if feature gate is enabled", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name:    "testMCP",
					URL:     "https://testMCP.com",
					Timeout: 10,
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "header1",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{Name: "value1"},
							},
						},
					},
				},
				{
					Name:    "testMCP2",
					URL:     "https://testMCP2.com",
					Timeout: 10,
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "header2",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{Name: "value2"},
							},
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

			Expect(appSrvConfigFile.MCPServers[0].Name).To(Equal("testMCP"))
			Expect(appSrvConfigFile.MCPServers[0].URL).To(Equal("https://testMCP.com"))
			Expect(appSrvConfigFile.MCPServers[0].Timeout).To(Equal(10))
			Expect(appSrvConfigFile.MCPServers[0].Headers).To(Equal(map[string]string{
				"header1": utils.MCPHeadersMountRoot + "/value1/" + utils.MCPSECRETDATAPATH,
			}))

			Expect(appSrvConfigFile.MCPServers[1].Name).To(Equal("testMCP2"))
			Expect(appSrvConfigFile.MCPServers[1].URL).To(Equal("https://testMCP2.com"))
			Expect(appSrvConfigFile.MCPServers[1].Timeout).To(Equal(10))
			Expect(appSrvConfigFile.MCPServers[1].Headers).To(Equal(map[string]string{
				"header2": utils.MCPHeadersMountRoot + "/value2/" + utils.MCPSECRETDATAPATH,
			}))
		})

		It("should not generate configmap with additional MCP server if feature gate is missing", func() {
			Expect(cr.Spec.FeatureGates).To(BeNil())
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name:    "testMCP",
					URL:     "https://testMCP.com",
					Timeout: 10,
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "header1",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{Name: "value1"},
							},
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
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name:    "testMCP",
					URL:     "https://testMCP.com",
					Timeout: 10,
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "header1",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{Name: "value1"},
							},
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
				"Name":    Equal("openshift"),
				"URL":     Equal(fmt.Sprintf(utils.OpenShiftMCPServerURL, utils.OpenShiftMCPServerPort)),
				"Timeout": Equal(utils.OpenShiftMCPServerTimeout),
			})))

			Expect(appSrvConfigFile.MCPServers).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Name":    Equal("testMCP"),
				"URL":     Equal("https://testMCP.com"),
				"Timeout": Equal(10),
				"Headers": Equal(map[string]string{
					"header1": utils.MCPHeadersMountRoot + "/value1/" + utils.MCPSECRETDATAPATH,
				}),
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

		It("should generate exporter configmap with service_id 'ols' by default", func() {
			cm, err := generateExporterConfigMap(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.ExporterConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(cm.Data[utils.ExporterConfigFilename]).To(ContainSubstring(`service_id: "` + utils.ServiceIDOLS + `"`))
		})

		It("should generate exporter configmap with service_id 'rhos-lightspeed' when label is present", func() {
			if cr.Labels == nil {
				cr.Labels = make(map[string]string)
			}

			cr.Labels[utils.RHOSOLightspeedOwnerIDLabel] = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxx"

			cm, err := generateExporterConfigMap(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.ExporterConfigCmName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(cm.Data[utils.ExporterConfigFilename]).To(ContainSubstring(`service_id: "` + utils.ServiceIDRHOSO + `"`))
		})

	})

	Context("empty custom resource", func() {
		BeforeEach(func() {
			cr = &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			}
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
      ca_cert_path: /etc/certs/postgres-ca/service-ca.crt
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
  tools_approval:
    approval_timeout: 600
    approval_type: tool_annotations
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
      ca_cert_path: /etc/certs/postgres-ca/service-ca.crt
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
  tools_approval:
    approval_timeout: 600
    approval_type: tool_annotations
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

		It("should generate the OLS service monitor", func() {
			serviceMonitor, err := GenerateServiceMonitor(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceMonitor.Name).To(Equal(utils.AppServerServiceMonitorName))
			Expect(serviceMonitor.Namespace).To(Equal(utils.OLSNamespaceDefault))
			valFalse := false
			serverName := fmt.Sprintf("%s.%s.svc", utils.OLSAppServerServiceName, utils.OLSNamespaceDefault)
			var schemeHTTPS monv1.Scheme = "https"
			Expect(serviceMonitor.Spec.Endpoints).To(ConsistOf(
				monv1.Endpoint{
					Port:     "https",
					Path:     utils.AppServerMetricsPath,
					Interval: "30s",
					Scheme:   &schemeHTTPS,
					HTTPConfigWithProxyAndTLSFiles: monv1.HTTPConfigWithProxyAndTLSFiles{
						HTTPConfigWithTLSFiles: monv1.HTTPConfigWithTLSFiles{
							TLSConfig: &monv1.TLSConfig{
								TLSFilesConfig: monv1.TLSFilesConfig{
									CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
									CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
									KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
								},
								SafeTLSConfig: monv1.SafeTLSConfig{
									InsecureSkipVerify: &valFalse,
									ServerName:         &serverName,
								},
							},
							HTTPConfigWithoutTLS: monv1.HTTPConfigWithoutTLS{
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
			err := testReconcilerInstance.Create(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the additional CA configmap")
			err := testReconcilerInstance.Delete(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())
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
		const proxyURL = "https://proxy.example.com:8080"
		var proxyCACm *corev1.ConfigMap

		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			By("create the proxy CA configmap")
			proxyCACm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					utils.ProxyCACertFileName: utils.TestCACert,
				},
			}
			err := testReconcilerInstance.Create(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the proxy CA configmap")
			err := testReconcilerInstance.Delete(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error if the CA text is malformed", func() {
			proxyCACm.Data[utils.ProxyCACertFileName] = "malformed certificate"
			err := testReconcilerInstance.Update(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())

			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: proxyURL,
				ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caConfigMapName,
					},
				},
			}
			_, err = GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to validate proxy CA certificate"))
		})

		It("should use custom Key when ProxyCACertificateRef.Key is set", func() {
			const proxyCertKey = "my-proxy-ca.crt"
			proxyCACm.Data[proxyCertKey] = utils.TestCACert
			err := testReconcilerInstance.Update(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())

			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: proxyURL,
				ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
					LocalObjectReference: corev1.LocalObjectReference{Name: caConfigMapName},
					Key:                  proxyCertKey,
				},
			}

			olsCm, err := GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("proxy_ca_cert_path: /etc/certs/proxy-ca/" + proxyCertKey))
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("proxy_url: " + proxyURL))
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
			MountPath: "/etc/certs/postgres-ca",
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

var _ = Describe("Helper function unit tests", func() {
	var cr *olsv1alpha1.OLSConfig
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.TODO()
		cr = utils.GetDefaultOLSConfigCR()
	})

	Context("buildOLSConfig", func() {
		It("should build OLS config without proxy", func() {
			cr.Spec.OLSConfig.ProxyConfig = nil
			config, err := buildOLSConfig(testReconcilerInstance, ctx, cr, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ProxyConfig).To(BeNil())
			Expect(config.DefaultModel).To(Equal(cr.Spec.OLSConfig.DefaultModel))
			Expect(config.DefaultProvider).To(Equal(cr.Spec.OLSConfig.DefaultProvider))
		})

		It("should build OLS config with proxy but no CA cert", func() {
			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: "http://proxy.example.com:8080",
			}
			config, err := buildOLSConfig(testReconcilerInstance, ctx, cr, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ProxyConfig).NotTo(BeNil())
			Expect(config.ProxyConfig.ProxyURL).To(Equal("http://proxy.example.com:8080"))
			Expect(config.ProxyConfig.ProxyCACertPath).To(BeEmpty())
		})

		It("should build RAG indexes for BYOK only mode", func() {
			cr.Spec.OLSConfig.ByokRAGOnly = true
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{IndexID: "test-index", Image: "test-image"},
			}
			config, err := buildOLSConfig(testReconcilerInstance, ctx, cr, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ReferenceContent.Indexes).To(HaveLen(1))
			Expect(config.ReferenceContent.Indexes[0].ProductDocsIndexId).To(Equal("test-index"))
		})

		It("should include OCP docs when BYOK only mode is disabled", func() {
			cr.Spec.OLSConfig.ByokRAGOnly = false
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{}
			config, err := buildOLSConfig(testReconcilerInstance, ctx, cr, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.ReferenceContent.Indexes).To(HaveLen(1))
			Expect(config.ReferenceContent.Indexes[0].ProductDocsIndexId).To(ContainSubstring("ocp-product-docs"))
		})

		It("should disable user data collection when data collector is disabled", func() {
			// Data collector is disabled by default in test environment
			config, err := buildOLSConfig(testReconcilerInstance, ctx, cr, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.UserDataCollection.FeedbackDisabled).To(BeTrue())
			Expect(config.UserDataCollection.TranscriptsDisabled).To(BeTrue())
		})

		It("should return error when proxy CA certificate ConfigMap does not exist", func() {
			cr.Spec.OLSConfig.ProxyConfig = &olsv1alpha1.ProxyConfig{
				ProxyURL: "http://proxy.example.com:8080",
				ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "nonexistent-proxy-ca",
					},
				},
			}
			// Don't create the ConfigMap - validation should fail
			_, err := buildOLSConfig(testReconcilerInstance, ctx, cr, false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to validate proxy CA certificate"))
			Expect(err.Error()).To(ContainSubstring("nonexistent-proxy-ca"))
		})
	})

	Context("generateMCPServerConfigs", func() {
		It("should return empty list when introspection is disabled and no user servers", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = false
			cr.Spec.MCPServers = nil
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(BeEmpty())
		})

		It("should add OpenShift MCP server when introspection is enabled", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Name).To(Equal("openshift"))
			Expect(servers[0].URL).To(ContainSubstring("8080"))
			Expect(servers[0].Headers).To(HaveKey(utils.K8S_AUTH_HEADER))
		})

		It("should use default timeout for OpenShift MCP server when MCPKubeServerConfig is not set", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.OLSConfig.MCPKubeServerConfig = nil
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Name).To(Equal("openshift"))
			Expect(servers[0].Timeout).To(Equal(utils.OpenShiftMCPServerTimeout))
		})

		It("should use custom timeout for OpenShift MCP server when MCPKubeServerConfig is set", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.OLSConfig.MCPKubeServerConfig = &olsv1alpha1.MCPKubeServerConfiguration{
				Timeout: 120,
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Name).To(Equal("openshift"))
			Expect(servers[0].Timeout).To(Equal(120))
		})

		It("should add user-defined MCP server with kubernetes header", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "custom-server",
					URL:  "http://custom.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "Authorization",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeKubernetes,
							},
						},
					},
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Name).To(Equal("custom-server"))
			Expect(servers[0].Headers["Authorization"]).To(Equal(utils.KUBERNETES_PLACEHOLDER))
		})

		It("should add user-defined MCP server with client header", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "custom-server",
					URL:  "http://custom.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "X-Client-ID",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeClient,
							},
						},
					},
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Headers["X-Client-ID"]).To(Equal(utils.CLIENT_PLACEHOLDER))
		})

		It("should add user-defined MCP server with secret-based header", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "custom-server",
					URL:  "http://custom.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "X-API-Key",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{
									Name: "my-api-key",
								},
							},
						},
					},
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Headers["X-API-Key"]).To(ContainSubstring("/etc/mcp/headers/my-api-key"))
		})

		It("should skip server with invalid secret header (missing secretRef)", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "invalid-server",
					URL:  "http://invalid.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "X-API-Key",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: nil, // Invalid: missing secretRef
							},
						},
					},
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(BeEmpty(), "server with invalid header should be skipped")
		})

		It("should skip server with invalid secret header (empty secretRef name)", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "invalid-server",
					URL:  "http://invalid.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "X-API-Key",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type:      olsv1alpha1.MCPHeaderSourceTypeSecret,
								SecretRef: &corev1.LocalObjectReference{Name: ""}, // Invalid: empty name
							},
						},
					},
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(BeEmpty(), "server with empty secret name should be skipped")
		})

		It("should add server with no headers (unauthenticated)", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name:    "public-server",
					URL:     "http://public.example.com",
					Headers: nil, // No authentication headers
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Name).To(Equal("public-server"))
			Expect(servers[0].URL).To(Equal("http://public.example.com"))
			Expect(servers[0].Headers).To(BeNil(), "server without headers should have nil Headers map")
		})

		It("should add custom timeout when specified", func() {
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name:    "custom-server",
					URL:     "http://custom.example.com",
					Timeout: 60,
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(1))
			Expect(servers[0].Timeout).To(Equal(60))
		})

		It("should include both OpenShift and user-defined servers", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.FeatureGates = []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer}
			cr.Spec.MCPServers = []olsv1alpha1.MCPServerConfig{
				{
					Name: "custom-server",
					URL:  "http://custom.example.com",
				},
			}
			servers, err := generateMCPServerConfigs(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(2))
			Expect(servers[0].Name).To(Equal("openshift"))
			Expect(servers[1].Name).To(Equal("custom-server"))
		})
	})
})

// setFirstLLMProviderNameAndType sets Providers[0] name and type; use utils.With* helpers for richer shapes.
func setFirstLLMProviderNameAndType(cr *olsv1alpha1.OLSConfig, name, providerType string) {
	cr.Spec.LLMConfig.Providers[0].Name = name
	cr.Spec.LLMConfig.Providers[0].Type = providerType
}

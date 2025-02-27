package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"path"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var testURL = "https://testURL"

var _ = Describe("App server assets", func() {
	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions
	var secret *corev1.Secret
	defaultVolumeMode := int32(420)

	Context("complete custom resource", func() {
		BeforeEach(func() {
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceImage: "lightspeed-service:latest",
				Namespace:              OLSNamespaceDefault,
			}
			cr = getDefaultOLSConfigCR()
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
			By("create the provider secret")
			secret, _ = generateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			secretCreationErr := r.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := r.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate a service account", func() {
			sa, err := r.generateServiceAccount(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(sa.Name).To(Equal(OLSAppServerServiceAccountName))
			Expect(sa.Namespace).To(Equal(OLSNamespaceDefault))
		})

		It("should generate the olsconfig config map", func() {
			createTelemetryPullSecret()
			major, minor, err := r.getClusterVersion(ctx)
			Expect(err).NotTo(HaveOccurred())

			cm, err := r.generateOLSConfigMap(context.TODO(), cr)
			// TODO: Update DB
			//OLSRedisMaxMemory := intstr.FromString(RedisMaxMemory)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(OLSNamespaceDefault))
			olsconfigGenerated := AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			olsConfigExpected := AppSrvConfigFile{
				OLSConfig: OLSConfig{
					DefaultModel:    "testModel",
					DefaultProvider: "testProvider",
					Logging: LoggingConfig{
						AppLogLevel:     "INFO",
						LibLogLevel:     "INFO",
						UvicornLogLevel: "INFO",
					},
					// TODO: Update DB
					// ConversationCache: ConversationCacheConfig{
					// 	Type: "redis",
					// 	Redis: RedisCacheConfig{
					// 		Host:            strings.Join([]string{RedisServiceName, OLSNamespaceDefault, "svc"}, "."),
					// 		Port:            RedisServicePort,
					// 		MaxMemory:       &OLSRedisMaxMemory,
					// 		MaxMemoryPolicy: RedisMaxMemoryPolicy,
					// 		PasswordPath:    path.Join(CredentialsMountRoot, RedisSecretName, OLSComponentPasswordFileName),
					// 		CACertPath:      path.Join(OLSAppCertsMountRoot, RedisCertsSecretName, RedisCAVolume, "service-ca.crt"),
					// 	},
					// },
					ConversationCache: ConversationCacheConfig{
						Type: "memory",
						Memory: MemoryCacheConfig{
							MaxEntries: 1000,
						},
					},
					TLSConfig: TLSConfig{
						TLSCertificatePath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.crt"),
						TLSKeyPath:         path.Join(OLSAppCertsMountRoot, OLSCertsSecretName, "tls.key"),
					},
					ReferenceContent: ReferenceContent{
						EmbeddingsModelPath:  "/app-root/embeddings_model",
						ProductDocsIndexId:   "ocp-product-docs-" + major + "_" + minor,
						ProductDocsIndexPath: "/app-root/vector_db/ocp_product_docs/" + major + "." + minor,
					},
					UserDataCollection: UserDataCollectionConfig{
						FeedbackDisabled:    false,
						FeedbackStorage:     "/app-root/ols-user-data/feedback",
						TranscriptsDisabled: false,
						TranscriptsStorage:  "/app-root/ols-user-data/transcripts",
					},
					IntrospectionEnabled: false,
				},
				LLMProviders: []ProviderConfig{
					{
						Name:            "testProvider",
						URL:             testURL,
						CredentialsPath: "/etc/apikeys/test-secret",
						Type:            "bam",
						Models: []ModelConfig{
							{
								Name: "testModel",
								URL:  testURL,
								Parameters: ModelParameters{
									MaxTokensForResponse: 20,
								},
								ContextWindowSize: 32768,
							},
						},
					},
				},
				UserDataCollectorConfig: UserDataCollectorConfig{
					DataStorage: "/app-root/ols-user-data",
					LogLevel:    "",
				},
			}

			Expect(olsconfigGenerated).To(Equal(olsConfigExpected))

			cmHash, err := hashBytes([]byte(cm.Data[OLSConfigFilename]))
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ObjectMeta.Annotations[OLSConfigHashKey]).To(Equal(cmHash))
			deleteTelemetryPullSecret()
		})

		It("should generate configmap with queryFilters", func() {
			crWithFilters := addQueryFiltersToCR(cr)
			cm, err := r.generateOLSConfigMap(context.TODO(), crWithFilters)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(OLSNamespaceDefault))
			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", HaveKeyWithValue("query_filters", ContainElement(MatchAllKeys(Keys{
				"name":         Equal("testFilter"),
				"pattern":      Equal("testPattern"),
				"replace_with": Equal("testReplace"),
			})))))
		})

		It("should generate configmap with Azure OpenAI provider", func() {
			azureOpenAI := addAzureOpenAIProvider(cr)
			cm, err := r.generateOLSConfigMap(context.TODO(), azureOpenAI)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name": Equal("openai"),
				"type": Equal("azure_openai"),
				"azure_openai_config": MatchKeys(Options(IgnoreExtras), Keys{
					"url":              Equal(testURL),
					"credentials_path": Equal("/etc/apikeys/test-secret"),
					"api_version":      Equal("2021-09-01"),
					"deployment_name":  Equal("testDeployment"),
				}),
			}))))
		})

		It("should generate configmap with IBM watsonx provider", func() {
			watsonx := addWatsonxProvider(cr)
			cm, err := r.generateOLSConfigMap(context.TODO(), watsonx)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name":       Equal("watsonx"),
				"type":       Equal("watsonx"),
				"project_id": Equal("testProjectID"),
			}))))
		})

		It("should generate configmap with rhoai_vllm provider", func() {
			provider := addRHOAIProvider(cr)
			cm, err := r.generateOLSConfigMap(context.TODO(), provider)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name": Equal("rhoai_vllm"),
				"type": Equal("rhoai_vllm"),
			}))))
		})

		It("should generate configmap with rhelia_vllm provider", func() {
			provider := addRHELAIProvider(cr)
			cm, err := r.generateOLSConfigMap(context.TODO(), provider)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("llm_providers", ContainElement(MatchKeys(Options(IgnoreExtras), Keys{
				"name": Equal("rhelai_vllm"),
				"type": Equal("rhelai_vllm"),
			}))))
		})

		It("should generate configmap with introspectionEnabled", func() {
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cm, err := r.generateOLSConfigMap(context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())

			var olsConfigMap map[string]interface{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsConfigMap).To(HaveKeyWithValue("ols_config", MatchKeys(Options(IgnoreExtras), Keys{
				"introspection_enabled": Equal(true),
			})))
		})

		It("should generate the OLS deployment", func() {
			By("generate full deployment when telemetry pull secret exists")
			createTelemetryPullSecret()

			dep, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
			// application container
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-service-api"))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf([]corev1.VolumeMount{
				{
					Name:      "secret-test-secret",
					MountPath: path.Join(APIKeyMountRoot, "test-secret"),
					ReadOnly:  true,
				},
				{
					Name:      "secret-lightspeed-tls",
					MountPath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName),
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
				{
					Name:      "ols-user-data",
					ReadOnly:  false,
					MountPath: "/app-root/ols-user-data",
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("1Gi")},
				Claims:   []corev1.ResourceClaim{},
			}))
			// telemetry container
			Expect(dep.Spec.Template.Spec.Containers[1].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal("lightspeed-service-user-data-collector"))
			Expect(dep.Spec.Template.Spec.Containers[1].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[1].Command).To(Equal([]string{"python3.11", "/app-root/ols/user_data_collection/data_collector.py"}))
			Expect(dep.Spec.Template.Spec.Containers[1].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[1].VolumeMounts).To(ConsistOf([]corev1.VolumeMount{
				{
					Name:      "ols-user-data",
					ReadOnly:  false,
					MountPath: "/app-root/ols-user-data",
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[1].Resources).To(Equal(corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
				Claims:   []corev1.ResourceClaim{},
			}))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf([]corev1.Volume{
				{
					Name: "secret-test-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "secret-lightspeed-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  OLSCertsSecretName,
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "cm-olsconfig",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSConfigCmName},
							DefaultMode:          &defaultVolumeMode,
						},
					},
				},
				{
					Name: "ols-user-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			}))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(generateAppServerSelectorLabels()))

			By("generate deployment without data collector when telemetry pull secret does not exist")
			deleteTelemetryPullSecret()
			dep, err = r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			// application container
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-service-api"))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf([]corev1.VolumeMount{
				{
					Name:      "secret-test-secret",
					MountPath: path.Join(APIKeyMountRoot, "test-secret"),
					ReadOnly:  true,
				},
				{
					Name:      "secret-lightspeed-tls",
					MountPath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName),
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
			}))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf([]corev1.Volume{
				{
					Name: "secret-test-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "secret-lightspeed-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  OLSCertsSecretName,
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "cm-olsconfig",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSConfigCmName},
							DefaultMode:          &defaultVolumeMode,
						},
					},
				},
			}))

			By("generate deployment without data collector when telemetry pull secret does not contain telemetry token")
			createTelemetryPullSecretWithoutTelemetryToken()
			dep, err = r.generateOLSDeployment(cr)

			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			// application container
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-service-api"))
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf([]corev1.VolumeMount{
				{
					Name:      "secret-test-secret",
					MountPath: path.Join(APIKeyMountRoot, "test-secret"),
					ReadOnly:  true,
				},
				{
					Name:      "secret-lightspeed-tls",
					MountPath: path.Join(OLSAppCertsMountRoot, OLSCertsSecretName),
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
			}))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf([]corev1.Volume{
				{
					Name: "secret-test-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "test-secret",
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "secret-lightspeed-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  OLSCertsSecretName,
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "cm-olsconfig",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSConfigCmName},
							DefaultMode:          &defaultVolumeMode,
						},
					},
				},
			}))
			deleteTelemetryPullSecret()
		})

		It("should generate the OLS service", func() {
			service, err := r.generateService(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(OLSAppServerServiceName))
			Expect(service.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(service.Spec.Selector).To(Equal(generateAppServerSelectorLabels()))
			Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
				{
					Name:       "https",
					Port:       OLSAppServerServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("https"),
				},
			}))
		})

		It("should switch data collection on and off as CR defines in .spec.ols_config.user_data_collection", func() {
			createTelemetryPullSecret()
			defer deleteTelemetryPullSecret()
			By("Switching data collection off")
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}
			cm, err := r.generateOLSConfigMap(context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated := AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.FeedbackDisabled).To(BeTrue())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.TranscriptsDisabled).To(BeTrue())

			deployment, err := r.generateOLSDeployment(cr)
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
			cm, err = r.generateOLSConfigMap(context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.FeedbackDisabled).To(BeFalse())
			Expect(olsconfigGenerated.OLSConfig.UserDataCollection.TranscriptsDisabled).To(BeFalse())

			deployment, err = r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deployment.Spec.Template.Spec.Containers[1].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(deployment.Spec.Template.Spec.Containers[1].Name).To(Equal("lightspeed-service-user-data-collector"))
			Expect(deployment.Spec.Template.Spec.Containers[1].Resources).ToNot(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers[1].Command).To(Equal([]string{"python3.11", "/app-root/ols/user_data_collection/data_collector.py"}))
			Expect(deployment.Spec.Template.Spec.Containers[1].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", OLSConfigFilename),
				},
			}))
			Expect(deployment.Spec.Template.Spec.Containers[1].VolumeMounts).To(ConsistOf([]corev1.VolumeMount{
				{
					Name:      "ols-user-data",
					ReadOnly:  false,
					MountPath: "/app-root/ols-user-data",
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
			}))
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
			cm, err := r.generateOLSConfigMap(context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			olsconfigGenerated := AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsconfigGenerated.OLSConfig.TLSConfig.TLSCertificatePath).To(Equal(path.Join(OLSAppCertsMountRoot, tlsSecretName, "tls.crt")))
			Expect(olsconfigGenerated.OLSConfig.TLSConfig.TLSKeyPath).To(Equal(path.Join(OLSAppCertsMountRoot, tlsSecretName, "tls.key")))

			deployment, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "secret-" + tlsSecretName,
					MountPath: path.Join(OLSAppCertsMountRoot, tlsSecretName),
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
					IndexPath: "/rag/vector_db/ocp_product_docs/4.15",
					IndexID:   "ocp-product-docs-4_15",
					Image:     "rag-ocp-product-docs:4.15",
				},
				{
					IndexPath: "/rag/vector_db/ansible_docs/2.18",
					IndexID:   "ansible-docs-2_18",
					Image:     "rag-ansible-docs:2.18",
				},
			}
			deployment, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: RAGVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}))

			Expect(deployment.Spec.Template.Spec.InitContainers).To(ConsistOf(
				corev1.Container{
					Name:    "rag-0",
					Image:   "rag-ocp-product-docs:4.15",
					Command: []string{"sh", "-c", "mkdir -p /rag-data/rag-0 && cp -a /rag/vector_db/ocp_product_docs/4.15 /rag-data/rag-0"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      RAGVolumeName,
							MountPath: "/rag-data",
						},
					},
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				corev1.Container{
					Name:    "rag-1",
					Image:   "rag-ansible-docs:2.18",
					Command: []string{"sh", "-c", "mkdir -p /rag-data/rag-1 && cp -a /rag/vector_db/ansible_docs/2.18 /rag-data/rag-1"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      RAGVolumeName,
							MountPath: "/rag-data",
						},
					},
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			))
		})

	})

	Context("empty custom resource", func() {
		BeforeEach(func() {
			cr = getEmptyOLSConfigCR()
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceImage: "lightspeed-service:latest",
				Namespace:              OLSNamespaceDefault,
			}
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
		})

		It("should generate a service account", func() {
			sa, err := r.generateServiceAccount(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(sa.Name).To(Equal(OLSAppServerServiceAccountName))
			Expect(sa.Namespace).To(Equal(OLSNamespaceDefault))
		})

		It("should generate the olsconfig config map", func() {
			// todo: this test is not complete
			// generateOLSConfigMap should return an error if the CR is missing required fields
			createTelemetryPullSecret()
			major, minor, err := r.getClusterVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
			cm, err := r.generateOLSConfigMap(context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(OLSNamespaceDefault))
			expectedConfigStr := `llm_providers: []
ols_config:
  conversation_cache:
    memory:
      max_entries: 1000
    type: memory
  logging_config:
    app_log_level: ""
    lib_log_level: ""
    uvicorn_log_level: ""
  reference_content:
    embeddings_model_path: /app-root/embeddings_model
    product_docs_index_id: ocp-product-docs-` + major + `_` + minor + `
    product_docs_index_path: /app-root/vector_db/ocp_product_docs/` + major + `.` + minor + `
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
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &actualConfig)
			Expect(err).NotTo(HaveOccurred())

			var expectedConfig map[string]interface{}
			err = yaml.Unmarshal([]byte(expectedConfigStr), &expectedConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(actualConfig).To(Equal(expectedConfig))
			deleteTelemetryPullSecret()
		})

		It("should generate the olsconfig config map without user_data_collector_config", func() {
			// pull-secret with out telemetry token should make the datacollection disabled
			// and user_data_collector_config should not be present in the config
			createTelemetryPullSecretWithoutTelemetryToken()
			major, minor, err := r.getClusterVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
			cm, err := r.generateOLSConfigMap(context.TODO(), cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(OLSNamespaceDefault))
			expectedConfigStr := `llm_providers: []
ols_config:
  conversation_cache:
    memory:
      max_entries: 1000
    type: memory
  logging_config:
    app_log_level: ""
    lib_log_level: ""
    uvicorn_log_level: ""
  reference_content:
    embeddings_model_path: /app-root/embeddings_model
    product_docs_index_id: ocp-product-docs-` + major + `_` + minor + `
    product_docs_index_path: /app-root/vector_db/ocp_product_docs/` + major + `.` + minor + `
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
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &actualConfig)
			Expect(err).NotTo(HaveOccurred())

			var expectedConfig map[string]interface{}
			err = yaml.Unmarshal([]byte(expectedConfigStr), &expectedConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(actualConfig).To(Equal(expectedConfig))
			deleteTelemetryPullSecret()
		})

		It("should generate the OLS service", func() {
			service, err := r.generateService(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(OLSAppServerServiceName))
			Expect(service.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(service.Spec.Selector).To(Equal(generateAppServerSelectorLabels()))
			Expect(service.Annotations[ServingCertSecretAnnotationKey]).To(Equal(OLSCertsSecretName))
			Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
				{
					Name:       "https",
					Port:       OLSAppServerServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("https"),
				},
			}))
		})

		It("should generate the OLS deployment", func() {
			// todo: update this test after updating the test for generateOLSConfigMap
			createTelemetryPullSecret()
			defer deleteTelemetryPullSecret()
			dep, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-service-api"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: OLSAppServerContainerPort,
					Name:          "https",
					Protocol:      corev1.ProtocolTCP,
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "OLS_CONFIG_FILE",
					Value: path.Join("/etc/ols", OLSConfigFilename),
				},
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(ConsistOf([]corev1.VolumeMount{
				{
					Name:      "secret-lightspeed-tls",
					MountPath: "/etc/certs/lightspeed-tls",
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
				{
					Name:      "ols-user-data",
					ReadOnly:  false,
					MountPath: "/app-root/ols-user-data",
				},
			}))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf([]corev1.Volume{
				{
					Name: "secret-lightspeed-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  OLSCertsSecretName,
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
				{
					Name: "cm-olsconfig",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSConfigCmName},
							DefaultMode:          &defaultVolumeMode,
						},
					},
				},
				{
					Name: "ols-user-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			}))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(generateAppServerSelectorLabels()))
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
			serviceMonitor, err := r.generateServiceMonitor(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceMonitor.Name).To(Equal(AppServerServiceMonitorName))
			Expect(serviceMonitor.Namespace).To(Equal(OLSNamespaceDefault))
			valFalse := false
			serverName := "lightspeed-app-server.openshift-lightspeed.svc"
			Expect(serviceMonitor.Spec.Endpoints).To(ConsistOf(
				monv1.Endpoint{
					Port:     "https",
					Path:     AppServerMetricsPath,
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
					BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			))
			Expect(serviceMonitor.Spec.Selector.MatchLabels).To(Equal(generateAppServerSelectorLabels()))
			Expect(serviceMonitor.ObjectMeta.Labels).To(HaveKeyWithValue("openshift.io/user-monitoring", "false"))
		})

		It("should generate the OLS prometheus rules", func() {
			prometheusRule, err := r.generatePrometheusRule(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(prometheusRule.Name).To(Equal(AppServerPrometheusRuleName))
			Expect(prometheusRule.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(len(prometheusRule.Spec.Groups[0].Rules)).To(Equal(4))
		})

		It("should generate the SAR cluster role", func() {
			clusterRole, err := r.generateSARClusterRole(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterRole.Name).To(Equal(OLSAppServerSARRoleName))
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
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceImage: "lightspeed-service:latest",
				Namespace:              OLSNamespaceDefault,
			}
			cr = getDefaultOLSConfigCR()
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
			By("create the provider secret")
			secret, _ = generateRandomSecret()
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "test-secret",
				},
			})
			err := r.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			By("create the additional CA configmap")
			additionalCACm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caConfigMapName,
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string]string{
					certFilename: testCACert,
				},
			}
			err = r.Create(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())

		})

		AfterEach(func() {
			By("Delete the provider secret")
			err := r.Delete(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
			By("Delete the additional CA configmap")
			err = r.Delete(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should update OLS config and mount volumes for additional CA", func() {
			olsCm, err := r.generateOLSConfigMap(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[OLSConfigFilename]).NotTo(ContainSubstring("extra_ca:"))

			dep, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Volumes).NotTo(ContainElement(
				corev1.Volume{
					Name: AdditionalCAVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: caConfigMapName,
							},
							DefaultMode: &defaultVolumeMode,
						},
					},
				}))
			Expect(dep.Spec.Template.Spec.Volumes).NotTo(ContainElement(
				corev1.Volume{
					Name: CertBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}))

			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: caConfigMapName,
			}

			olsCm, err = r.generateOLSConfigMap(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[OLSConfigFilename]).To(ContainSubstring("extra_ca:\n  - /etc/certs/ols-additional-ca/additional-ca.crt"))
			Expect(olsCm.Data[OLSConfigFilename]).To(ContainSubstring("certificate_directory: /etc/certs/cert-bundle"))

			dep, err = r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Volumes).To(ContainElements(
				corev1.Volume{
					Name: AdditionalCAVolumeName,
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
					Name: CertBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			))

		})

		It("should return error if the CA text is malformed", func() {
			additionalCACm.Data[certFilename] = "malformed certificate"
			err := r.Update(ctx, additionalCACm)
			Expect(err).NotTo(HaveOccurred())

			cr.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{
				Name: caConfigMapName,
			}
			_, err = r.generateOLSConfigMap(ctx, cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to validate additional CA certificate"))

		})

	})
})

func generateRandomSecret() (*corev1.Secret, error) {
	randomPassword := make([]byte, 12)
	_, _ = rand.Read(randomPassword)
	// Encode the password to base64
	encodedPassword := base64.StdEncoding.EncodeToString(randomPassword)
	passwordHash, _ := hashBytes([]byte(encodedPassword))

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Generate self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-secret",
			Namespace:   "openshift-lightspeed",
			Labels:      generateAppServerSelectorLabels(),
			Annotations: map[string]string{},
		},
		Data: map[string][]byte{
			"client_secret": []byte(passwordHash),
			"tls.key":       privateKeyPEM,
			"tls.crt":       certPEM,
		},
	}

	return &secret, nil
}

func generateRandomConfigMap() (*corev1.ConfigMap, error) {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "openshift-lightspeed",
		},
		Data: map[string]string{
			"service-ca.crt": "random ca cert content",
		},
	}
	return &configMap, nil
}

func getDefaultOLSConfigCR() *olsv1alpha1.OLSConfig {
	// fill the CR with all implemented fields in the configuration file
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: "testProvider",
						URL:  testURL,
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Type: "bam",
						Models: []olsv1alpha1.ModelSpec{
							{
								Name: "testModel",
								URL:  testURL,
								Parameters: olsv1alpha1.ModelParametersSpec{
									MaxTokensForResponse: 20,
								},
								ContextWindowSize: 32768,
							},
						},
					},
				},
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				DefaultModel:    "testModel",
				DefaultProvider: "testProvider",
				LogLevel:        "INFO",
			},
		},
	}
}

func getEmptyOLSConfigCR() *olsv1alpha1.OLSConfig {
	// The CR has no fields set in its specs
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
}

func addQueryFiltersToCR(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.OLSConfig.QueryFilters = []olsv1alpha1.QueryFiltersSpec{
		{
			Name:        "testFilter",
			Pattern:     "testPattern",
			ReplaceWith: "testReplace",
		},
	}
	return cr
}

func addAzureOpenAIProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "openai"
	cr.Spec.LLMConfig.Providers[0].Type = "azure_openai"
	cr.Spec.LLMConfig.Providers[0].AzureDeploymentName = "testDeployment"
	cr.Spec.LLMConfig.Providers[0].APIVersion = "2021-09-01"
	return cr
}

func addWatsonxProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "watsonx"
	cr.Spec.LLMConfig.Providers[0].Type = "watsonx"
	cr.Spec.LLMConfig.Providers[0].WatsonProjectID = "testProjectID"
	return cr
}

func addRHOAIProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "rhoai_vllm"
	cr.Spec.LLMConfig.Providers[0].Type = "rhoai_vllm"
	return cr
}

func addRHELAIProvider(cr *olsv1alpha1.OLSConfig) *olsv1alpha1.OLSConfig {
	cr.Spec.LLMConfig.Providers[0].Name = "rhelai_vllm"
	cr.Spec.LLMConfig.Providers[0].Type = "rhelai_vllm"
	return cr
}

func createTelemetryPullSecret() {
	const telemetryToken = `
		{
			"auths": {
				"cloud.openshift.com": {
					"auth": "testkey",
					"email": "testm@test.test"
				}
			}
		}
		`
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(telemetryToken),
		},
	}

	err := k8sClient.Create(ctx, pullSecret)
	Expect(err).NotTo(HaveOccurred())
}

func createTelemetryPullSecretWithoutTelemetryToken() {
	const telemetryToken = `
		{
			"auths": {
				"other.token": {
					"auth": "testkey",
					"email": "testm@test.test"
				}
			}
		}
		`
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(telemetryToken),
		},
	}

	err := k8sClient.Create(ctx, pullSecret)
	Expect(err).NotTo(HaveOccurred())
}

func deleteTelemetryPullSecret() {
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: "openshift-config",
		},
	}
	err := k8sClient.Delete(ctx, pullSecret)
	Expect(err).NotTo(HaveOccurred())
}

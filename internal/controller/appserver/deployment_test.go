package appserver

import (
	"context"
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("App server deployment generation", func() {
	var cr *olsv1alpha1.OLSConfig
	var secret *corev1.Secret
	var configmap *corev1.ConfigMap

	Context("complete custom resource - deployment tests", func() {
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

		It("should generate the OLS deployment with telemetry", func() {
			By("generate full deployment when telemetry pull secret exists")
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OLSAppServerImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.OLSAppServerContainerName))

			By("generate deployment without data collector when telemetry pull secret does not exist")
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))

			By("generate deployment without data collector when telemetry pull secret does not contain telemetry token")
			utils.CreateTelemetryPullSecret(ctx, k8sClient, false)
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))

			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should use configured log level for data collector container", func() {
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)

			By("using default INFO log level when not specified")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{}
			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.DataverseExporterContainerName))
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(ContainElement(string(olsv1alpha1.LogLevelInfo)))

			By("using DEBUG log level when configured")
			cr.Spec.OLSDataCollectorConfig = olsv1alpha1.OLSDataCollectorSpec{
				LogLevel: olsv1alpha1.LogLevelDebug,
			}
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers[1].Args).To(ContainElement(string(olsv1alpha1.LogLevelDebug)))

			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should generate RAG volumes and initContainers", func() {
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					IndexPath: "/rag/vector_db/ocp_product_docs/4.19",
					IndexID:   "ocp-product-docs-4_19",
					Image:     "rag-ocp-product-docs:4.19",
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
			Expect(deployment.Spec.Template.Spec.InitContainers).ToNot(BeEmpty())
		})

		It("should generate deployment with MCP server sidecar when introspectionEnabled is true", func() {
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			defer utils.DeleteTelemetryPullSecret(ctx, k8sClient)

			By("Enabling introspection")
			cr.Spec.OLSConfig.IntrospectionEnabled = true

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.OLSAppServerDeploymentName))

			// Should have 3 containers: main app, telemetry, and MCP server
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(3))

			// Verify OpenShift MCP server container
			openshiftMCPServerContainer := dep.Spec.Template.Spec.Containers[2]
			Expect(openshiftMCPServerContainer.Name).To(Equal(utils.OpenShiftMCPServerContainerName))
			Expect(openshiftMCPServerContainer.Image).To(Equal(utils.OpenShiftMCPServerImageDefault))
			Expect(openshiftMCPServerContainer.Command).To(Equal([]string{
				"/openshift-mcp-server",
				"--config", utils.GetOpenShiftMCPServerConfigPath(),
				"--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerPort),
			}))

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
			By("introspection enabled, data collection enabled")
			utils.CreateTelemetryPullSecret(ctx, k8sClient, true)
			cr.Spec.OLSConfig.IntrospectionEnabled = true
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    false,
				TranscriptsDisabled: false,
			}

			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(3))
			Expect(dep.Spec.Template.Spec.Containers[2].Name).To(Equal(utils.OpenShiftMCPServerContainerName))

			By("introspection enabled, data collection disabled")
			cr.Spec.OLSConfig.UserDataCollection = olsv1alpha1.UserDataCollectionSpec{
				FeedbackDisabled:    true,
				TranscriptsDisabled: true,
			}

			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(dep.Spec.Template.Spec.Containers[1].Name).To(Equal(utils.OpenShiftMCPServerContainerName))

			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
		})

		It("should deploy MCP container when introspection is enabled regardless of telemetry settings", func() {
			By("introspection enabled with no telemetry pull secret")
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
			Expect(mcpContainer.Command).To(Equal([]string{
				"/openshift-mcp-server",
				"--config", utils.GetOpenShiftMCPServerConfigPath(),
				"--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerPort),
			}))
		})
	})

	Context("empty custom resource - deployment tests", func() {
		BeforeEach(func() {
			cr = &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testOLSConfigCR",
					Namespace: utils.OLSNamespaceDefault,
				},
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{},
					},
					OLSConfig: olsv1alpha1.OLSSpec{
						DefaultModel:    "testModel",
						DefaultProvider: "testProvider",
					},
				},
			}
		})

		It("should generate the OLS deployment with required volumes and probes", func() {
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
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(utils.GenerateAppServerSelectorLabels()))
			Expect(dep.Spec.Template.Spec.Containers[0].LivenessProbe).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Port).To(Equal(intstr.FromString("https")))
			Expect(dep.Spec.Template.Spec.Containers[0].ReadinessProbe).ToNot(BeNil())
			Expect(dep.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Port).To(Equal(intstr.FromString("https")))
		})
	})

	Context("Additional CA - deployment tests", func() {
		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
		})

		It("should generate ImagePullSecrets in the app server deployment pod template", func() {
			Expect(cr.Spec.OLSConfig.ImagePullSecrets).To(BeNil())
			dep, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(BeNil())

			imagePullSecrets := []corev1.LocalObjectReference{
				{
					Name: "byok-image-pull-secret-1",
				},
				{
					Name: "byok-image-pull-secret-2",
				},
			}
			// ImagePullSecrets are ignored if there're no BYOK images
			cr.Spec.OLSConfig.ImagePullSecrets = imagePullSecrets
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(BeNil())

			// ImagePullSecrets should be set when there are BYOK RAG images
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image:     "rag-image-1",
					IndexPath: "/path/to/index-1",
					IndexID:   "index-id-1",
				},
			}
			dep, err = GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(Equal(imagePullSecrets))
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
			// Config always uses /etc/certs/lightspeed-tls/ path regardless of secret name
			Expect(olsconfigGenerated.OLSConfig.TLSConfig.TLSCertificatePath).To(Equal(path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls", "tls.crt")))
			Expect(olsconfigGenerated.OLSConfig.TLSConfig.TLSKeyPath).To(Equal(path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls", "tls.key")))

			deployment, err := GenerateOLSDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			// Volume mount uses canonical name regardless of secret name
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "secret-lightspeed-tls",
					MountPath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls"),
					ReadOnly:  true,
				},
			))
			// Volume has canonical name but references the user's secret
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(
				corev1.Volume{
					Name: "secret-lightspeed-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  tlsSecretName, // References user's secret
							DefaultMode: &defaultVolumeMode,
						},
					},
				},
			))
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
				string(olsv1alpha1.LogLevelInfo),
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
	})

	Context("Additional CA", func() {
		const caConfigMapName = "test-ca-configmap"
		const certFilename = "additional-ca.crt"
		var secret *corev1.Secret
		var configmap *corev1.ConfigMap
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

	})

	Context("Proxy settings", func() {
		const caConfigMapName = "test-ca-configmap"
		const proxyURL = "https://proxy.example.com:8080"
		var secret *corev1.Secret
		var configmap *corev1.ConfigMap
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
			configMapDeletionErr := testReconcilerInstance.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
			By("Delete the proxy CA configmap")
			err = testReconcilerInstance.Delete(ctx, proxyCACm)
			Expect(err).NotTo(HaveOccurred())
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
				ProxyURL: proxyURL,
				ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caConfigMapName,
					},
					// No Key specified - tests backward compatibility
				},
			}

			olsCm, err = GenerateOLSConfigMap(testReconcilerInstance, ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("proxy_ca_cert_path: /etc/certs/proxy-ca/" + utils.ProxyCACertFileName))
			Expect(olsCm.Data[utils.OLSConfigFilename]).To(ContainSubstring("proxy_url: " + proxyURL))

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
							Items: []corev1.KeyToPath{
								{Key: utils.ProxyCACertFileName, Path: utils.ProxyCACertFileName},
							},
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

	})

})

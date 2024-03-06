package controller

import (
	"path"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("App server assets", func() {

	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions

	Context("complete custom resource", func() {
		BeforeEach(func() {
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceImage: "lightspeed-service:latest",
			}
			cr = getCompleteOLSConfigCR()
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
			Expect(sa.Namespace).To(Equal(cr.Namespace))
		})

		It("should generate the olsconfig config map", func() {
			cm, err := r.generateOLSConfigMap(cr)
			OLSRedisMaxMemory := intstr.FromString(OLSAppRedisMaxMemory)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(cr.Namespace))
			olsconfigGenerated := AppSrvConfigFile{}
			err = yaml.Unmarshal([]byte(cm.Data[OLSConfigFilename]), &olsconfigGenerated)
			Expect(err).NotTo(HaveOccurred())

			olsConfigExpected := AppSrvConfigFile{
				OLSConfig: OLSConfig{
					DefaultModel:    "testModel",
					DefaultProvider: "testProvider",
					Logging: LoggingConfig{
						AppLogLevel: "INFO",
						LibLogLevel: "INFO",
					},
					ConversationCache: ConversationCacheConfig{
						Type: "redis",
						Redis: RedisCacheConfig{
							Host:            strings.Join([]string{OLSAppRedisServiceName, cr.Namespace, "svc"}, "."),
							Port:            OLSAppRedisServicePort,
							MaxMemory:       &OLSRedisMaxMemory,
							MaxMemoryPolicy: OLSAppRedisMaxMemoryPolicy,
							PasswordPath:    path.Join(CredentialsMountRoot, OLSAppRedisSecretName, OLSPasswordFileName),
							CACertPath:      path.Join(OLSAppCertsMountRoot, OLSAppRedisCertsSecretName, OLSRedisCAVolumeName, "service-ca.crt"),
						},
					},
				},
				LLMProviders: []ProviderConfig{
					{
						Name:            "testProvider",
						URL:             "testURL",
						CredentialsPath: "/etc/apikeys/test-secret/apitoken",
						Models: []ModelConfig{
							{
								Name: "testModel",
								URL:  "testURL",
							},
						},
					},
				},
			}

			Expect(olsconfigGenerated).To(Equal(olsConfigExpected))

			cmHash, err := hashBytes([]byte(cm.Data[OLSConfigFilename]))
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ObjectMeta.Annotations[OLSConfigHashKey]).To(Equal(cmHash))

		})

		It("should generate the OLS deployment", func() {
			dep, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal(cr.Namespace))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-service-api"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: OLSAppServerContainerPort,
					Name:          "http",
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
					Name:      "secret-lightspeed-redis-secret",
					MountPath: "/etc/credentials/lightspeed-redis-secret",
					ReadOnly:  true,
				},
				{
					Name:      "secret-test-secret",
					MountPath: path.Join(APIKeyMountRoot, "test-secret"),
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsredisca",
					MountPath: "/etc/certs/lightspeed-redis-certs/cm-olsredisca",
					ReadOnly:  true,
				},
			}))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf([]corev1.Volume{
				{
					Name: "secret-test-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "test-secret",
						},
					},
				},
				{
					Name: "secret-lightspeed-redis-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: OLSAppRedisSecretName,
						},
					},
				},
				{
					Name: "cm-olsconfig",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSConfigCmName},
						},
					},
				},
				{
					Name: "cm-olsredisca",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSRedisCACmName},
						},
					},
				},
			}))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(generateAppServerSelectorLabels()))
		})

		It("should generate the OLS service", func() {
			service, err := r.generateService(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(OLSAppServerServiceName))
			Expect(service.Namespace).To(Equal(cr.Namespace))
			Expect(service.Spec.Selector).To(Equal(generateAppServerSelectorLabels()))
			Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
				{
					Name:       "http",
					Port:       OLSAppServerServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("http"),
				},
			}))
		})

	})

	Context("empty custom resource", func() {
		BeforeEach(func() {
			cr = getEmptyOLSConfigCR()
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceImage: "lightspeed-service:latest",
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
			Expect(sa.Namespace).To(Equal("openshift-lightspeed"))
		})

		It("should generate the olsconfig config map", func() {
			// todo: this test is not complete
			// generateOLSConfigMap should return an error if the CR is missing required fields
			cm, err := r.generateOLSConfigMap(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal("openshift-lightspeed"))
			const expectedConfigStr = `llm_providers: []
ols_config:
  conversation_cache:
    redis:
      ca_cert_path: /etc/certs/lightspeed-redis-certs/cm-olsredisca/service-ca.crt
      host: lightspeed-redis-server.openshift-lightspeed.svc
      max_memory: 1024mb
      max_memory_policy: allkeys-lru
      password_path: /etc/credentials/lightspeed-redis-secret/password
      port: 6379
    type: redis
  logging_config:
    app_log_level: ""
    lib_log_level: ""
`
			Expect(cm.Data[OLSConfigFilename]).To(Equal(expectedConfigStr))
		})

		It("should generate the OLS service", func() {
			service, err := r.generateService(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Name).To(Equal(OLSAppServerServiceName))
			Expect(service.Namespace).To(Equal("openshift-lightspeed"))
			Expect(service.Spec.Selector).To(Equal(generateAppServerSelectorLabels()))
			Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
				{
					Name:       "http",
					Port:       OLSAppServerServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("http"),
				},
			}))
		})

		It("should generate the OLS deployment", func() {
			// todo: update this test after updating the test for generateOLSConfigMap
			cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecretRef.Name = OLSAppRedisSecretName
			dep, err := r.generateOLSDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(OLSAppServerDeploymentName))
			Expect(dep.Namespace).To(Equal("openshift-lightspeed"))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(rOptions.LightspeedServiceImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-service-api"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
				{
					ContainerPort: OLSAppServerContainerPort,
					Name:          "http",
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
					Name:      "secret-lightspeed-redis-secret",
					MountPath: "/etc/credentials/lightspeed-redis-secret",
					ReadOnly:  true,
				},
				{
					Name:      "cm-olsconfig",
					MountPath: "/etc/ols",
					ReadOnly:  true,
				},

				{
					Name:      "cm-olsredisca",
					MountPath: "/etc/certs/lightspeed-redis-certs/cm-olsredisca",
					ReadOnly:  true,
				},
			}))
			Expect(dep.Spec.Template.Spec.Volumes).To(ConsistOf([]corev1.Volume{
				{
					Name: "secret-lightspeed-redis-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: OLSAppRedisSecretName,
						},
					},
				},
				{
					Name: "cm-olsconfig",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: OLSConfigCmName},
						},
					},
				},
				{
					Name: "cm-olsredisca",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: OLSRedisCACmName,
							},
						},
					},
				},
			}))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(generateAppServerSelectorLabels()))

		})
	})

})

func getCompleteOLSConfigCR() *olsv1alpha1.OLSConfig {
	// fill the CR with all implemented fields in the configuration file
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "openshift-lightspeed",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: "testProvider",
						URL:  "testURL",
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						Models: []olsv1alpha1.ModelSpec{
							{
								Name: "testModel",
								URL:  "testURL",
							},
						},
					},
				},
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				DefaultModel:    "testModel",
				DefaultProvider: "testProvider",
				LogLevel:        "INFO",
				ConversationCache: olsv1alpha1.ConversationCacheSpec{
					Type: olsv1alpha1.Redis,
					Redis: olsv1alpha1.RedisSpec{
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: OLSAppRedisSecretName,
						},
					},
				},
			},
		},
	}
}

func getEmptyOLSConfigCR() *olsv1alpha1.OLSConfig {
	// The CR has no fields set in its specs
	return &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "openshift-lightspeed",
		},
	}

}

package controller

import (
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

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
		})

		It("should generate a service account", func() {
			sa, err := r.generateServiceAccount(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(sa.Name).To(Equal(OLSAppServerServiceAccountName))
			Expect(sa.Namespace).To(Equal(OLSNamespaceDefault))
		})

		It("should generate the olsconfig config map", func() {
			cm, err := r.generateOLSConfigMap(cr)
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
						AppLogLevel: "INFO",
						LibLogLevel: "INFO",
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
				DevConfig: DevConfig{
					DisableAuth: false,
				},
			}

			Expect(olsconfigGenerated).To(Equal(olsConfigExpected))

			cmHash, err := hashBytes([]byte(cm.Data[OLSConfigFilename]))
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ObjectMeta.Annotations[OLSConfigHashKey]).To(Equal(cmHash))
		})

		It("should generate configmap with queryFilters", func() {
			crWithFilters := addQueryFiltersToCR(cr)
			cm, err := r.generateOLSConfigMap(crWithFilters)
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

		It("should generate the OLS deployment", func() {
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
					Name: "secret-lightspeed-tls",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: OLSCertsSecretName,
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
					Name: "ols-user-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			}))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(generateAppServerSelectorLabels()))
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
			cm, err := r.generateOLSConfigMap(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(OLSConfigCmName))
			Expect(cm.Namespace).To(Equal(OLSNamespaceDefault))
			const expectedConfigStr = `dev_config:
  disable_auth: false
llm_providers: []
ols_config:
  conversation_cache:
    memory:
      max_entries: 1000
    type: memory
  logging_config:
    app_log_level: ""
    lib_log_level: ""
  tls_config:
    tls_certificate_path: /etc/certs/lightspeed-tls/tls.crt
    tls_key_path: /etc/certs/lightspeed-tls/tls.key
`
			Expect(cm.Data[OLSConfigFilename]).To(Equal(expectedConfigStr))
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
							SecretName: OLSCertsSecretName,
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
		})
	})
})

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
				DisableAuth:     false,
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

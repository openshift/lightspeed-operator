package utils

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("StatusHasCondition", func() {
	var testStatus olsv1alpha1.OLSConfigStatus

	BeforeEach(func() {
		testStatus = olsv1alpha1.OLSConfigStatus{
			Conditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Reason:             "AllComponentsReady",
					Message:            "All components are ready",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.Now(),
				},
				{
					Type:               "Degraded",
					Status:             metav1.ConditionFalse,
					Reason:             "NoIssues",
					Message:            "No degradation detected",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.Now(),
				},
			},
		}
	})

	It("should find matching condition with same Type, Status, Reason, and Message", func() {
		condition := metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AllComponentsReady",
			Message: "All components are ready",
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeTrue())
	})

	It("should return false when condition Type does not match", func() {
		condition := metav1.Condition{
			Type:    "NonExistent",
			Status:  metav1.ConditionTrue,
			Reason:  "AllComponentsReady",
			Message: "All components are ready",
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeFalse())
	})

	It("should return false when Status does not match", func() {
		condition := metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse, // Different from actual
			Reason:  "AllComponentsReady",
			Message: "All components are ready",
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeFalse())
	})

	It("should return false when Reason does not match", func() {
		condition := metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "DifferentReason", // Different from actual
			Message: "All components are ready",
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeFalse())
	})

	It("should return false when Message does not match", func() {
		condition := metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AllComponentsReady",
			Message: "Different message", // Different from actual
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeFalse())
	})

	It("should ignore ObservedGeneration when comparing conditions", func() {
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "AllComponentsReady",
			Message:            "All components are ready",
			ObservedGeneration: 999, // Different from actual (1)
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeTrue())
	})

	It("should ignore LastTransitionTime when comparing conditions", func() {
		futureTime := metav1.Time{Time: metav1.Now().Add(1000000)} // Far in the future
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "AllComponentsReady",
			Message:            "All components are ready",
			LastTransitionTime: futureTime, // Different from actual
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeTrue())
	})

	It("should match the second condition when multiple conditions exist", func() {
		condition := metav1.Condition{
			Type:    "Degraded",
			Status:  metav1.ConditionFalse,
			Reason:  "NoIssues",
			Message: "No degradation detected",
		}
		Expect(StatusHasCondition(testStatus, condition)).To(BeTrue())
	})

	It("should return false when status has no conditions", func() {
		emptyStatus := olsv1alpha1.OLSConfigStatus{
			Conditions: []metav1.Condition{},
		}
		condition := metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AllComponentsReady",
			Message: "All components are ready",
		}
		Expect(StatusHasCondition(emptyStatus, condition)).To(BeFalse())
	})

	It("should handle empty condition fields correctly", func() {
		statusWithEmptyFields := olsv1alpha1.OLSConfigStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "Empty",
					Status:  metav1.ConditionTrue,
					Reason:  "",
					Message: "",
				},
			},
		}
		condition := metav1.Condition{
			Type:    "Empty",
			Status:  metav1.ConditionTrue,
			Reason:  "",
			Message: "",
		}
		Expect(StatusHasCondition(statusWithEmptyFields, condition)).To(BeTrue())
	})
})

var _ = Describe("Utility Functions", func() {
	Describe("ProviderNameToEnvVarName", func() {
		It("should convert provider name to uppercase env var format", func() {
			result := ProviderNameToEnvVarName("my-provider")
			Expect(result).To(Equal("MY_PROVIDER"))
		})

		It("should handle multiple hyphens", func() {
			result := ProviderNameToEnvVarName("my-test-provider-name")
			Expect(result).To(Equal("MY_TEST_PROVIDER_NAME"))
		})

		It("should handle already uppercase names", func() {
			result := ProviderNameToEnvVarName("PROVIDER")
			Expect(result).To(Equal("PROVIDER"))
		})

		It("should handle names without hyphens", func() {
			result := ProviderNameToEnvVarName("provider")
			Expect(result).To(Equal("PROVIDER"))
		})

		It("should handle empty string", func() {
			result := ProviderNameToEnvVarName("")
			Expect(result).To(Equal(""))
		})

		It("should handle mixed case with hyphens", func() {
			result := ProviderNameToEnvVarName("OpenAI-Provider")
			Expect(result).To(Equal("OPENAI_PROVIDER"))
		})
	})

	Describe("SetDefaults_Deployment", func() {
		var deployment *appsv1.Deployment

		BeforeEach(func() {
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "app", Image: "myapp:v1"},
							},
						},
					},
				},
			}
		})

		It("should set default replicas to 1", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Replicas).NotTo(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should not override existing replicas", func() {
			replicas := int32(3)
			deployment.Spec.Replicas = &replicas

			SetDefaults_Deployment(deployment)

			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should set default strategy to RollingUpdate", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
		})

		It("should set default MaxUnavailable to 25%", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.RollingUpdate).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.String()).To(Equal("25%"))
		})

		It("should set default MaxSurge to 25%", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.RollingUpdate).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge.String()).To(Equal("25%"))
		})

		It("should set default RevisionHistoryLimit to 10", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.RevisionHistoryLimit).NotTo(BeNil())
			Expect(*deployment.Spec.RevisionHistoryLimit).To(Equal(int32(10)))
		})

		It("should set default ProgressDeadlineSeconds to 600", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.ProgressDeadlineSeconds).NotTo(BeNil())
			Expect(*deployment.Spec.ProgressDeadlineSeconds).To(Equal(int32(600)))
		})

		It("should not override existing strategy settings", func() {
			maxUnavailable := intstr.FromInt(1)
			maxSurge := intstr.FromInt(2)
			deployment.Spec.Strategy = appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			}

			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(1))
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(2))
		})

		It("should handle Recreate strategy type", func() {
			deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType

			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
			Expect(deployment.Spec.Strategy.RollingUpdate).To(BeNil())
		})
	})

	Describe("GeneratePostgresSelectorLabels", func() {
		It("should return correct Postgres selector labels", func() {
			labels := GeneratePostgresSelectorLabels()

			Expect(labels).To(HaveLen(4))
			Expect(labels["app.kubernetes.io/component"]).To(Equal("postgres-server"))
			Expect(labels["app.kubernetes.io/managed-by"]).To(Equal("lightspeed-operator"))
			Expect(labels["app.kubernetes.io/name"]).To(Equal("lightspeed-service-postgres"))
			Expect(labels["app.kubernetes.io/part-of"]).To(Equal("openshift-lightspeed"))
		})

		It("should return consistent labels across multiple calls", func() {
			labels1 := GeneratePostgresSelectorLabels()
			labels2 := GeneratePostgresSelectorLabels()

			Expect(labels1).To(Equal(labels2))
		})
	})

	Describe("GenerateAppServerSelectorLabels", func() {
		It("should return correct App Server selector labels", func() {
			labels := GenerateAppServerSelectorLabels()

			Expect(labels).To(HaveLen(4))
			Expect(labels["app.kubernetes.io/component"]).To(Equal("application-server"))
			Expect(labels["app.kubernetes.io/managed-by"]).To(Equal("lightspeed-operator"))
			Expect(labels["app.kubernetes.io/name"]).To(Equal("lightspeed-service-api"))
			Expect(labels["app.kubernetes.io/part-of"]).To(Equal("openshift-lightspeed"))
		})

		It("should return consistent labels across multiple calls", func() {
			labels1 := GenerateAppServerSelectorLabels()
			labels2 := GenerateAppServerSelectorLabels()

			Expect(labels1).To(Equal(labels2))
		})

		It("should return different labels than Postgres labels", func() {
			appLabels := GenerateAppServerSelectorLabels()
			pgLabels := GeneratePostgresSelectorLabels()

			Expect(appLabels["app.kubernetes.io/component"]).NotTo(Equal(pgLabels["app.kubernetes.io/component"]))
			Expect(appLabels["app.kubernetes.io/name"]).NotTo(Equal(pgLabels["app.kubernetes.io/name"]))
		})
	})

	Describe("GetPostgresCAConfigVolume", func() {
		It("should return a volume with correct structure", func() {
			volume := GetPostgresCAConfigVolume()

			Expect(volume.Name).To(Equal(PostgresCAVolume))
			Expect(volume.VolumeSource.ConfigMap).NotTo(BeNil())
			Expect(volume.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(OLSCAConfigMap))
			Expect(volume.VolumeSource.ConfigMap.DefaultMode).NotTo(BeNil())
			Expect(*volume.VolumeSource.ConfigMap.DefaultMode).To(Equal(VolumeDefaultMode))
		})

		It("should return consistent volume across multiple calls", func() {
			volume1 := GetPostgresCAConfigVolume()
			volume2 := GetPostgresCAConfigVolume()

			Expect(volume1.Name).To(Equal(volume2.Name))
			Expect(volume1.VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(volume2.VolumeSource.ConfigMap.LocalObjectReference.Name))
		})
	})

	Describe("GetPostgresCAVolumeMount", func() {
		It("should return a volume mount with correct structure", func() {
			mountPath := "/test/path/to/ca"
			volumeMount := GetPostgresCAVolumeMount(mountPath)

			Expect(volumeMount.Name).To(Equal(PostgresCAVolume))
			Expect(volumeMount.MountPath).To(Equal(mountPath))
			Expect(volumeMount.ReadOnly).To(BeTrue())
		})

		It("should use the provided mount path", func() {
			path1 := "/path/one"
			path2 := "/path/two"

			mount1 := GetPostgresCAVolumeMount(path1)
			mount2 := GetPostgresCAVolumeMount(path2)

			Expect(mount1.MountPath).To(Equal(path1))
			Expect(mount2.MountPath).To(Equal(path2))
			Expect(mount1.Name).To(Equal(mount2.Name)) // Same volume name
		})
	})

	Describe("AnnotateSecretWatcher", func() {
		It("should add watcher annotation to secret with nil annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations).NotTo(BeNil())
			Expect(annotations).To(HaveLen(1))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should add watcher annotation to secret with existing annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"existing-key": "existing-value",
					},
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations).To(HaveLen(2))
			Expect(annotations["existing-key"]).To(Equal("existing-value"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should overwrite existing watcher annotation", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						WatcherAnnotationKey: "old-value",
					},
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should preserve other annotations when adding watcher annotation", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"annotation1": "value1",
						"annotation2": "value2",
						"annotation3": "value3",
					},
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations).To(HaveLen(4))
			Expect(annotations["annotation1"]).To(Equal("value1"))
			Expect(annotations["annotation2"]).To(Equal("value2"))
			Expect(annotations["annotation3"]).To(Equal("value3"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})
	})

	Describe("AnnotateConfigMapWatcher", func() {
		It("should add watcher annotation to configmap with nil annotations", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations).NotTo(BeNil())
			Expect(annotations).To(HaveLen(1))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should add watcher annotation to configmap with existing annotations", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"existing-key": "existing-value",
					},
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations).To(HaveLen(2))
			Expect(annotations["existing-key"]).To(Equal("existing-value"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should overwrite existing watcher annotation", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						WatcherAnnotationKey: "old-value",
					},
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should preserve other annotations when adding watcher annotation", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"annotation1": "value1",
						"annotation2": "value2",
						"annotation3": "value3",
					},
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations).To(HaveLen(4))
			Expect(annotations["annotation1"]).To(Equal("value1"))
			Expect(annotations["annotation2"]).To(Equal("value2"))
			Expect(annotations["annotation3"]).To(Equal("value3"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})
	})

	Describe("GetProxyEnvVars", func() {
		var originalEnvVars map[string]string

		BeforeEach(func() {
			// Save original environment variables
			originalEnvVars = make(map[string]string)
			for _, envvar := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
				if value := os.Getenv(envvar); value != "" {
					originalEnvVars[envvar] = value
				}
			}

			// Clear all proxy environment variables for clean test state
			for _, envvar := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
				os.Unsetenv(envvar)
			}
		})

		AfterEach(func() {
			// Restore original environment variables
			for _, envvar := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
				os.Unsetenv(envvar)
			}
			for key, value := range originalEnvVars {
				os.Setenv(key, value)
			}
		})

		It("should return empty slice when no proxy env vars are set", func() {
			envVars := GetProxyEnvVars()

			Expect(envVars).To(BeEmpty())
		})

		It("should return HTTPS_PROXY as lowercase", func() {
			os.Setenv("HTTPS_PROXY", "https://proxy.example.com:8443")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("https_proxy"))
			Expect(envVars[0].Value).To(Equal("https://proxy.example.com:8443"))
		})

		It("should return https_proxy as lowercase", func() {
			os.Setenv("https_proxy", "https://proxy.example.com:8443")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("https_proxy"))
			Expect(envVars[0].Value).To(Equal("https://proxy.example.com:8443"))
		})

		It("should return HTTP_PROXY as lowercase", func() {
			os.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("http_proxy"))
			Expect(envVars[0].Value).To(Equal("http://proxy.example.com:8080"))
		})

		It("should return http_proxy as lowercase", func() {
			os.Setenv("http_proxy", "http://proxy.example.com:8080")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("http_proxy"))
			Expect(envVars[0].Value).To(Equal("http://proxy.example.com:8080"))
		})

		It("should return NO_PROXY as lowercase", func() {
			os.Setenv("NO_PROXY", "localhost,127.0.0.1,.example.com")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("no_proxy"))
			Expect(envVars[0].Value).To(Equal("localhost,127.0.0.1,.example.com"))
		})

		It("should return no_proxy as lowercase", func() {
			os.Setenv("no_proxy", "localhost,127.0.0.1,.example.com")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("no_proxy"))
			Expect(envVars[0].Value).To(Equal("localhost,127.0.0.1,.example.com"))
		})

		It("should return all proxy env vars when all are set", func() {
			os.Setenv("HTTPS_PROXY", "https://proxy.example.com:8443")
			os.Setenv("https_proxy", "https://proxy2.example.com:8443")
			os.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
			os.Setenv("http_proxy", "http://proxy2.example.com:8080")
			os.Setenv("NO_PROXY", "localhost,127.0.0.1")
			os.Setenv("no_proxy", "localhost,127.0.0.1,.example.com")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(6))

			// Verify all env vars are present with lowercase names
			envVarMap := make(map[string]string)
			for _, ev := range envVars {
				envVarMap[ev.Name] = ev.Value
			}

			Expect(envVarMap).To(HaveKey("https_proxy"))
			Expect(envVarMap).To(HaveKey("http_proxy"))
			Expect(envVarMap).To(HaveKey("no_proxy"))
		})

		It("should handle mixed case proxy values", func() {
			os.Setenv("HTTPS_PROXY", "https://PROXY.EXAMPLE.COM:8443")
			os.Setenv("HTTP_PROXY", "http://Proxy.Example.Com:8080")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(2))
			// Values should be preserved as-is (not lowercased)
			for _, ev := range envVars {
				if ev.Name == "https_proxy" {
					Expect(ev.Value).To(Equal("https://PROXY.EXAMPLE.COM:8443"))
				}
				if ev.Name == "http_proxy" {
					Expect(ev.Value).To(Equal("http://Proxy.Example.Com:8080"))
				}
			}
		})

		It("should ignore empty string values", func() {
			os.Setenv("HTTPS_PROXY", "")
			os.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
			os.Setenv("NO_PROXY", "")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("http_proxy"))
			Expect(envVars[0].Value).To(Equal("http://proxy.example.com:8080"))
		})

		It("should handle proxy URLs with authentication", func() {
			os.Setenv("HTTPS_PROXY", "https://user:password@proxy.example.com:8443")
			os.Setenv("HTTP_PROXY", "http://user:password@proxy.example.com:8080")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(2))
			for _, ev := range envVars {
				if ev.Name == "https_proxy" {
					Expect(ev.Value).To(Equal("https://user:password@proxy.example.com:8443"))
				}
				if ev.Name == "http_proxy" {
					Expect(ev.Value).To(Equal("http://user:password@proxy.example.com:8080"))
				}
			}
		})

		It("should handle complex NO_PROXY values", func() {
			os.Setenv("NO_PROXY", "localhost,127.0.0.1,10.0.0.0/8,.cluster.local,.svc,.example.com,192.168.1.0/24")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("no_proxy"))
			Expect(envVars[0].Value).To(Equal("localhost,127.0.0.1,10.0.0.0/8,.cluster.local,.svc,.example.com,192.168.1.0/24"))
		})

		It("should preserve order of environment variables", func() {
			// Set only specific vars to test order
			os.Setenv("HTTPS_PROXY", "https://proxy1.example.com:8443")
			os.Setenv("HTTP_PROXY", "http://proxy2.example.com:8080")
			os.Setenv("NO_PROXY", "localhost")

			envVars := GetProxyEnvVars()

			Expect(envVars).To(HaveLen(3))
			// Order should match the iteration order in the function:
			// HTTPS_PROXY, https_proxy, HTTP_PROXY, http_proxy, NO_PROXY, no_proxy
			Expect(envVars[0].Name).To(Equal("https_proxy"))
			Expect(envVars[1].Name).To(Equal("http_proxy"))
			Expect(envVars[2].Name).To(Equal("no_proxy"))
		})
	})

	Describe("ValidateCertificateFormat", func() {
		// Valid self-signed certificate for testing (generated with openssl)
		validCert := []byte(`-----BEGIN CERTIFICATE-----
MIIDizCCAnOgAwIBAgIUS1PdfaKIT3naQJBM5Yz4iEqEP3UwDQYJKoZIhvcNAQEL
BQAwVTELMAkGA1UEBhMCVVMxDTALBgNVBAgMBFRlc3QxDTALBgNVBAcMBFRlc3Qx
DTALBgNVBAoMBFRlc3QxGTAXBgNVBAMMEHRlc3QuZXhhbXBsZS5jb20wHhcNMjUx
MTEzMTUxMzMyWhcNMjYxMTEzMTUxMzMyWjBVMQswCQYDVQQGEwJVUzENMAsGA1UE
CAwEVGVzdDENMAsGA1UEBwwEVGVzdDENMAsGA1UECgwEVGVzdDEZMBcGA1UEAwwQ
dGVzdC5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AJc7RDRVjezikh83yIC/YUmrqDGU3fk4vKjQdK+AHi7Ini0IbaRMXdnlPegErMm7
JzBvrQkP6XkkXjabwQfevCdQtZ02t626NMmR5/QF80wDogCPUrYv9iD5U6wm3l3i
Dq8pUx4wZdrrrS4oN4p5f8kjWi1y8wJOKaidwI/HgGcyD9G1GV2Um3vkRZjgCdNx
xAvLXFNb7dGwmhP1DRwre88D8qozft6bGomU1cfPk79ry5p3rBZfvd+FOOoNco0v
THox2lihd5nRopnuTJ7x2TSotxGncXcOz7OjrB40Vrdf06+28CeSDlSsLujFfo7g
KIcROBT1OyNCB3V5mU1ZizsCAwEAAaNTMFEwHQYDVR0OBBYEFNjP/oYNRmP/+7Hp
ONAwlIA1bfRnMB8GA1UdIwQYMBaAFNjP/oYNRmP/+7HpONAwlIA1bfRnMA8GA1Ud
EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAAoftlL/rFw9W1B0f3WMrI5p
dl+ILT79s8H0gDCcMEe1/YhUjTWqThMXtXjz/isnTZbCUyaMeTir5tJoq/2MXLuj
OZFFxJxe3WtDF8XDysSO2vRCd2dhEV8oV9pO2+z9H+WR4mlKonEkzUYSyyjaO2DW
ov653fup9YH68vWdS5IhjLDknEbcFo7s5MvnZX8cbtZn4ZIoOSffPf6KhkkQYJ7h
UiTB+ZvDI7BO3bBErmNglQegjZkFshaNJ/uwRf1pcJWyPfMwlDQ391+YGbxPfYHo
cNHlzbRSivTDuHmXJdCYIdd8cnH6EbPm3zNg0jU5Au6OrvDZYifP+DtuiLmJct4=
-----END CERTIFICATE-----`) // notsecret

		It("should accept valid PEM certificate", func() {
			err := ValidateCertificateFormat(validCert)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject empty certificate", func() {
			err := ValidateCertificateFormat([]byte{})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("certificate is empty"))
		})

		It("should reject non-PEM data", func() {
			invalidPEM := []byte("This is not a PEM certificate")

			err := ValidateCertificateFormat(invalidPEM)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to decode PEM certificate"))
		})

		It("should reject PEM with wrong block type", func() {
			wrongBlockType := []byte(`-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDDVapWpmpWpmpW
pmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpWpmpW
-----END PRIVATE KEY-----`) // notsecret

			err := ValidateCertificateFormat(wrongBlockType)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("block type is not certificate"))
		})

		It("should reject malformed certificate data", func() {
			malformedCert := []byte(`-----BEGIN CERTIFICATE-----
This is not valid base64 encoded certificate data!
-----END CERTIFICATE-----`) // notsecret

			err := ValidateCertificateFormat(malformedCert)

			Expect(err).To(HaveOccurred())
			// PEM decoder fails before parsing, so we get "failed to decode PEM certificate"
			Expect(err.Error()).To(ContainSubstring("failed to decode PEM certificate"))
		})

		It("should reject certificate with invalid base64", func() {
			invalidBase64 := []byte(`-----BEGIN CERTIFICATE-----
!!!INVALID BASE64!!!
-----END CERTIFICATE-----`) // notsecret

			err := ValidateCertificateFormat(invalidBase64)

			Expect(err).To(HaveOccurred())
			// PEM decoder fails before parsing, so we get "failed to decode PEM certificate"
			Expect(err.Error()).To(ContainSubstring("failed to decode PEM certificate"))
		})

		It("should handle certificate with extra whitespace", func() {
			certWithWhitespace := []byte(`
-----BEGIN CERTIFICATE-----
MIIDizCCAnOgAwIBAgIUS1PdfaKIT3naQJBM5Yz4iEqEP3UwDQYJKoZIhvcNAQEL
BQAwVTELMAkGA1UEBhMCVVMxDTALBgNVBAgMBFRlc3QxDTALBgNVBAcMBFRlc3Qx
DTALBgNVBAoMBFRlc3QxGTAXBgNVBAMMEHRlc3QuZXhhbXBsZS5jb20wHhcNMjUx
MTEzMTUxMzMyWhcNMjYxMTEzMTUxMzMyWjBVMQswCQYDVQQGEwJVUzENMAsGA1UE
CAwEVGVzdDENMAsGA1UEBwwEVGVzdDENMAsGA1UECgwEVGVzdDEZMBcGA1UEAwwQ
dGVzdC5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AJc7RDRVjezikh83yIC/YUmrqDGU3fk4vKjQdK+AHi7Ini0IbaRMXdnlPegErMm7
JzBvrQkP6XkkXjabwQfevCdQtZ02t626NMmR5/QF80wDogCPUrYv9iD5U6wm3l3i
Dq8pUx4wZdrrrS4oN4p5f8kjWi1y8wJOKaidwI/HgGcyD9G1GV2Um3vkRZjgCdNx
xAvLXFNb7dGwmhP1DRwre88D8qozft6bGomU1cfPk79ry5p3rBZfvd+FOOoNco0v
THox2lihd5nRopnuTJ7x2TSotxGncXcOz7OjrB40Vrdf06+28CeSDlSsLujFfo7g
KIcROBT1OyNCB3V5mU1ZizsCAwEAAaNTMFEwHQYDVR0OBBYEFNjP/oYNRmP/+7Hp
ONAwlIA1bfRnMB8GA1UdIwQYMBaAFNjP/oYNRmP/+7HpONAwlIA1bfRnMA8GA1Ud
EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAAoftlL/rFw9W1B0f3WMrI5p
dl+ILT79s8H0gDCcMEe1/YhUjTWqThMXtXjz/isnTZbCUyaMeTir5tJoq/2MXLuj
OZFFxJxe3WtDF8XDysSO2vRCd2dhEV8oV9pO2+z9H+WR4mlKonEkzUYSyyjaO2DW
ov653fup9YH68vWdS5IhjLDknEbcFo7s5MvnZX8cbtZn4ZIoOSffPf6KhkkQYJ7h
UiTB+ZvDI7BO3bBErmNglQegjZkFshaNJ/uwRf1pcJWyPfMwlDQ391+YGbxPfYHo
cNHlzbRSivTDuHmXJdCYIdd8cnH6EbPm3zNg0jU5Au6OrvDZYifP+DtuiLmJct4=
-----END CERTIFICATE-----
`) // notsecret

			err := ValidateCertificateFormat(certWithWhitespace)

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ValidateLLMCredentials", func() {
		var testReconciler *TestReconciler
		var testSecret *corev1.Secret

		BeforeEach(func() {
			testReconciler = NewTestReconciler(
				k8sClient,
				logf.Log.WithName("test"),
				k8sClient.Scheme(),
				OLSNamespaceDefault,
			)
		})

		AfterEach(func() {
			if testSecret != nil {
				_ = k8sClient.Delete(testCtx, testSecret)
				testSecret = nil
			}
		})

		It("should validate LLM credentials exist", func() {
			By("Create a test secret with apitoken")
			testSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-secret",
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string][]byte{
					"apitoken": []byte("test-token"),
				},
			}
			err := k8sClient.Create(testCtx, testSecret)
			Expect(err).NotTo(HaveOccurred())

			By("Create a test CR with LLM provider")
			testCR := GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "test-llm-secret"

			By("Check LLM credentials - should succeed")
			err = ValidateLLMCredentials(testReconciler, testCtx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when LLM credentials secret is missing", func() {
			By("Create a test CR with non-existent secret")
			testCR := GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "non-existent-secret"

			By("Check LLM credentials - should fail")
			err := ValidateLLMCredentials(testReconciler, testCtx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credential secret non-existent-secret not found"))
		})

		It("should fail when provider is missing credentials secret name", func() {
			By("Create a test CR with empty secret name")
			testCR := GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = ""

			By("Check LLM credentials - should fail")
			err := ValidateLLMCredentials(testReconciler, testCtx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing credentials secret"))
		})

		It("should fail when secret is missing apitoken key", func() {
			By("Create a test secret without apitoken")
			testSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-secret-no-token",
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string][]byte{
					"wrongkey": []byte("test-value"),
				},
			}
			err := k8sClient.Create(testCtx, testSecret)
			Expect(err).NotTo(HaveOccurred())

			By("Create a test CR with the secret")
			testCR := GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "test-llm-secret-no-token"

			By("Check LLM credentials - should fail")
			err = ValidateLLMCredentials(testReconciler, testCtx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing key 'apitoken'"))
		})

		It("should accept Azure OpenAI secret with client credentials", func() {
			By("Create a test secret with Azure client credentials")
			testSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-azure-secret",
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string][]byte{
					"client_id":     []byte("test-client-id"),
					"tenant_id":     []byte("test-tenant-id"),
					"client_secret": []byte("test-client-secret"),
				},
			}
			err := k8sClient.Create(testCtx, testSecret)
			Expect(err).NotTo(HaveOccurred())

			By("Create a test CR with Azure OpenAI provider")
			testCR := GetDefaultOLSConfigCR()
			testCR.Spec.LLMConfig.Providers[0].Type = AzureOpenAIType
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "test-azure-secret"

			By("Check LLM credentials - should succeed")
			err = ValidateLLMCredentials(testReconciler, testCtx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("ImageStreamNameFor", func() {
	It("converts a typical image reference to a lowercase slug with RFC1123-safe chars and a 6-char suffix", func() {
		image := "quay.io/org/my-image:v1.0"
		got := ImageStreamNameFor(image)
		Expect(got).To(MatchRegexp(`^[a-z0-9_-]+-[a-f0-9]{6}$`))
		Expect(len(got)).To(BeNumerically("<=", imageStreamNameMaxLength))
		Expect(got[:len(got)-7]).To(Equal("quay-io-org-my-image-v1-0"))
	})

	It("is deterministic: same image produces same name", func() {
		image := "registry.example.com/ns/foo:tag"
		Expect(ImageStreamNameFor(image)).To(Equal(ImageStreamNameFor(image)))
	})

	It("produces different suffixes for different images", func() {
		a := ImageStreamNameFor("quay.io/a:1")
		b := ImageStreamNameFor("quay.io/b:2")
		Expect(a).NotTo(Equal(b))
	})

	It("lowercases the slug", func() {
		got := ImageStreamNameFor("Quay.IO/ORG/My-Image:V1")
		Expect(got).To(MatchRegexp(`^[a-z0-9_-]+-[a-f0-9]{6}$`))
		Expect(got).To(ContainSubstring("quay-io-org-my-image-v1"))
	})

	It("replaces slash, colon, and at-sign with hyphens in the slug (via normalize-then-replace)", func() {
		got := ImageStreamNameFor("reg.io/ns/img:v1")
		Expect(got).To(ContainSubstring("reg-io-ns-img-v1"))
		got2 := ImageStreamNameFor("reg.io/ns/img@sha256:abc123")
		Expect(got2).To(ContainSubstring("reg-io-ns-img-sha256-abc123"))
	})

	It("truncates long image names to ImageStreamSlugMaxLength in the slug and keeps total length <= ImageStreamNameMaxLength", func() {
		longImage := "quay.io/" + strings.Repeat("a", 200) + ":tag"
		got := ImageStreamNameFor(longImage)
		slugPart := strings.TrimSuffix(got, "-"+got[strings.LastIndex(got, "-")+1:])
		Expect(len(slugPart)).To(BeNumerically("<=", imageStreamSlugMaxLength))
		Expect(len(got)).To(BeNumerically("<=", imageStreamNameMaxLength))
	})

	It("replaces non-RFC1123 characters in the slug with hyphens", func() {
		got := ImageStreamNameFor("reg.io/some.image_name:tag")
		Expect(got).To(MatchRegexp(`^[a-z0-9_-]+-[a-f0-9]{6}$`))
	})
})

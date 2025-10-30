package utils

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("Hash Functions", func() {
	Describe("HashBytes", func() {
		It("should generate consistent hash for same input", func() {
			input := []byte("test-data")
			hash1, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash1).NotTo(BeEmpty())

			hash2, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash2).To(Equal(hash1))
		})

		It("should generate different hashes for different inputs", func() {
			hash1, err := HashBytes([]byte("data1"))
			Expect(err).NotTo(HaveOccurred())

			hash2, err := HashBytes([]byte("data2"))
			Expect(err).NotTo(HaveOccurred())

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should generate SHA256 hash with correct length", func() {
			input := []byte("test")
			hash, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())
			// SHA256 produces 64 hex characters
			Expect(len(hash)).To(Equal(64))
		})

		It("should handle empty input", func() {
			hash, err := HashBytes([]byte(""))
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).NotTo(BeEmpty())
			// Empty string should still produce a valid SHA256 hash
			Expect(len(hash)).To(Equal(64))
		})
	})
})

var _ = Describe("Secret Functions", func() {
	var testClient client.Client
	var testSecret *corev1.Secret
	var ctx context.Context

	BeforeEach(func() {
		testClient = k8sClient
		ctx = context.Background() // Use Background context for K8s operations
		testSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-utils",
				Namespace: OLSNamespaceDefault,
			},
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
				"apitoken": []byte("token456"),
			},
		}
		Expect(testClient.Create(ctx, testSecret)).To(Succeed())
	})

	AfterEach(func() {
		_ = testClient.Delete(ctx, testSecret)
	})

	Describe("GetSecretContent", func() {
		It("should retrieve specified fields from secret", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{"username", "password"}

			result, err := GetSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result["username"]).To(Equal("admin"))
			Expect(result["password"]).To(Equal("secret123"))
		})

		It("should return error for non-existent secret", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{"username"}

			_, err := GetSecretContent(testClient, "non-existent", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret not found"))
		})

		It("should return error for missing field", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{"missing-field"}

			_, err := GetSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not present in the secret"))
		})

		It("should handle empty field list", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{}

			result, err := GetSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Describe("GetAllSecretContent", func() {
		It("should retrieve all fields from secret", func() {
			foundSecret := &corev1.Secret{}

			result, err := GetAllSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(3))
			Expect(result["username"]).To(Equal("admin"))
			Expect(result["password"]).To(Equal("secret123"))
			Expect(result["apitoken"]).To(Equal("token456"))
		})

		It("should return error for non-existent secret", func() {
			foundSecret := &corev1.Secret{}

			_, err := GetAllSecretContent(testClient, "non-existent", OLSNamespaceDefault, foundSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret not found"))
		})

		It("should handle empty secret", func() {
			emptySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-secret",
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string][]byte{},
			}
			Expect(testClient.Create(ctx, emptySecret)).To(Succeed())
			defer testClient.Delete(ctx, emptySecret)

			foundSecret := &corev1.Secret{}
			result, err := GetAllSecretContent(testClient, "empty-secret", OLSNamespaceDefault, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})
})

var _ = Describe("Volume Comparison", func() {
	Describe("PodVolumeEqual", func() {
		It("should return true for identical volumes", func() {
			volumes := []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
						},
					},
				},
			}
			Expect(PodVolumeEqual(volumes, volumes)).To(BeTrue())
		})

		It("should return false for different length", func() {
			volumes1 := []corev1.Volume{
				{Name: "vol1"},
			}
			volumes2 := []corev1.Volume{
				{Name: "vol1"},
				{Name: "vol2"},
			}
			Expect(PodVolumeEqual(volumes1, volumes2)).To(BeFalse())
		})

		It("should compare secret volumes correctly", func() {
			volumes1 := []corev1.Volume{
				{
					Name: "secret-vol",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "my-secret",
						},
					},
				},
			}
			volumes2 := []corev1.Volume{
				{
					Name: "secret-vol",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "different-secret",
						},
					},
				},
			}
			Expect(PodVolumeEqual(volumes1, volumes2)).To(BeFalse())
		})

		It("should compare configmap volumes correctly", func() {
			volumes1 := []corev1.Volume{
				{
					Name: "cm-vol",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "cm1"},
						},
					},
				},
			}
			volumes2 := volumes1
			Expect(PodVolumeEqual(volumes1, volumes2)).To(BeTrue())
		})

		It("should compare emptyDir volumes correctly", func() {
			volumes := []corev1.Volume{
				{
					Name: "empty",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			}
			Expect(PodVolumeEqual(volumes, volumes)).To(BeTrue())
		})

		It("should handle empty volume lists", func() {
			Expect(PodVolumeEqual([]corev1.Volume{}, []corev1.Volume{})).To(BeTrue())
		})
	})
})

var _ = Describe("Container Comparison", func() {
	Describe("ContainersEqual", func() {
		It("should return true for identical containers", func() {
			containers := []corev1.Container{
				{
					Name:  "app",
					Image: "myapp:v1",
				},
			}
			Expect(ContainersEqual(containers, containers)).To(BeTrue())
		})

		It("should return false for different length", func() {
			containers1 := []corev1.Container{{Name: "app1"}}
			containers2 := []corev1.Container{{Name: "app1"}, {Name: "app2"}}
			Expect(ContainersEqual(containers1, containers2)).To(BeFalse())
		})

		It("should return false for different images", func() {
			containers1 := []corev1.Container{
				{Name: "app", Image: "myapp:v1"},
			}
			containers2 := []corev1.Container{
				{Name: "app", Image: "myapp:v2"},
			}
			Expect(ContainersEqual(containers1, containers2)).To(BeFalse())
		})

		It("should handle empty container lists", func() {
			Expect(ContainersEqual([]corev1.Container{}, []corev1.Container{})).To(BeTrue())
		})
	})
})

var _ = Describe("Probe Equality", func() {
	It("should return true when probes are equal", func() {
		var sixty int64 = int64(60)
		probe1 := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/liveness",
					Port:   intstr.FromString("https"),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds:           60,
			PeriodSeconds:                 10,
			TimeoutSeconds:                1,
			FailureThreshold:              3,
			SuccessThreshold:              1,
			TerminationGracePeriodSeconds: &sixty,
		}
		probe2 := probe1.DeepCopy()
		Expect(ProbeEqual(probe1, probe2)).To(BeTrue())
	})
	It("should return false when probes are not equal", func() {
		probe1 := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/liveness",
					Port:   intstr.FromString("https"),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       10,
			TimeoutSeconds:      1,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}
		probe2 := probe1.DeepCopy()
		probe2.InitialDelaySeconds = probe2.InitialDelaySeconds + 1
		Expect(ProbeEqual(probe1, probe2)).To(BeFalse())
	})
	It("should ignore empty values when comparing partial defined probes with complete probes", func() {
		partialDefined := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/liveness",
					Port:   intstr.FromString("https"),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       10,
		}
		defaultFilled := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/liveness",
					Port:   intstr.FromString("https"),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       10,
			TimeoutSeconds:      1,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}
		Expect(ProbeEqual(partialDefined, defaultFilled)).To(BeTrue())
	})

})

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

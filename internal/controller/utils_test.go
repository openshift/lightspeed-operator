package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"

	// . "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
		Expect(probeEqual(probe1, probe2)).To(BeTrue())
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
		Expect(probeEqual(probe1, probe2)).To(BeFalse())
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
		Expect(probeEqual(partialDefined, defaultFilled)).To(BeTrue())
	})

})

var _ = Describe("TLS Security Profile", func() {
	Context("Get Profile Spec", Ordered, func() {
		apiServer := &configv1.APIServer{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
			Spec: configv1.APIServerSpec{
				TLSSecurityProfile: &configv1.TLSSecurityProfile{
					Type:         configv1.TLSProfileIntermediateType,
					Intermediate: &configv1.IntermediateTLSProfile{},
				},
			},
		}
		BeforeAll(func() {
			By("Create the APIServer object")
			err := k8sClient.Create(context.TODO(), apiServer)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterAll(func() {
			By("Delete the APIServer object")
			err := k8sClient.Delete(context.TODO(), apiServer)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return the default TLS Security Profile for API Endpoint when none are defined", func() {
			profileSpec, err := getTlsSecurityProfileSpec(nil, k8sClient)
			Expect(err).To(BeNil())
			Expect(profileSpec).To(Equal(*configv1.TLSProfiles[configv1.TLSProfileIntermediateType]))
		})

		It("should return the custom TLS Security Profile for API Endpoint when defined", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"ECDHE-ECDSA-CHACHA20-POLY1305", "ECDHE-RSA-CHACHA20-POLY1305"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}
			profileSpec, err := getTlsSecurityProfileSpec(profile, k8sClient)
			Expect(err).To(BeNil())
			Expect(profileSpec).To(Equal(profile.Custom.TLSProfileSpec))
		})

	})

})

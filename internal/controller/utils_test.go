package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// . "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
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

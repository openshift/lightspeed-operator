package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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

var _ = Describe("Resource Comparison Functions", func() {
	Describe("DeploymentSpecEqual", func() {
		var deployment1, deployment2 *appsv1.DeploymentSpec

		BeforeEach(func() {
			replicas := int32(2)
			deployment1 = &appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
				},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						NodeSelector: map[string]string{"disktype": "ssd"},
						Tolerations: []corev1.Toleration{
							{Key: "key1", Operator: corev1.TolerationOpEqual, Value: "value1"},
						},
						Containers: []corev1.Container{
							{Name: "app", Image: "myapp:v1"},
						},
						Volumes: []corev1.Volume{
							{Name: "vol1", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						},
					},
				},
			}
			deployment2 = deployment1.DeepCopy()
		})

		It("should return true for identical deployment specs", func() {
			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeTrue())
		})

		It("should return false when replicas differ", func() {
			newReplicas := int32(3)
			deployment2.Replicas = &newReplicas

			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeFalse())
		})

		It("should return false when node selector differs", func() {
			deployment2.Template.Spec.NodeSelector = map[string]string{"disktype": "hdd"}

			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeFalse())
		})

		It("should return false when tolerations differ", func() {
			deployment2.Template.Spec.Tolerations = []corev1.Toleration{}

			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeFalse())
		})

		It("should return false when strategy differs", func() {
			deployment2.Strategy.Type = appsv1.RecreateDeploymentStrategyType

			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeFalse())
		})

		It("should return false when volumes differ", func() {
			deployment2.Template.Spec.Volumes = []corev1.Volume{}

			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeFalse())
		})

		It("should return false when containers differ", func() {
			deployment2.Template.Spec.Containers[0].Image = "myapp:v2"

			Expect(DeploymentSpecEqual(deployment1, deployment2, true)).To(BeFalse())
		})
	})

	Describe("ServiceEqual", func() {
		var service1, service2 *corev1.Service

		BeforeEach(func() {
			service1 = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-service",
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
					Ports: []corev1.ServicePort{
						{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
					},
				},
			}
			service2 = service1.DeepCopy()
		})

		It("should return true for identical services", func() {
			Expect(ServiceEqual(service1, service2)).To(BeTrue())
		})

		It("should return false when labels differ", func() {
			service2.Labels = map[string]string{"app": "different"}

			Expect(ServiceEqual(service1, service2)).To(BeFalse())
		})

		It("should return false when selector differs", func() {
			service2.Spec.Selector = map[string]string{"app": "different"}

			Expect(ServiceEqual(service1, service2)).To(BeFalse())
		})

		It("should return false when port count differs", func() {
			service2.Spec.Ports = append(service2.Spec.Ports, corev1.ServicePort{
				Name: "https", Port: 443,
			})

			Expect(ServiceEqual(service1, service2)).To(BeFalse())
		})

		It("should return false when port details differ", func() {
			service2.Spec.Ports[0].Port = 8080

			Expect(ServiceEqual(service1, service2)).To(BeFalse())
		})

		It("should handle empty labels", func() {
			service1.Labels = map[string]string{}
			service2.Labels = map[string]string{}

			Expect(ServiceEqual(service1, service2)).To(BeTrue())
		})
	})

	Describe("ServiceMonitorEqual", func() {
		var sm1, sm2 *monv1.ServiceMonitor

		BeforeEach(func() {
			sm1 = &monv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-monitor",
					Labels: map[string]string{"monitoring": "true"},
				},
				Spec: monv1.ServiceMonitorSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Endpoints: []monv1.Endpoint{
						{Port: "metrics", Interval: monv1.Duration("30s")},
					},
				},
			}
			sm2 = sm1.DeepCopy()
		})

		It("should return true for identical service monitors", func() {
			Expect(ServiceMonitorEqual(sm1, sm2)).To(BeTrue())
		})

		It("should return false when labels differ", func() {
			sm2.Labels = map[string]string{"monitoring": "false"}

			Expect(ServiceMonitorEqual(sm1, sm2)).To(BeFalse())
		})

		It("should return false when spec differs", func() {
			sm2.Spec.Endpoints[0].Interval = monv1.Duration("60s")

			Expect(ServiceMonitorEqual(sm1, sm2)).To(BeFalse())
		})
	})

	Describe("PrometheusRuleEqual", func() {
		var rule1, rule2 *monv1.PrometheusRule

		BeforeEach(func() {
			interval := monv1.Duration("30s")
			rule1 = &monv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-rule",
					Labels: map[string]string{"prometheus": "true"},
				},
				Spec: monv1.PrometheusRuleSpec{
					Groups: []monv1.RuleGroup{
						{
							Name:     "test-group",
							Interval: &interval,
						},
					},
				},
			}
			rule2 = rule1.DeepCopy()
		})

		It("should return true for identical prometheus rules", func() {
			Expect(PrometheusRuleEqual(rule1, rule2)).To(BeTrue())
		})

		It("should return false when labels differ", func() {
			rule2.Labels = map[string]string{"prometheus": "false"}

			Expect(PrometheusRuleEqual(rule1, rule2)).To(BeFalse())
		})

		It("should return false when spec differs", func() {
			newInterval := monv1.Duration("60s")
			rule2.Spec.Groups[0].Interval = &newInterval

			Expect(PrometheusRuleEqual(rule1, rule2)).To(BeFalse())
		})
	})

	Describe("NetworkPolicyEqual", func() {
		var np1, np2 *networkingv1.NetworkPolicy

		BeforeEach(func() {
			np1 = &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-policy",
					Labels: map[string]string{"policy": "test"},
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				},
			}
			np2 = np1.DeepCopy()
		})

		It("should return true for identical network policies", func() {
			Expect(NetworkPolicyEqual(np1, np2)).To(BeTrue())
		})

		It("should return false when labels differ", func() {
			np2.Labels = map[string]string{"policy": "different"}

			Expect(NetworkPolicyEqual(np1, np2)).To(BeFalse())
		})

		It("should return false when spec differs", func() {
			np2.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}

			Expect(NetworkPolicyEqual(np1, np2)).To(BeFalse())
		})
	})
})

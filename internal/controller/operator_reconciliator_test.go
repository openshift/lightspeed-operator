package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// . "github.com/onsi/gomega/gstruct"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("App server assets", func() {
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions
	var operatorDeployment *appsv1.Deployment

	Context("Operator Service Monitor", func() {
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

			operatorDeployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lightspeed-operator-controller-manager",
					Namespace: r.Options.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"control-plane": "controller-manager",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"control-plane": "controller-manager",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "manager",
									Image: "lightspeed-operator:latest",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), operatorDeployment)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			err := k8sClient.Delete(context.Background(), operatorDeployment)
			Expect(err).To(BeNil())
		})

		It("should generate operator service monitor in operator's namespace", func() {

			err := r.reconcileServiceMonitorForOperator(context.Background())
			Expect(err).To(BeNil())

			sm := &monv1.ServiceMonitor{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: OperatorServiceMonitorName, Namespace: r.Options.Namespace}, sm)
			Expect(err).To(BeNil())

			valFalse := false
			serverName := strings.Join([]string{"lightspeed-operator-controller-manager-service", OLSNamespaceDefault, "svc"}, ".")

			expectedSM := monv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      OperatorServiceMonitorName,
					Namespace: r.Options.Namespace,
					Labels: map[string]string{
						"control-plane":                              "controller-manager",
						"app.kubernetes.io/component":                "metrics",
						"app.kubernetes.io/managed-by":               "lightspeed-operator",
						"app.kubernetes.io/name":                     "servicemonitor",
						"app.kubernetes.io/instance":                 "controller-manager-metrics-monitor",
						"app.kubernetes.io/part-of":                  "lightspeed-operator",
						"monitoring.openshift.io/collection-profile": "full",
						"openshift.io/user-monitoring":               "false",
					},
				},
				Spec: monv1.ServiceMonitorSpec{
					Endpoints: []monv1.Endpoint{
						{
							Port:     "metrics",
							Path:     "/metrics",
							Interval: "30s",
							Scheme:   "https",
							TLSConfig: &monv1.TLSConfig{
								CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
								CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
								KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
								SafeTLSConfig: monv1.SafeTLSConfig{
									InsecureSkipVerify: &valFalse,
									ServerName:         &serverName,
								},
							},
						},
					},
					JobLabel: "app.kubernetes.io/name",
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"control-plane": "controller-manager",
						},
					},
				},
			}
			Expect(sm.ObjectMeta.Name).To(Equal(OperatorServiceMonitorName))
			Expect(sm.ObjectMeta.Namespace).To(Equal(r.Options.Namespace))
			Expect(sm.ObjectMeta.Labels).To(Equal(expectedSM.ObjectMeta.Labels))
			Expect(sm.Spec.Endpoints).To(ConsistOf(expectedSM.Spec.Endpoints))
			Expect(sm.Spec.JobLabel).To(Equal(expectedSM.Spec.JobLabel))
			Expect(sm.Spec.Selector.MatchLabels).To(Equal(expectedSM.Spec.Selector.MatchLabels))
			Expect(sm.ObjectMeta.OwnerReferences).To(HaveLen(1))
			Expect(sm.ObjectMeta.OwnerReferences[0].Name).To(Equal(operatorDeployment.Name))
		})

	})

	Context("Operator Network Policy", func() {
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

		AfterEach(func() {
		})

		It("should generate operator network policy in operator's namespace", func() {
			err := r.reconcileNetworkPolicyForOperator(context.Background())
			Expect(err).To(BeNil())
			np := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: OperatorNetworkPolicyName, Namespace: r.Options.Namespace}, np)
			Expect(err).To(BeNil())
			Expect(np.ObjectMeta.Name).To(Equal(OperatorNetworkPolicyName))
			Expect(np.ObjectMeta.Namespace).To(Equal(r.Options.Namespace))
			Expect(np.Spec.PodSelector.MatchLabels).To(Equal(map[string]string{"control-plane": "controller-manager"}))
			Expect(np.Spec.PolicyTypes).To(ConsistOf([]networkingv1.PolicyType{"Ingress"}))
			Expect(np.Spec.Ingress).To(HaveLen(2))
			Expect(np.Spec.Ingress).To(ConsistOf(
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-monitoring",
								},
							},
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
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(OperatorMetricsPort)}[0],
						},
					},
				},
				networkingv1.NetworkPolicyIngressRule{
					// Allow ingress from API server namespaces for webhook validation
					From: []networkingv1.NetworkPolicyPeer{
						{
							// Allow traffic from host network (where API server pods run in OpenShift)
							// refer to https://access.redhat.com/solutions/7008681
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"policy-group.network.openshift.io/host-network": "",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromString("webhook")}[0],
						},
					},
				},
			))
		})

	})
})

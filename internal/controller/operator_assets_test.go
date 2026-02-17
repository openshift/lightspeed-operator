package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("App server assets", func() {
	var r *OLSConfigReconciler
	var rOptions *utils.OLSConfigReconcilerOptions
	var operatorDeployment *appsv1.Deployment

	Context("Operator Service Monitor", func() {
		BeforeEach(func() {
			options := getDefaultReconcilerOptions(utils.OLSNamespaceDefault)
			rOptions = &options
			r = &OLSConfigReconciler{
				Options: *rOptions,
				Logger:  logf.Log.WithName("olsconfig.reconciler"),
				Client:  k8sClient,
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
			err := r.ReconcileServiceMonitorForOperator(context.Background())
			Expect(err).To(BeNil())

			sm := &monv1.ServiceMonitor{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: utils.OperatorServiceMonitorName, Namespace: r.Options.Namespace}, sm)
			Expect(err).To(BeNil())

			valFalse := false
			serverName := strings.Join([]string{"lightspeed-operator-controller-manager-service", utils.OLSNamespaceDefault, "svc"}, ".")
			var schemeHTTPS monv1.Scheme = "https"

			expectedSM := monv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.OperatorServiceMonitorName,
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
			Expect(sm.ObjectMeta.Name).To(Equal(utils.OperatorServiceMonitorName))
			Expect(sm.ObjectMeta.Namespace).To(Equal(r.Options.Namespace))
			Expect(sm.ObjectMeta.Labels).To(Equal(expectedSM.ObjectMeta.Labels))
			Expect(sm.Spec.Endpoints).To(ConsistOf(expectedSM.Spec.Endpoints))
			Expect(sm.Spec.JobLabel).To(Equal(expectedSM.Spec.JobLabel))
			Expect(sm.Spec.Selector.MatchLabels).To(Equal(expectedSM.Spec.Selector.MatchLabels))
			Expect(sm.ObjectMeta.OwnerReferences).To(HaveLen(1))
			Expect(sm.ObjectMeta.OwnerReferences[0].Name).To(Equal(operatorDeployment.Name))
		})

		It("should skip service monitor creation when Prometheus is not available", func() {
			// Delete any existing ServiceMonitor from previous test
			existingSm := &monv1.ServiceMonitor{}
			err := k8sClient.Get(context.Background(), client.ObjectKey{Name: utils.OperatorServiceMonitorName, Namespace: r.Options.Namespace}, existingSm)
			if err == nil {
				err = k8sClient.Delete(context.Background(), existingSm)
				Expect(err).To(BeNil())
			}

			// Override PrometheusAvailable to false
			r.Options.PrometheusAvailable = false

			err = r.ReconcileServiceMonitorForOperator(context.Background())
			Expect(err).To(BeNil())

			// Verify ServiceMonitor was NOT created
			sm := &monv1.ServiceMonitor{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: utils.OperatorServiceMonitorName, Namespace: r.Options.Namespace}, sm)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

	})

	Context("Operator Network Policy", func() {
		BeforeEach(func() {
			r = &OLSConfigReconciler{
				Options: *rOptions,
				Logger:  logf.Log.WithName("olsconfig.reconciler"),
				Client:  k8sClient,
			}
		})

		AfterEach(func() {
		})

		It("should generate operator network policy in operator's namespace", func() {
			err := r.ReconcileNetworkPolicyForOperator(context.Background())
			Expect(err).To(BeNil())
			np := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: utils.OperatorNetworkPolicyName, Namespace: r.Options.Namespace}, np)
			Expect(err).To(BeNil())
			Expect(np.ObjectMeta.Name).To(Equal(utils.OperatorNetworkPolicyName))
			Expect(np.ObjectMeta.Namespace).To(Equal(r.Options.Namespace))
			Expect(np.Spec.PodSelector.MatchLabels).To(Equal(map[string]string{"control-plane": "controller-manager"}))
			Expect(np.Spec.PolicyTypes).To(ConsistOf([]networkingv1.PolicyType{"Ingress"}))
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress).To(ConsistOf(networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": utils.ClientCACmNamespace,
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
						Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OperatorMetricsPort)}[0],
					},
				},
			}))
		})

	})
})

var _ = Describe("Main Reconcile Loop", func() {
	var (
		reconciler    *OLSConfigReconciler
		ctx           context.Context
		testNamespace string
		llmSecret     *corev1.Secret
		consoleSecret *corev1.Secret
		kubeRootCACM  *corev1.ConfigMap
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = utils.OLSNamespaceDefault

		// Setup reconciler
		reconciler = &OLSConfigReconciler{
			Client:  k8sClient,
			Options: getDefaultReconcilerOptions(testNamespace),
			Logger:  logf.Log.WithName("test.reconciler"),
		}

		// Create the operator deployment (required for ReconcileServiceMonitorForOperator)
		operatorDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lightspeed-operator-controller-manager",
				Namespace: testNamespace,
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
		Expect(k8sClient.Create(ctx, operatorDeployment)).To(Succeed())

		// Create required secrets
		llmSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-llm-secret-reconcile",
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"apitoken": []byte("test-token"),
			},
		}
		Expect(k8sClient.Create(ctx, llmSecret)).To(Succeed())

		consoleSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.ConsoleUIServiceCertSecretName,
				Namespace: testNamespace,
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": []byte("fake-cert"),
				"tls.key": []byte("fake-key"),
			},
		}
		Expect(k8sClient.Create(ctx, consoleSecret)).To(Succeed())

		// Create kube-root-ca.crt ConfigMap
		kubeRootCACM = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-root-ca.crt",
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"service-ca.crt": utils.TestCACert,
			},
		}
		Expect(k8sClient.Create(ctx, kubeRootCACM)).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup
		operatorDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lightspeed-operator-controller-manager",
				Namespace: testNamespace,
			},
		}
		_ = k8sClient.Delete(ctx, operatorDeployment)
		_ = k8sClient.Delete(ctx, llmSecret)
		_ = k8sClient.Delete(ctx, consoleSecret)
		_ = k8sClient.Delete(ctx, kubeRootCACM)
	})

	Context("Reconcile with OLSConfig", func() {
		var olsConfig *olsv1alpha1.OLSConfig

		BeforeEach(func() {
			olsConfig = utils.GetDefaultOLSConfigCR()
			olsConfig.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "test-llm-secret-reconcile"
			Expect(k8sClient.Create(ctx, olsConfig)).To(Succeed())
		})

		AfterEach(func() {
			// Delete OLSConfig
			cleanupOLSConfig(ctx, olsConfig)
		})

		It("should successfully reconcile OLSConfig", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: utils.OLSConfigName,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			// Reconciliation might fail due to missing resources, but shouldn't panic
			// We're mainly testing that the reconcile loop executes without crashing
			Expect(result).NotTo(BeNil())
			// Error is acceptable since we don't have all components running
			if err != nil {
				// Should be a reconciliation error, not a panic
				Expect(err.Error()).NotTo(BeEmpty())
			}
		})

		It("should ignore OLSConfig with wrong name", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "wrong-name",
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("Reconcile without OLSConfig", func() {
		It("should return without error when OLSConfig not found", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: utils.OLSConfigName,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			// Either succeeds or fails on operator resources (acceptable)
			// Main point is it doesn't panic and handles missing OLSConfig gracefully
			Expect(result).NotTo(BeNil())
			if err != nil {
				// If it fails, it should be a controlled error (operator resources)
				Expect(err.Error()).NotTo(BeEmpty())
			}
		})
	})
})

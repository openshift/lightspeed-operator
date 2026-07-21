package utils

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ReconcileServiceMonitor", func() {
	const smName = "test-reconcile-service-monitor"

	var (
		testReconciler *TestReconciler
		ctx            context.Context
		schemeHTTPS    monv1.Scheme
	)

	desiredSM := func() *monv1.ServiceMonitor {
		return &monv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      smName,
				Namespace: OLSNamespaceDefault,
				Labels: map[string]string{
					"app.kubernetes.io/name": smName,
					"test-label":             "v1",
				},
			},
			Spec: monv1.ServiceMonitorSpec{
				Endpoints: []monv1.Endpoint{
					{
						Port:     "metrics",
						Path:     "/metrics",
						Interval: "30s",
						Scheme:   &schemeHTTPS,
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": smName,
					},
				},
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		schemeHTTPS = "https"
		testReconciler = NewTestReconciler(
			k8sClient,
			logf.Log.WithName("test"),
			k8sClient.Scheme(),
			OLSNamespaceDefault,
		)

		existing := &monv1.ServiceMonitor{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, existing)
		if err == nil {
			Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, &monv1.ServiceMonitor{})
			}).Should(HaveOccurred())
		} else {
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
		}
	})

	It("skips reconciliation when Prometheus Operator is unavailable", func() {
		testReconciler.PrometheusAvailable = false
		Expect(ReconcileServiceMonitor(testReconciler, ctx, desiredSM())).To(Succeed())

		err := k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, &monv1.ServiceMonitor{})
		Expect(err).To(HaveOccurred())
		Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
	})

	It("creates a ServiceMonitor when missing", func() {
		Expect(ReconcileServiceMonitor(testReconciler, ctx, desiredSM())).To(Succeed())

		found := &monv1.ServiceMonitor{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, found)).To(Succeed())
		Expect(found.Labels).To(HaveKeyWithValue("test-label", "v1"))
		Expect(found.Spec.Endpoints).To(HaveLen(1))
		Expect(found.Spec.Endpoints[0].Port).To(Equal("metrics"))
	})

	It("updates Spec and Labels when the ServiceMonitor drifts", func() {
		Expect(ReconcileServiceMonitor(testReconciler, ctx, desiredSM())).To(Succeed())

		found := &monv1.ServiceMonitor{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, found)).To(Succeed())
		found.Labels = map[string]string{"test-label": "drifted"}
		found.Spec.Endpoints[0].Interval = "60s"
		Expect(k8sClient.Update(ctx, found)).To(Succeed())

		Expect(ReconcileServiceMonitor(testReconciler, ctx, desiredSM())).To(Succeed())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, found)).To(Succeed())
		Expect(found.Labels).To(HaveKeyWithValue("test-label", "v1"))
		Expect(found.Spec.Endpoints[0].Interval).To(Equal(monv1.Duration("30s")))
	})

	It("skips update when ServiceMonitor is unchanged", func() {
		Expect(ReconcileServiceMonitor(testReconciler, ctx, desiredSM())).To(Succeed())

		found := &monv1.ServiceMonitor{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, found)).To(Succeed())
		oldRV := found.ResourceVersion

		Expect(ReconcileServiceMonitor(testReconciler, ctx, desiredSM())).To(Succeed())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: smName, Namespace: OLSNamespaceDefault}, found)).To(Succeed())
		Expect(found.ResourceVersion).To(Equal(oldRV))
	})
})

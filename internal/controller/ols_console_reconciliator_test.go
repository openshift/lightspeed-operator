package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Console UI reconciliator", Ordered, func() {

	Context("Creation logic", Ordered, func() {
		BeforeAll(func() {
			console := openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					Plugins: []string{"monitoring-plugin"},
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
				},
			}
			err := k8sClient.Create(ctx, &console)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileConsoleUI(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service lightspeed-console-plugin", func() {
			By("Get the service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIServiceName, Namespace: cr.Namespace}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a config map lightspeed-console-plugin", func() {
			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIConfigMapName, Namespace: cr.Namespace}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-console-plugin", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: cr.Namespace}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a console plugin lightspeed-console-plugin", func() {
			By("Get the console plugin")
			plugin := &consolev1.ConsolePlugin{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIPluginName}, plugin)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should activate the console plugin", func() {
			By("Get the console plugin")
			console := &openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			Expect(err).NotTo(HaveOccurred())
			Expect(console.Spec.Plugins).To(ContainElement(ConsoleUIPluginName))
		})

	})
})

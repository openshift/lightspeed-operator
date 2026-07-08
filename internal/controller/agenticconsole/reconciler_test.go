package agenticconsole

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Agentic Console UI reconciler", Ordered, func() {
	Context("Creation logic", Ordered, func() {
		var tlsSecret *corev1.Secret

		BeforeAll(func() {
			console := openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{Name: utils.ConsoleCRName},
				Spec: openshiftv1.ConsoleSpec{
					Plugins: []string{"monitoring-plugin"},
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &console)).To(Succeed())

			tlsSecret, _ = utils.GenerateRandomTLSSecret()
			tlsSecret.Name = utils.AgenticConsoleUIServiceCertSecretName
			Expect(testReconcilerInstance.Create(ctx, tlsSecret)).To(Succeed())

			Expect(k8sClient.Get(ctx, crNamespacedName, cr)).To(Succeed())
			crDefault := utils.GetDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
		})

		AfterAll(func() {
			console := openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.ConsoleCRName}, &console)
			if err == nil {
				Expect(k8sClient.Delete(ctx, &console)).To(Succeed())
			}
			_ = testReconcilerInstance.Delete(ctx, tlsSecret)
		})

		It("should reconcile and create plugin resources", func() {
			Expect(ReconcileAgenticConsoleUI(testReconcilerInstance, ctx, cr)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIServiceName, Namespace: utils.OLSNamespaceDefault}, &corev1.Service{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIConfigMapName, Namespace: utils.OLSNamespaceDefault}, &corev1.ConfigMap{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIDeploymentName, Namespace: utils.OLSNamespaceDefault}, &appsv1.Deployment{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIPluginName}, &consolev1.ConsolePlugin{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUINetworkPolicyName, Namespace: utils.OLSNamespaceDefault}, &networkingv1.NetworkPolicy{})).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIServiceAccountName, Namespace: utils.OLSNamespaceDefault}, &corev1.ServiceAccount{})).To(Succeed())
		})

		It("should activate the agentic console plugin", func() {
			console := &openshiftv1.Console{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.ConsoleCRName}, console)).To(Succeed())
			Expect(console.Spec.Plugins).To(ContainElement(utils.AgenticConsoleUIPluginName))
		})
	})

	Context("Deleting logic", Ordered, func() {
		BeforeAll(func() {
			console := openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{Name: utils.ConsoleCRName},
				Spec: openshiftv1.ConsoleSpec{
					Plugins: []string{"monitoring-plugin", utils.AgenticConsoleUIPluginName},
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &console)).To(Succeed())
		})

		AfterAll(func() {
			console := openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.ConsoleCRName}, &console)
			if err == nil {
				Expect(k8sClient.Delete(ctx, &console)).To(Succeed())
			}
		})

		It("should remove the agentic console plugin", func() {
			Expect(RemoveAgenticConsole(testReconcilerInstance, ctx)).To(Succeed())

			plugin := &consolev1.ConsolePlugin{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIPluginName}, plugin)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			console := &openshiftv1.Console{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.ConsoleCRName}, console)).To(Succeed())
			Expect(console.Spec.Plugins).NotTo(ContainElement(utils.AgenticConsoleUIPluginName))
		})
	})

	It("should apply agentic console deployment overrides from the CR", func() {
		olsConfig := &olsv1alpha1.OLSConfig{}
		Expect(k8sClient.Get(ctx, crNamespacedName, olsConfig)).To(Succeed())
		olsConfig.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer = &olsv1alpha1.Config{
			NodeSelector: map[string]string{
				"agentic": "node",
			},
		}

		Expect(ReconcileAgenticConsoleUIDeployment(testReconcilerInstance, ctx, olsConfig)).To(Succeed())

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.AgenticConsoleUIDeploymentName, Namespace: utils.OLSNamespaceDefault}, dep)).To(Succeed())
		Expect(dep.Spec.Template.Spec.NodeSelector).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer.NodeSelector))
	})
})

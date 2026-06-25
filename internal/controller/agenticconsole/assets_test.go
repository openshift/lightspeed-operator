package agenticconsole

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("Agentic Console UI assets", func() {
	var cr *olsv1alpha1.OLSConfig
	labels := map[string]string{
		"app.kubernetes.io/name":       utils.AgenticConsoleUIPluginName,
		"app.kubernetes.io/component":  "console",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
	}

	Context("complete custom resource", func() {
		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
		})

		It("should generate the nginx config map", func() {
			cm, err := GenerateAgenticConsoleUIConfigMap(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(utils.AgenticConsoleUIConfigMapName))
			Expect(cm.Namespace).To(Equal(utils.OLSNamespaceDefault))
			Expect(cm.Labels).To(Equal(labels))
			Expect(cm.Data["nginx.conf"]).To(ContainSubstring("listen              9443 ssl"))
		})

		It("should generate the agentic console UI service", func() {
			svc, err := GenerateAgenticConsoleUIService(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Name).To(Equal(utils.AgenticConsoleUIServiceName))
			Expect(svc.Labels).To(Equal(labels))
			Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.AgenticConsoleUIServiceCertSecretName))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(utils.AgenticConsoleUIHTTPSPort)))
			Expect(svc.Spec.Ports[0].TargetPort.IntVal).To(Equal(int32(utils.AgenticConsoleUIHTTPSPort)))
			Expect(svc.Spec.Selector).To(Equal(map[string]string{
				"app.kubernetes.io/name": utils.AgenticConsoleUIPluginName,
			}))
		})

		It("should generate the agentic console UI deployment", func() {
			dep, err := GenerateAgenticConsoleUIDeployment(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(utils.AgenticConsoleUIDeploymentName))
			Expect(dep.Labels).To(Equal(labels))
			Expect(dep.Spec.Selector.MatchLabels).To(Equal(map[string]string{
				"app.kubernetes.io/name": utils.AgenticConsoleUIPluginName,
			}))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.AgenticConsoleUIContainerName))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.AgenticConsoleUIImageDefault))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(utils.AgenticConsoleUIHTTPSPort)))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].Name).To(BeEmpty())
			Expect(dep.Spec.Template.Spec.Containers[0].Resources).To(Equal(*utils.DefaultConsolePluginResourceRequirements()))
			Expect(dep.Spec.Template.Spec.Containers[0].Env).To(BeNil())
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should generate the agentic console UI plugin without proxy", func() {
			plugin, err := GenerateAgenticConsoleUIPlugin(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.Name).To(Equal(utils.AgenticConsoleUIPluginName))
			Expect(plugin.Labels).To(Equal(labels))
			Expect(plugin.Spec.DisplayName).To(Equal(utils.AgenticConsoleUIPluginDisplayName))
			Expect(plugin.Spec.Backend.Service.Name).To(Equal(utils.AgenticConsoleUIServiceName))
			Expect(plugin.Spec.Backend.Service.Port).To(Equal(int32(utils.AgenticConsoleUIHTTPSPort)))
			Expect(plugin.Spec.Proxy).To(BeNil())
		})

		It("should generate the agentic console UI plugin NetworkPolicy", func() {
			np, err := GenerateAgenticConsoleUINetworkPolicy(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(np.Name).To(Equal(utils.AgenticConsoleUINetworkPolicyName))
			Expect(np.Labels).To(Equal(labels))
		})

		It("should generate the agentic console UI service account", func() {
			sa, err := GenerateAgenticConsoleUIServiceAccount(testReconcilerInstance, cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(sa.Name).To(Equal(utils.AgenticConsoleUIServiceAccountName))
			Expect(sa.Namespace).To(Equal(utils.OLSNamespaceDefault))
		})
	})
})

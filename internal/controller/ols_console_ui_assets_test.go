package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	consolev1 "github.com/openshift/api/console/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Console UI assets", func() {
	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions
	labels := map[string]string{
		"app.kubernetes.io/component":  "console-plugin",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-console-plugin",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}

	Context("complete custom resource", func() {
		BeforeEach(func() {
			rOptions = &OLSConfigReconcilerOptions{
				ConsoleUIImage: ConsoleUIImageDefault,
				Namespace:      OLSNamespaceDefault,
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

		It("should generate the nginx config map", func() {
			cm, err := r.generateConsoleUIConfigMap(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(ConsoleUIConfigMapName))
			Expect(cm.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(cm.Labels).To(Equal(labels))

			// todo: check the nginx config
		})

		It("should generate the console UI service", func() {
			svc, err := r.generateConsoleUIService(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Name).To(Equal(ConsoleUIServiceName))
			Expect(svc.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(svc.Labels).To(Equal(labels))
			Expect(svc.ObjectMeta.Annotations["service.beta.openshift.io/serving-cert-secret-name"]).To(Equal(ConsoleUIServiceCertSecretName))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(ConsoleUIHTTPSPort)))
			Expect(svc.Spec.Ports[0].TargetPort.StrVal).To(Equal("https"))
			Expect(svc.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should generate the console UI deployment", func() {
			dep, err := r.generateConsoleUIDeployment(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Name).To(Equal(ConsoleUIDeploymentName))
			Expect(dep.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(dep.Labels).To(Equal(labels))
			Expect(dep.Spec.Template.Labels).To(Equal(labels))
			Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal("lightspeed-console-plugin"))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(r.Options.ConsoleUIImage))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(ConsoleUIHTTPSPort)))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].Name).To(Equal("https"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should generate the console UI plugin", func() {
			plugin, err := r.generateConsoleUIPlugin(cr)
			Expect(err).NotTo(HaveOccurred())
			Expect(plugin.Name).To(Equal(ConsoleUIPluginName))
			Expect(plugin.Labels).To(Equal(labels))
			Expect(plugin.Spec.Backend.Service.Name).To(Equal(ConsoleUIServiceName))
			Expect(plugin.Spec.Backend.Service.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(plugin.Spec.Backend.Service.Port).To(Equal(int32(ConsoleUIHTTPSPort)))
			Expect(plugin.Spec.Backend.Service.BasePath).To(Equal("/"))
			Expect(plugin.Spec.Backend.Type).To(Equal(consolev1.Service))

			Expect(plugin.Spec.Proxy).To(HaveLen(1))
			Expect(plugin.Spec.Proxy[0].Endpoint.Service.Name).To(Equal(OLSAppServerServiceName))
			Expect(plugin.Spec.Proxy[0].Endpoint.Service.Port).To(Equal(int32(OLSAppServerServicePort)))
			Expect(plugin.Spec.Proxy[0].Endpoint.Service.Namespace).To(Equal(OLSNamespaceDefault))
			Expect(plugin.Spec.Proxy[0].Endpoint.Type).To(Equal(consolev1.ProxyTypeService))
		})
	})
})

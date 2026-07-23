package utils

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

const testConsolePluginName = "test-console-plugin"

var testConsolePluginLabels = map[string]string{
	"app.kubernetes.io/name":       testConsolePluginName,
	"app.kubernetes.io/managed-by": "lightspeed-operator",
}

var _ = Describe("Console plugin shared utilities", func() {
	var (
		testReconciler *TestReconciler
		testCr         *olsv1alpha1.OLSConfig
		ctx            context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		testReconciler = NewTestReconciler(
			k8sClient,
			logf.Log.WithName("test"),
			k8sClient.Scheme(),
			OLSNamespaceDefault,
		)
		testCr = GetDefaultOLSConfigCR()
	})

	Describe("DefaultConsolePluginResourceRequirements", func() {
		It("returns expected defaults", func() {
			resources := DefaultConsolePluginResourceRequirements()
			Expect(resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("10m")))
			Expect(resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("50Mi")))
			Expect(resources.Limits).To(BeNil())
		})
	})

	Describe("GenerateConsolePluginNginxConfigMap", func() {
		It("creates a configmap with nginx.conf and owner reference", func() {
			cm, err := GenerateConsolePluginNginxConfigMap(
				testReconciler, testCr, testConsolePluginName, testConsolePluginLabels, "events {}",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Name).To(Equal(testConsolePluginName))
			Expect(cm.Data["nginx.conf"]).To(Equal("events {}"))
			Expect(cm.OwnerReferences).To(HaveLen(1))
			Expect(cm.OwnerReferences[0].Kind).To(Equal("OLSConfig"))
		})
	})

	Describe("GenerateConsolePluginNetworkPolicy", func() {
		It("allows ingress from openshift-console pods on the plugin port", func() {
			np, err := GenerateConsolePluginNetworkPolicy(
				testReconciler, testCr, testConsolePluginName, testConsolePluginLabels, ConsoleUIHTTPSPort,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(np.Name).To(Equal(testConsolePluginName))
			Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress}))
			Expect(np.Spec.Ingress).To(HaveLen(1))
			Expect(np.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels).To(HaveKeyWithValue(
				"kubernetes.io/metadata.name", "openshift-console",
			))
			Expect(np.Spec.Ingress[0].From[0].PodSelector.MatchLabels).To(HaveKeyWithValue("app", "console"))
			Expect(*np.Spec.Ingress[0].Ports[0].Port).To(Equal(intstr.FromInt32(ConsoleUIHTTPSPort)))
			Expect(np.Spec.PodSelector.MatchLabels).To(Equal(testConsolePluginLabels))
		})
	})

	Describe("GenerateConsolePluginDeployment", func() {
		It("builds an nginx console plugin deployment from options", func() {
			resources := DefaultConsolePluginResourceRequirements()
			dep, err := GenerateConsolePluginDeployment(testReconciler, testCr, ConsolePluginDeploymentOptions{
				Name:                testConsolePluginName,
				Labels:              testConsolePluginLabels,
				SelectorLabels:      testConsolePluginLabels,
				ServiceAccountName:  testConsolePluginName,
				ContainerName:       "console",
				Image:               "example/console:latest",
				Port:                ConsoleUIHTTPSPort,
				PortName:            "https",
				CertVolumeName:      "cert",
				CertSecretName:      testConsolePluginName + "-cert",
				NginxVolumeName:     "nginx-conf",
				NginxConfigMapName:  testConsolePluginName,
				NginxTempVolumeName: "nginx-tmp",
				Resources:           resources,
				DeploymentConfig:    olsv1alpha1.Config{},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("example/console:latest"))
			Expect(dep.Spec.Template.Spec.Containers[0].Ports[0].Name).To(Equal("https"))
			Expect(dep.Spec.Template.Spec.Volumes).To(HaveLen(3))
		})
	})

	Describe("ReconcileConsolePluginConfigMap", func() {
		It("creates and updates a configmap", func() {
			cm, err := GenerateConsolePluginNginxConfigMap(
				testReconciler, testCr, testConsolePluginName+"-cm", testConsolePluginLabels, "events {}",
			)
			Expect(err).NotTo(HaveOccurred())

			err = ReconcileConsolePluginConfigMap(testReconciler, ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			cm.Data["nginx.conf"] = "events { worker_connections 1024; }"
			err = ReconcileConsolePluginConfigMap(testReconciler, ctx, cm)
			Expect(err).NotTo(HaveOccurred())

			found := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: OLSNamespaceDefault}, found)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Data["nginx.conf"]).To(ContainSubstring("worker_connections"))
		})
	})

	Describe("ActivateConsolePlugin and DeactivateConsolePlugin", func() {
		BeforeEach(func() {
			console := &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{Name: ConsoleCRName},
				Spec: openshiftv1.ConsoleSpec{
					Plugins: []string{"monitoring-plugin"},
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
				},
			}
			Expect(k8sClient.Create(ctx, console)).To(Succeed())
		})

		AfterEach(func() {
			console := &openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			if err == nil {
				Expect(k8sClient.Delete(ctx, console)).To(Succeed())
			}
		})

		It("adds and removes a plugin from the Console CR", func() {
			err := ActivateConsolePlugin(testReconciler, ctx, testConsolePluginName)
			Expect(err).NotTo(HaveOccurred())

			console := &openshiftv1.Console{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			Expect(err).NotTo(HaveOccurred())
			Expect(console.Spec.Plugins).To(ContainElement(testConsolePluginName))

			err = DeactivateConsolePlugin(testReconciler, ctx, testConsolePluginName)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			Expect(err).NotTo(HaveOccurred())
			Expect(console.Spec.Plugins).NotTo(ContainElement(testConsolePluginName))
		})
	})

	Describe("RemoveConsolePlugin", func() {
		BeforeEach(func() {
			console := &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{Name: ConsoleCRName},
				Spec: openshiftv1.ConsoleSpec{
					Plugins: []string{testConsolePluginName},
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
				},
			}
			Expect(k8sClient.Create(ctx, console)).To(Succeed())

			plugin := &consolev1.ConsolePlugin{
				ObjectMeta: metav1.ObjectMeta{Name: testConsolePluginName},
				Spec: consolev1.ConsolePluginSpec{
					DisplayName: "Test Plugin",
					Backend: consolev1.ConsolePluginBackend{
						Type: consolev1.Service,
						Service: &consolev1.ConsolePluginService{
							Name:      testConsolePluginName,
							Namespace: OLSNamespaceDefault,
							Port:      ConsoleUIHTTPSPort,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plugin)).To(Succeed())
		})

		AfterEach(func() {
			plugin := &consolev1.ConsolePlugin{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: testConsolePluginName}, plugin)
			if plugin.Name != "" {
				_ = k8sClient.Delete(ctx, plugin)
			}
			console := &openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			if err == nil {
				_ = k8sClient.Delete(ctx, console)
			}
		})

		It("deactivates and deletes the ConsolePlugin CR", func() {
			err := RemoveConsolePlugin(testReconciler, ctx, testConsolePluginName)
			Expect(err).NotTo(HaveOccurred())

			plugin := &consolev1.ConsolePlugin{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: testConsolePluginName}, plugin)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			console := &openshiftv1.Console{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			Expect(err).NotTo(HaveOccurred())
			Expect(console.Spec.Plugins).NotTo(ContainElement(testConsolePluginName))
		})
	})

	Describe("WaitForConsolePluginTLSSecret", func() {
		It("waits until tls.key and tls.crt are present", func() {
			secretName := testConsolePluginName + "-tls"
			secret, err := GenerateRandomTLSSecret()
			Expect(err).NotTo(HaveOccurred())
			secret.Name = secretName
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, secret)
			}()

			err = WaitForConsolePluginTLSSecret(testReconciler, ctx, secretName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RunReconcileTasks", func() {
		It("aggregates errors when continueOnError is true", func() {
			tasks := []ReconcileTask{
				{
					Name: "failing task",
					Task: func(_ reconciler.Reconciler, _ context.Context, _ *olsv1alpha1.OLSConfig) error {
						return fmt.Errorf("boom")
					},
				},
			}
			err := RunReconcileTasks(testReconciler, ctx, testCr, "test phase", tasks, true)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failing task"))
		})

		It("stops on first error when continueOnError is false", func() {
			called := false
			tasks := []ReconcileTask{
				{
					Name: "failing task",
					Task: func(_ reconciler.Reconciler, _ context.Context, _ *olsv1alpha1.OLSConfig) error {
						return fmt.Errorf("boom")
					},
				},
				{
					Name: "second task",
					Task: func(_ reconciler.Reconciler, _ context.Context, _ *olsv1alpha1.OLSConfig) error {
						called = true
						return nil
					},
				},
			}
			err := RunReconcileTasks(testReconciler, ctx, testCr, "test phase", tasks, false)
			Expect(err).To(HaveOccurred())
			Expect(called).To(BeFalse())
		})
	})

	Describe("RestartConsolePluginDeployment", func() {
		It("sets the force-reload annotation on the deployment pod template", func() {
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testConsolePluginName + "-dep",
					Namespace: OLSNamespaceDefault,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: testConsolePluginLabels},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: testConsolePluginLabels},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "console", Image: "example:latest"}},
						},
					},
				},
			}
			Expect(controllerutil.SetControllerReference(testCr, dep, testReconciler.GetScheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, dep)
			}()

			err := RestartConsolePluginDeployment(testReconciler, ctx, dep.Name, dep)
			Expect(err).NotTo(HaveOccurred())

			found := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: OLSNamespaceDefault}, found)
			Expect(err).NotTo(HaveOccurred())
			Expect(found.Spec.Template.Annotations).To(HaveKey(ForceReloadAnnotationKey))
		})
	})
})

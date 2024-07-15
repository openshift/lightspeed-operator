package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Console UI reconciliator", Ordered, func() {

	Context("Creation logic", Ordered, func() {
		var tlsSecret *corev1.Secret
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
			By("create the console tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = ConsoleUIServiceCertSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       ConsoleUIServiceCertSecretName,
				},
			})
			secretCreationErr := reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			console := openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, &console)
			if err == nil {
				err = k8sClient.Delete(ctx, &console)
				Expect(err).NotTo(HaveOccurred())
				return
			} else if errors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			By("Delete the console tls secret")
			secretDeletionErr := reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileConsoleUI(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
			reconciler.updateStatusCondition(ctx, cr, typeConsolePluginReady, true, "All components are successfully deployed", nil)
			expectedCondition := metav1.Condition{
				Type:   typeConsolePluginReady,
				Status: metav1.ConditionTrue,
			}
			Expect(cr.Status.Conditions).To(ContainElement(HaveField("Type", expectedCondition.Type)))
			Expect(cr.Status.Conditions).To(ContainElement(HaveField("Status", expectedCondition.Status)))
		})

		It("should create a service lightspeed-console-plugin", func() {
			By("Get the service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIServiceName, Namespace: OLSNamespaceDefault}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a config map lightspeed-console-plugin", func() {
			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIConfigMapName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-console-plugin", func() {
			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
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

		It("should trigger rolling update of the console deployment when changing tls secret content", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[OLSConsoleTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			foundSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIServiceCertSecretName, Namespace: OLSNamespaceDefault}, foundSecret)
			Expect(err).NotTo(HaveOccurred())

			By("Update the console tls secret content")
			foundSecret.Data["tls.key"] = []byte("new-value")
			err = k8sClient.Update(ctx, foundSecret)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile the console
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the console")
			err = reconciler.reconcileConsoleUI(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the updated deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())

			// Verify that the hash in deployment annotations has been updated
			Expect(dep.Annotations[OLSConsoleTLSHashKey]).NotTo(Equal(oldHash))
		})

		It("should trigger rolling update of the console deployment when recreating tls secret", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())
			oldHash := dep.Spec.Template.Annotations[OLSConsoleTLSHashKey]
			Expect(oldHash).NotTo(BeEmpty())

			By("Delete the console tls secret")
			secretDeletionErr := reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Recreate the provider secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = ConsoleUIServiceCertSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       ConsoleUIServiceCertSecretName,
				},
			})
			secretCreationErr := reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			// Reconcile the console
			olsConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the console")
			err = reconciler.reconcileConsoleUI(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the updated deployment")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Spec.Template.Annotations).NotTo(BeNil())

			// Verify that the hash in deployment annotations has been updated
			Expect(dep.Annotations[OLSConsoleTLSHashKey]).NotTo(Equal(oldHash))
			By("Delete the console tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

	})

	It("should trigger rolling update of the deployment when updating the tolerations", func() {
		By("Get the deployment")
		dep := &appsv1.Deployment{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
		Expect(err).NotTo(HaveOccurred())

		By("Update the OLSConfig custom resource")
		olsConfig := &olsv1alpha1.OLSConfig{}
		err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
		Expect(err).NotTo(HaveOccurred())
		olsConfig.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Tolerations = []corev1.Toleration{
			{
				Key:      "key",
				Operator: corev1.TolerationOpEqual,
				Value:    "value",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		}

		By("Reconcile the app server")
		err = reconciler.reconcileConsoleUIDeployment(ctx, olsConfig)
		Expect(err).NotTo(HaveOccurred())

		By("Get the deployment")
		err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Spec.Template.Spec.Tolerations).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Tolerations))
	})

	It("should trigger rolling update of the deployment when updating the nodeselector", func() {
		By("Get the deployment")
		dep := &appsv1.Deployment{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
		Expect(err).NotTo(HaveOccurred())

		By("Update the OLSConfig custom resource")
		olsConfig := &olsv1alpha1.OLSConfig{}
		err = k8sClient.Get(ctx, crNamespacedName, olsConfig)
		Expect(err).NotTo(HaveOccurred())
		olsConfig.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.NodeSelector = map[string]string{
			"key": "value",
		}

		By("Reconcile the app server")
		err = reconciler.reconcileConsoleUIDeployment(ctx, olsConfig)
		Expect(err).NotTo(HaveOccurred())

		By("Get the deployment")
		err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIDeploymentName, Namespace: OLSNamespaceDefault}, dep)
		Expect(err).NotTo(HaveOccurred())
		Expect(dep.Spec.Template.Spec.NodeSelector).To(Equal(olsConfig.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.NodeSelector))
	})

	Context("Deleting logic", Ordered, func() {
		var tlsSecret *corev1.Secret
		BeforeAll(func() {
			console := openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					Plugins: []string{"monitoring-plugin", ConsoleUIPluginName},
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
				},
			}
			err := k8sClient.Create(ctx, &console)
			Expect(err).NotTo(HaveOccurred())
			By("create the console tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = ConsoleUIServiceCertSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       ConsoleUIServiceCertSecretName,
				},
			})
			secretCreationErr := reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			console := openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, &console)
			if err == nil {
				err = k8sClient.Delete(ctx, &console)
				Expect(err).NotTo(HaveOccurred())
				return
			} else if errors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			By("Delete the console tls secret")
			secretDeletionErr := reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should reconcile from OLSConfig custom resource", func() {
			By("Reconcile the OLSConfig custom resource")
			err := reconciler.reconcileConsoleUI(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete the console plugin lightspeed-console-plugin", func() {
			By("Delete the console plugin")
			err := reconciler.removeConsoleUI(ctx)
			Expect(err).NotTo(HaveOccurred())
			By("Get the console plugin")
			plugin := &consolev1.ConsolePlugin{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleUIPluginName}, plugin)
			Expect(errors.IsNotFound(err)).To(BeTrue())
			By("Get the console plugin list")
			console := &openshiftv1.Console{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, console)
			Expect(err).NotTo(HaveOccurred())
			Expect(console.Spec.Plugins).NotTo(ContainElement(ConsoleUIPluginName))

		})

	})
})

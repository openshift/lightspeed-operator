package controller

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("LSC App server reconciliator", Label("LSCBackend"), Ordered, func() {
	Context("Creation logic", Ordered, func() {
		var providerSecret *corev1.Secret
		var tlsSecret *corev1.Secret
		var configmap *corev1.ConfigMap
		var openshiftCertsConfigmap *corev1.ConfigMap
		BeforeEach(func() {
			By("create magic configmap")
			configmap, _ = generateRandomConfigMap()
			configmap.Name = LSCAppServerActivatorCmName
			configMapCreationErr := reconciler.Create(ctx, configmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())
			By("create the provider secret")
			providerSecret, _ = generateRandomSecret()
			secretCreationErr := reconciler.Create(ctx, providerSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the default tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = OLSCertsSecretName
			tlsSecret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       OLSCertsSecretName,
				},
			})
			secretCreationErr = reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the Openshift certificates config map")
			openshiftCertsConfigmap, _ = generateRandomConfigMap()
			openshiftCertsConfigmap.Name = DefaultOpenShiftCerts
			configMapCreationErr = reconciler.Create(ctx, openshiftCertsConfigmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())

			By("Set OLSConfig CR to default")
			crDefault := getDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, providerSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the magic configmap")
			configMapDeletionErr := reconciler.Delete(ctx, configmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())

			By("Delete the Openshift certificates config map")
			configMapDeletionErr = reconciler.Delete(ctx, openshiftCertsConfigmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should call reconcileAppServerLSC when the magic configmap exists", func() {
			By("Choose the correct reconcile function")
			reconcileFunc, err := reconciler.getAppServerReconcileFunction(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(reflect.ValueOf(reconcileFunc).Pointer()).To(Equal(reflect.ValueOf(reconciler.reconcileAppServerLSC).Pointer()))
		})

		It("should create a LSC configmap", func() {
			By("Reconcile the LSC app server")
			err := reconciler.reconcileAppServerLSC(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: AppServerConfigCmName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a Llama Stack configmap", func() {
			// todo: implement this test after the Llama Stack configmap implementation is complete
			Skip("Llama Stack configmap implementation is not complete")
		})

		It("should create a deployment lightspeed-app-server", func() {
			By("Reconcile the LSC app server")
			err := reconciler.reconcileAppServerLSC(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: OLSNamespaceDefault}, dep)
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("LSC ConfigMap reconciliation", Ordered, func() {
		var providerSecret *corev1.Secret
		var openshiftCertsConfigmap *corev1.ConfigMap
		BeforeEach(func() {
			By("create the provider secret")
			providerSecret, _ = generateRandomSecret()
			secretCreationErr := reconciler.Create(ctx, providerSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the tls secret")
			tlsSecret, _ = generateRandomSecret()
			tlsSecret.Name = OLSCertsSecretName
			secretCreationErr = reconciler.Create(ctx, tlsSecret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			By("create the Openshift certificates config map")
			openshiftCertsConfigmap, _ = generateRandomConfigMap()
			openshiftCertsConfigmap.Name = DefaultOpenShiftCerts
			configMapCreationErr := reconciler.Create(ctx, openshiftCertsConfigmap)
			Expect(configMapCreationErr).NotTo(HaveOccurred())

			By("Set OLSConfig CR to default")
			crDefault := getDefaultOLSConfigCR()
			cr.Spec = crDefault.Spec
		})

		AfterEach(func() {
			By("Delete the provider secret")
			secretDeletionErr := reconciler.Delete(ctx, providerSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the tls secret")
			secretDeletionErr = reconciler.Delete(ctx, tlsSecret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())

			By("Delete the Openshift certificates config map")
			configMapDeletionErr := reconciler.Delete(ctx, openshiftCertsConfigmap)
			Expect(configMapDeletionErr).NotTo(HaveOccurred())
		})

		It("should create a new LSC configmap when it does not exist", func() {
			By("Reconcile the LSC configmap")
			err := reconciler.reconcileLSCConfigMap(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Get the configmap")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: AppServerConfigCmName, Namespace: OLSNamespaceDefault}, cm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})

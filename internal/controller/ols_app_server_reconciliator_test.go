package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("App server reconciliator", Ordered, func() {

	ctx := context.Background()
	var reconciler *OLSConfigReconciler
	BeforeAll(func() {
		By("Create the namespace openshift-lightspeed")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-lightspeed",
			},
		}
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		reconciler = &OLSConfigReconciler{
			Options: OLSConfigReconcilerOptions{
				LightspeedServiceImage:      "lightspeed-service:latest",
				LightspeedServiceRedisImage: "lightspeed-service-redis:latest",
			},
			logger:     logf.Log.WithName("olsconfig.reconciler"),
			Client:     k8sClient,
			Scheme:     k8sClient.Scheme(),
			stateCache: make(map[string]string),
		}
	})
	AfterAll(func() {
		By("Delete the namespace openshift-lightspeed")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-lightspeed",
			},
		}
		err := k8sClient.Delete(ctx, ns)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Creation logic", Ordered, func() {

		cr := &olsv1alpha1.OLSConfig{}
		crNamespacedName := types.NamespacedName{
			Name:      "cluster",
			Namespace: "openshift-lightspeed",
		}

		It("should reconcile from OLSConfig custom resource", func() {
			By("Create a complete OLSConfig custom resource")
			err := k8sClient.Get(ctx, crNamespacedName, cr)
			if err != nil && errors.IsNotFound(err) {
				cr = getCompleteOLSConfigCR()
				err = k8sClient.Create(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
			} else if err == nil {
				cr = getCompleteOLSConfigCR()
				err = k8sClient.Update(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
			} else {
				Fail("Failed to create or update the OLSConfig custom resource")
			}

			By("Get the OLSConfig custom resource")
			err = k8sClient.Get(ctx, crNamespacedName, cr)
			Expect(err).NotTo(HaveOccurred())

			By("Reconcile the OLSConfig custom resource")
			err = reconciler.reconcileAppServer(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should create a service account lightspeed-app-server", func() {

			By("Get the service account")
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerServiceAccountName, Namespace: cr.Namespace}, sa)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service lightspeed-app-server", func() {

			By("Get the service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerServiceName, Namespace: cr.Namespace}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a service for lightspeed-redis-server", func() {

			By("Get redis service")
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppRedisServiceName, Namespace: cr.Namespace}, svc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a config map olsconfig", func() {

			By("Get the config map")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigCmName, Namespace: cr.Namespace}, cm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create a deployment lightspeed-app-server", func() {

			By("Get the deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppServerDeploymentName, Namespace: cr.Namespace}, dep)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should create a deployment lightspeed-redis-server", func() {

			By("Get redis deployment")
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: OLSAppRedisDeploymentName, Namespace: cr.Namespace}, dep)
			Expect(err).NotTo(HaveOccurred())

		})

	})

})

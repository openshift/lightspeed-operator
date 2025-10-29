package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("LSC App server assets", Label("LSCBackend"), Ordered, func() {
	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions
	var ctx context.Context

	Context("LSC asset generation", func() {
		BeforeEach(func() {
			ctx = context.Background()
			rOptions = &OLSConfigReconcilerOptions{
				OpenShiftMajor:          "123",
				OpenshiftMinor:          "456",
				LightspeedServiceImage:  "lightspeed-service:latest",
				OpenShiftMCPServerImage: "openshift-mcp-server:latest",
				Namespace:               OLSNamespaceDefault,
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

		Describe("generateLSCConfigMap", func() {
			It("should generate a valid configmap", func() {
				cm, err := r.generateLSCConfigMap(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(cm).NotTo(BeNil())
			})

			// TODO: Add more tests cases for once implementation is complete
		})

		Describe("generateLSCDeployment", func() {
			It("should generate a valid deployment", func() {
				deployment, err := r.generateLSCDeployment(ctx, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(deployment).NotTo(BeNil())
			})

			// TODO: Add more tests cases for once implementation is complete
		})

		Describe("updateLSCDeployment", func() {
			var existingDeployment *appsv1.Deployment
			var desiredDeployment *appsv1.Deployment

			BeforeEach(func() {
				existingDeployment, _ = r.generateLSCDeployment(ctx, cr)
			})

			It("should successfully update deployment", func() {
				desiredDeployment, _ = r.generateLSCDeployment(ctx, cr)
				err := r.updateLSCDeployment(ctx, existingDeployment, desiredDeployment)
				Expect(err).NotTo(HaveOccurred())
			})

			// TODO: Add more tests cases for once implementation is complete
		})
	})

})

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("Watchers", func() {

	Context("secret", Ordered, func() {
		ctx := context.Background()
		It("should identify watched secret by annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-secret"},
			}
			requests := SecretWatcherFilter(ctx, secret)
			Expect(requests).To(BeEmpty())

			AnnotateSecretWatcher(secret)
			requests = SecretWatcherFilter(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))
		})
	})

	Context("configmap", Ordered, func() {
		ctx := context.Background()
		It("should identify watched configmap by annotations", func() {
			// Create a reconciler instance for testing
			r := &OLSConfigReconciler{
				Options: utils.OLSConfigReconcilerOptions{
					Namespace: "default",
				},
			}

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-configmap"},
			}
			requests := r.ConfigMapWatcherFilter(ctx, configMap, false)
			Expect(requests).To(BeEmpty())

			AnnotateConfigMapWatcher(configMap)
			requests = r.ConfigMapWatcherFilter(ctx, configMap, false)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))

			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: utils.DefaultOpenShiftCerts},
			}
			requests = r.ConfigMapWatcherFilter(ctx, configMap, false)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))
		})
	})

})

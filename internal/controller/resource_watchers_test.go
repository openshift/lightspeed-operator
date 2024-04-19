package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Watchers", func() {

	Context("secret", Ordered, func() {
		ctx := context.Background()
		It("should identify watched secret by annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-secret"},
			}
			requests := secretWatcherFilter(ctx, secret)
			Expect(requests).To(BeEmpty())

			annotateSecretWatcher(secret)
			requests = secretWatcherFilter(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(OLSConfigName))
		})
	})

})

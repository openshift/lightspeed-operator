package watchers

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestWatchers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Watchers Suite")
}

// Helper function to create a test reconciler
func createTestReconciler() reconciler.Reconciler {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	logger := zap.New(zap.UseDevMode(true))

	return utils.NewTestReconciler(fakeClient, logger, s, "default")
}

var _ = Describe("Watchers", func() {

	Context("secret", Ordered, func() {
		ctx := context.Background()

		It("should identify watched secret by annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-secret"},
			}
			requests := SecretWatcherFilter(ctx, secret)
			Expect(requests).To(BeEmpty())

			utils.AnnotateSecretWatcher(secret)
			requests = SecretWatcherFilter(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))
		})

		It("should identify telemetry pull secret by namespace and name", func() {
			// Wrong namespace
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "wrong-namespace",
					Name:      utils.TelemetryPullSecretName,
				},
			}
			requests := SecretWatcherFilter(ctx, secret)
			Expect(requests).To(BeEmpty())

			// Wrong name
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.TelemetryPullSecretNamespace,
					Name:      "wrong-name",
				},
			}
			requests = SecretWatcherFilter(ctx, secret)
			Expect(requests).To(BeEmpty())

			// Correct namespace and name (telemetry pull secret)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: utils.TelemetryPullSecretNamespace,
					Name:      utils.TelemetryPullSecretName,
				},
			}
			requests = SecretWatcherFilter(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))
		})
	})

	Context("configmap", Ordered, func() {
		ctx := context.Background()
		It("should identify watched configmap by annotations", func() {
			// Create a test reconciler instance
			r := createTestReconciler()

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-configmap"},
			}
			// useLCore=false (AppServer), inCluster=false (skip restart)
			requests := ConfigMapWatcherFilter(r, ctx, configMap, false, false)
			Expect(requests).To(BeEmpty())

			utils.AnnotateConfigMapWatcher(configMap)
			requests = ConfigMapWatcherFilter(r, ctx, configMap, false, false)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))
		})

		It("should identify OpenShift default certs configmap by name", func() {
			r := createTestReconciler()

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: utils.DefaultOpenShiftCerts},
			}
			// useLCore=false (AppServer), inCluster=false (skip restart)
			requests := ConfigMapWatcherFilter(r, ctx, configMap, false, false)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(utils.OLSConfigName))
		})
	})

})

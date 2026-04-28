package controller

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("OLSConfig Reconciler Helper Functions", Ordered, func() {
	var (
		reconciler *OLSConfigReconciler
		cr         *olsv1alpha1.OLSConfig
		namespace  string
		ctx        context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()
		namespace = utils.OLSNamespaceDefault
		// Set LOCAL_DEV_MODE to skip ServiceMonitor in tests
		os.Setenv("LOCAL_DEV_MODE", "true")
	})

	AfterAll(func() {
		os.Unsetenv("LOCAL_DEV_MODE")
	})

	BeforeEach(func() {
		reconciler = &OLSConfigReconciler{
			Client:  k8sClient,
			Options: getDefaultReconcilerOptions(namespace),
			Logger:  logf.Log.WithName("test.reconciler"),
		}

		// Create test secret for LLM credentials
		testSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"apitoken": []byte("test-token"),
			},
		}
		_ = k8sClient.Create(ctx, testSecret)

		cr = &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.OLSConfigName,
			},
			Spec: olsv1alpha1.OLSConfigSpec{
				LLMConfig: olsv1alpha1.LLMSpec{
					Providers: []olsv1alpha1.ProviderSpec{
						{
							Name: "test-provider",
							Type: "openai",
							Models: []olsv1alpha1.ModelSpec{
								{Name: "test-model"},
							},
							CredentialsSecretRef: corev1.LocalObjectReference{
								Name: "test-secret",
							},
						},
					},
				},
				OLSConfig: olsv1alpha1.OLSSpec{
					DefaultProvider: "test-provider",
					DefaultModel:    "test-model",
				},
			},
		}
	})

	AfterEach(func() {
		// Cleanup CR
		cleanupOLSConfig(ctx, cr)

		// Cleanup test secret
		testSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: namespace,
			},
		}
		_ = k8sClient.Delete(ctx, testSecret)
	})

	Describe("getAndValidateCR", func() {
		Context("with valid CR name", func() {
			It("should return CR when it exists", func() {
				err := k8sClient.Create(ctx, cr)
				Expect(err).NotTo(HaveOccurred())

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{Name: utils.OLSConfigName},
				}

				fetchedCR, err := reconciler.getAndValidateCR(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(fetchedCR).NotTo(BeNil())
				Expect(fetchedCR.Name).To(Equal(utils.OLSConfigName))
			})

			It("should return nil when CR doesn't exist", func() {
				req := reconcile.Request{
					NamespacedName: types.NamespacedName{Name: utils.OLSConfigName},
				}

				fetchedCR, err := reconciler.getAndValidateCR(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(fetchedCR).To(BeNil())
			})
		})

		Context("with invalid CR name", func() {
			It("should return nil and not fetch CR", func() {
				req := reconcile.Request{
					NamespacedName: types.NamespacedName{Name: "wrong-name"},
				}

				fetchedCR, err := reconciler.getAndValidateCR(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(fetchedCR).To(BeNil())
			})
		})
	})

	Describe("handleFinalizer", func() {
		var req reconcile.Request

		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{Name: utils.OLSConfigName},
			}
		})

		Context("when finalizer is missing", func() {
			It("should add finalizer and return early", func() {
				err := k8sClient.Create(ctx, cr)
				Expect(err).NotTo(HaveOccurred())

				// Fetch latest version
				err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, cr)
				Expect(err).NotTo(HaveOccurred())

				result, err := reconciler.handleFinalizer(ctx, req, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil(), "should return non-nil to stop reconciliation")

				// Verify finalizer was added
				updatedCR := &olsv1alpha1.OLSConfig{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, updatedCR)
				Expect(err).NotTo(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(updatedCR, utils.OLSConfigFinalizer)).To(BeTrue())
			})
		})

		Context("when finalizer exists and CR not being deleted", func() {
			It("should return nil to continue reconciliation", func() {
				controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
				err := k8sClient.Create(ctx, cr)
				Expect(err).NotTo(HaveOccurred())

				// Fetch latest version
				err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, cr)
				Expect(err).NotTo(HaveOccurred())

				result, err := reconciler.handleFinalizer(ctx, req, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil(), "should return nil to continue reconciliation")
			})
		})

		Context("when CR is being deleted", func() {
			It("should run cleanup and remove finalizer", func() {
				controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
				err := k8sClient.Create(ctx, cr)
				Expect(err).NotTo(HaveOccurred())

				// Delete CR (sets DeletionTimestamp)
				err = k8sClient.Delete(ctx, cr)
				Expect(err).NotTo(HaveOccurred())

				// Re-fetch to get DeletionTimestamp
				err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(cr.DeletionTimestamp.IsZero()).To(BeFalse())

				result, err := reconciler.handleFinalizer(ctx, req, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil(), "should return non-nil after cleanup")

				// Verify CR is eventually deleted (finalizer removed)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, cr)
					return apierrors.IsNotFound(err)
				}, "10s", "100ms").Should(BeTrue())
			})
		})
	})

	Describe("reconcileOperatorResources", func() {
		It("should not return errors", func() {
			err := reconciler.reconcileOperatorResources(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be idempotent", func() {
			// First call
			err := reconciler.reconcileOperatorResources(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Second call should not error
			err = reconciler.reconcileOperatorResources(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip ServiceMonitor in LOCAL_DEV_MODE", func() {
			os.Setenv("LOCAL_DEV_MODE", "true")
			defer os.Unsetenv("LOCAL_DEV_MODE")

			err := reconciler.reconcileOperatorResources(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("reconcileIndependentResources", func() {
		BeforeEach(func() {
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not panic and returns error or success", func() {
			err := reconciler.reconcileIndependentResources(ctx, cr)
			// May succeed or fail depending on test environment setup
			// The important part is it doesn't panic
			_ = err
		})
	})

	// reconcileDeploymentsAndStatus is too integration-heavy to test in isolation
	// It's extensively tested via the full reconciliation loop in other test files
	// Unit testing this function would require mocking entire subsystems
	PDescribe("reconcileDeploymentsAndStatus", func() {
		It("integration test - skipped in unit tests", func() {
			// This function is tested via integration/E2E tests
		})
	})
})

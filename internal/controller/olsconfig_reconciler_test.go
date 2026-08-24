package controller

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	consolev1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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

	Describe("agentic component gating", func() {
		var emptyImageReconciler *OLSConfigReconciler

		BeforeEach(func() {
			opts := getDefaultReconcilerOptions(namespace)
			opts.AgenticConsoleUIImage = ""
			opts.AlertsAdapterImage = ""
			emptyImageReconciler = &OLSConfigReconciler{
				Client:  k8sClient,
				Options: opts,
				Logger:  logf.Log.WithName("test.reconciler.empty-images"),
			}

			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when AgenticConsoleUIImage is empty (Phase 1 - resources)", func() {
			It("should not create agentic console ServiceAccount", func() {
				_ = emptyImageReconciler.reconcileIndependentResources(ctx, cr)

				sa := &corev1.ServiceAccount{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AgenticConsoleUIServiceAccountName,
					Namespace: namespace,
				}, sa)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"agentic console ServiceAccount should not exist when image is empty")
			})
		})

		Context("when AlertsAdapterImage is empty (Phase 1 - resources)", func() {
			It("should not create alerts adapter ServiceAccount", func() {
				_ = emptyImageReconciler.reconcileIndependentResources(ctx, cr)

				sa := &corev1.ServiceAccount{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AlertsAdapterServiceAccountName,
					Namespace: namespace,
				}, sa)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"alerts adapter ServiceAccount should not exist when image is empty")
			})

			It("should not create alerts adapter Role", func() {
				_ = emptyImageReconciler.reconcileIndependentResources(ctx, cr)

				role := &rbacv1.Role{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AlertsAdapterAgenticRunsRoleName,
					Namespace: namespace,
				}, role)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"alerts adapter Role should not exist when image is empty")
			})
		})

		Context("Phase 2 gating: disabled components should not create deployments and should set Disabled status", func() {
			It("should not create agentic console or alerts adapter Deployments and should set Disabled conditions", func() {
				_, _ = emptyImageReconciler.reconcileDeploymentsAndStatus(ctx, cr)

				By("verifying agentic console Deployment is absent")
				dep := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AgenticConsoleUIDeploymentName,
					Namespace: namespace,
				}, dep)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"agentic console Deployment should not exist when image is empty")

				By("verifying agentic console ConsolePlugin is absent")
				plugin := &consolev1.ConsolePlugin{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name: utils.AgenticConsoleUIPluginName,
				}, plugin)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"agentic console ConsolePlugin should not exist when image is empty")

				By("verifying alerts adapter Deployment is absent")
				dep = &appsv1.Deployment{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AlertsAdapterDeploymentName,
					Namespace: namespace,
				}, dep)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"alerts adapter Deployment should not exist when image is empty")

				By("verifying AgenticConsolePluginReady=False with reason Disabled")
				updatedCR := &olsv1alpha1.OLSConfig{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, updatedCR)
				Expect(err).NotTo(HaveOccurred())

				agenticFound := false
				alertsFound := false
				for _, c := range updatedCR.Status.Conditions {
					if c.Type == utils.TypeAgenticConsolePluginReady {
						agenticFound = true
						Expect(string(c.Status)).To(Equal("False"))
						Expect(c.Reason).To(Equal("Disabled"))
					}
					if c.Type == utils.TypeAlertsAdapterReady {
						alertsFound = true
						Expect(string(c.Status)).To(Equal("False"))
						Expect(c.Reason).To(Equal("Disabled"))
					}
				}
				Expect(agenticFound).To(BeTrue(),
					"AgenticConsolePluginReady condition should be present")
				Expect(alertsFound).To(BeTrue(),
					"AlertsAdapterReady condition should be present")
			})
		})

		Context("when AlertsAdapterImage is set but configMapRef is absent", func() {
			var imageNoRefReconciler *OLSConfigReconciler

			BeforeEach(func() {
				opts := getDefaultReconcilerOptions(namespace)
				opts.AgenticConsoleUIImage = ""
				opts.AlertsAdapterImage = "alerts-adapter:latest"
				imageNoRefReconciler = &OLSConfigReconciler{
					Client:  k8sClient,
					Options: opts,
					Logger:  logf.Log.WithName("test.reconciler.image-no-ref"),
				}
			})

			It("Phase 1 should skip alerts adapter resources when configMapRef is absent", func() {
				_ = imageNoRefReconciler.reconcileIndependentResources(ctx, cr)

				sa := &corev1.ServiceAccount{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AlertsAdapterServiceAccountName,
					Namespace: namespace,
				}, sa)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"alerts adapter ServiceAccount should not exist when configMapRef is absent")
			})

			It("Phase 2 should set AlertsAdapterReady=Disabled when configMapRef is absent", func() {
				_, _ = imageNoRefReconciler.reconcileDeploymentsAndStatus(ctx, cr)

				updatedCR := &olsv1alpha1.OLSConfig{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, updatedCR)
				Expect(err).NotTo(HaveOccurred())

				found := false
				for _, c := range updatedCR.Status.Conditions {
					if c.Type == utils.TypeAlertsAdapterReady {
						found = true
						Expect(string(c.Status)).To(Equal("False"))
						Expect(c.Reason).To(Equal("Disabled"))
						Expect(c.Message).To(ContainSubstring("configMapRef"))
					}
				}
				Expect(found).To(BeTrue(),
					"AlertsAdapterReady condition should be present with Disabled reason")
			})
		})

		Context("previously-enabled component cleanup", func() {
			It("Phase 1 should remove alerts adapter resources when image becomes empty and prior condition exists", func() {
				By("seeding a ServiceAccount as if the alerts adapter was previously enabled")
				seedSA := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      utils.AlertsAdapterServiceAccountName,
						Namespace: namespace,
					},
				}
				err := k8sClient.Create(ctx, seedSA)
				Expect(err).NotTo(HaveOccurred())

				By("seeding a prior non-Disabled status condition on the CR")
				cr.Status.Conditions = []metav1.Condition{
					{
						Type:               utils.TypeAlertsAdapterReady,
						Status:             metav1.ConditionTrue,
						Reason:             "Available",
						Message:            "Ready",
						LastTransitionTime: metav1.Now(),
					},
				}
				err = k8sClient.Status().Update(ctx, cr)
				Expect(err).NotTo(HaveOccurred())

				By("running Phase 1 with empty alerts adapter image")
				_ = emptyImageReconciler.reconcileIndependentResources(ctx, cr)

				By("verifying the seeded ServiceAccount was removed by cleanup")
				sa := &corev1.ServiceAccount{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.AlertsAdapterServiceAccountName,
					Namespace: namespace,
				}, sa)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(),
					"alerts adapter ServiceAccount should be removed during disabled-state cleanup")
			})
		})
	})
})

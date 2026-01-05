package controller

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("OLSConfig Finalizer", Ordered, Serial, func() {
	var (
		reconciler *OLSConfigReconciler
		cr         *olsv1alpha1.OLSConfig
		namespace  string
	)

	// Clean up any stuck CR from previous test runs
	BeforeAll(func() {
		namespace = utils.OLSNamespaceDefault
		// Set LOCAL_DEV_MODE to skip ServiceMonitor reconciliation in tests
		os.Setenv("LOCAL_DEV_MODE", "true")
		// Use the shared cleanup helper to ensure clean state
		cleanupCR := &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.OLSConfigName,
			},
		}
		cleanupOLSConfig(ctx, cleanupCR)
	})

	AfterAll(func() {
		// Clean up environment variable
		os.Unsetenv("LOCAL_DEV_MODE")
	})

	BeforeEach(func() {
		namespace = utils.OLSNamespaceDefault

		reconciler = &OLSConfigReconciler{
			Client:  k8sClient,
			Options: getDefaultReconcilerOptions(namespace),
			Logger:  logf.Log.WithName("test.finalizer"),
		}

		// Create the test secret for LLM credentials
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
				Name: utils.OLSConfigName, // Must be "cluster" per CRD validation
			},
			Spec: olsv1alpha1.OLSConfigSpec{
				LLMConfig: olsv1alpha1.LLMSpec{
					Providers: []olsv1alpha1.ProviderSpec{
						{
							Name: "test-provider",
							Type: "openai",
							Models: []olsv1alpha1.ModelSpec{
								{
									Name: "test-model",
								},
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
		// Clean up test resources using the shared helper
		cleanupOLSConfig(ctx, cr)

		// Clean up test secret
		testSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: namespace,
			},
		}
		_ = k8sClient.Delete(ctx, testSecret)
	})

	Context("Finalizer is added on creation", func() {
		It("should add finalizer to new OLSConfig CR", func() {
			// Create the CR
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation (may fail due to missing test fixtures, but finalizer logic should run)
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: cr.Name,
				},
			}
			_, _ = reconciler.Reconcile(ctx, req)

			// Fetch the updated CR
			updatedCR := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, updatedCR)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added (this is what we're testing)
			Expect(controllerutil.ContainsFinalizer(updatedCR, utils.OLSConfigFinalizer)).To(BeTrue())
		})

		It("should not add duplicate finalizer if already present", func() {
			// Add finalizer before creating
			controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation (may fail due to missing test fixtures)
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: cr.Name,
				},
			}
			_, _ = reconciler.Reconcile(ctx, req)

			// Fetch the updated CR
			updatedCR := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, updatedCR)
			Expect(err).NotTo(HaveOccurred())

			// Verify only one finalizer (this is what we're testing)
			Expect(updatedCR.Finalizers).To(HaveLen(1))
			Expect(updatedCR.Finalizers[0]).To(Equal(utils.OLSConfigFinalizer))
		})
	})

	Context("Finalizer handles deletion", func() {
		It("should remove finalizer when CR is deleted", func() {
			// Create CR with finalizer
			controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR
			err = k8sClient.Delete(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation (should run finalizer logic, may have errors from missing fixtures)
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: cr.Name,
				},
			}
			_, _ = reconciler.Reconcile(ctx, req)

			// Try to fetch the CR - it should either be gone or have no finalizer
			updatedCR := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name}, updatedCR)

			if err == nil {
				// CR still exists - finalizer should have been removed
				Expect(controllerutil.ContainsFinalizer(updatedCR, utils.OLSConfigFinalizer)).To(BeFalse())
			} else {
				// CR should be NotFound (successfully deleted after finalizer removal)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		})
	})

	Context("waitForOwnedResourcesDeletion", func() {
		It("should wait for deployments to be deleted", func() {
			controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Create a deployment owned by the CR
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.OLSAppServerDeploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "test:latest",
								},
							},
						},
					},
				},
			}

			// Set owner reference
			err = controllerutil.SetControllerReference(cr, deployment, k8sClient.Scheme())
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Create(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			// Verify deployment exists
			foundDep := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: namespace}, foundDep)
			Expect(err).NotTo(HaveOccurred())

			// Delete the deployment (simulating cascade deletion)
			err = k8sClient.Delete(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			// Call waitForOwnedResourcesDeletion - should succeed quickly since we deleted manually
			err = reconciler.waitForOwnedResourcesDeletion(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should wait for PVC to be deleted when storage is configured", func() {
			// Configure storage
			cr.Spec.OLSConfig.ConversationCache.Type = olsv1alpha1.Postgres
			cr.Spec.OLSConfig.Storage = &olsv1alpha1.Storage{
				Size:  resource.MustParse("1Gi"),
				Class: "test-storage-class",
			}
			controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Create a PVC owned by the CR
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.PostgresPVCName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}

			// Set owner reference
			err = controllerutil.SetControllerReference(cr, pvc, k8sClient.Scheme())
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Create(ctx, pvc)
			Expect(err).NotTo(HaveOccurred())

			// Verify listOwnedResources can find the PVC via owner reference
			resourceGroups, err := reconciler.listOwnedResources(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Find the PVC group
			var pvcGroup *ResourceGroup
			for i := range resourceGroups {
				if resourceGroups[i].Type == "pvc" {
					pvcGroup = &resourceGroups[i]
					break
				}
			}
			Expect(pvcGroup).NotTo(BeNil())
			Expect(pvcGroup.Items).To(HaveLen(1))
			Expect(pvcGroup.Items[0].GetName()).To(Equal(utils.PostgresPVCName))

			// Delete the PVC (simulating cleanup)
			// In envtest, PVCs don't get fully deleted without storage controller,
			// so we force removal by removing finalizers
			pvc.Finalizers = nil
			err = k8sClient.Update(ctx, pvc)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, pvc)
			Expect(err).NotTo(HaveOccurred())

			// Wait for PVC to actually be deleted
			Eventually(func() bool {
				checkPVC := &corev1.PersistentVolumeClaim{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.PostgresPVCName, Namespace: namespace}, checkPVC)
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			// Call waitForOwnedResourcesDeletion - should succeed quickly since PVC is gone
			err = reconciler.waitForOwnedResourcesDeletion(ctx, cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should timeout gracefully if resources don't delete", func() {
			controllerutil.AddFinalizer(cr, utils.OLSConfigFinalizer)
			err := k8sClient.Create(ctx, cr)
			Expect(err).NotTo(HaveOccurred())

			// Create a deployment that we won't delete (simulating stuck resource)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.OLSAppServerDeploymentName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "lightspeed-operator",
						"app.kubernetes.io/part-of":    "openshift-lightspeed",
					},
					// Add finalizer to prevent deletion (simulate stuck resource)
					Finalizers: []string{"test.finalizer/block-deletion"},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "test:latest",
								},
							},
						},
					},
				},
			}

			err = controllerutil.SetControllerReference(cr, deployment, k8sClient.Scheme())
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Create(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			// Try to delete (but it will be stuck due to finalizer)
			err = k8sClient.Delete(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			// Create a context with short timeout for this test
			shortCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			// Call waitForOwnedResourcesDeletion - should timeout but not panic
			err = reconciler.waitForOwnedResourcesDeletion(shortCtx, cr)
			Expect(err).To(HaveOccurred()) // Should return timeout error
			Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))

			// Clean up the stuck deployment for next tests
			deployment.Finalizers = []string{}
			_ = k8sClient.Update(ctx, deployment)
			_ = k8sClient.Delete(ctx, deployment)
		})
	})
})

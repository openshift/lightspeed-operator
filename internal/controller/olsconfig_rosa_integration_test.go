package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftv1 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("OLSConfig ROSA Integration", func() {

	Context("Reconcile with ROSA detection", func() {
		var console *openshiftv1.Console
		var olsConfig *olsv1alpha1.OLSConfig
		var secret *corev1.Secret

		BeforeEach(func() {
			// Clean up any existing resources
			existingConsole := &openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, existingConsole)
			if err == nil {
				_ = k8sClient.Delete(ctx, existingConsole)
			}

			existingOLSConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigName}, existingOLSConfig)
			if err == nil {
				_ = k8sClient.Delete(ctx, existingOLSConfig)
			}
		})

		AfterEach(func() {
			// Clean up resources
			if olsConfig != nil {
				_ = k8sClient.Delete(ctx, olsConfig)
				olsConfig = nil
			}
			if secret != nil {
				_ = k8sClient.Delete(ctx, secret)
				secret = nil
			}
			if console != nil {
				_ = k8sClient.Delete(ctx, console)
				console = nil
			}
		})

		It("should handle ROSA detection failure gracefully", func() {
			By("Creating an OLSConfig without Console object present")
			olsConfig = &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: OLSConfigName,
				},
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{
							{
								Name: "openai",
								Type: "openai",
								URL:  "https://api.openai.com/v1",
								CredentialsSecretRef: corev1.LocalObjectReference{
									Name: "openai-secret-1",
								},
								Models: []olsv1alpha1.ModelSpec{
									{
										Name: "gpt-3.5-turbo",
									},
								},
							},
						},
					},
					OLSConfig: olsv1alpha1.OLSSpec{
						DefaultModel:    "openai",
						DefaultProvider: "openai",
						LogLevel:        "INFO",
						ConversationCache: olsv1alpha1.ConversationCacheSpec{
							Type: "postgres",
						},
					},
				},
			}

			err := k8sClient.Create(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			// Create a secret for the provider
			secret, _ = generateRandomSecret()
			secret.Name = "openai-secret-1"
			err = k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: OLSConfigName,
				},
			}

			// The reconciliation should succeed even when Console object doesn't exist
			// because ROSA detection should gracefully handle missing Console object
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			By("Verifying OLSConfig status is updated")
			updatedOLSConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigName}, updatedOLSConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should detect ROSA environment and proceed with reconciliation", func() {
			By("Creating a Console object with ROSA branding")
			console = &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
					Customization: openshiftv1.ConsoleCustomization{
						Brand: "ROSA",
					},
				},
			}
			err := k8sClient.Create(ctx, console)
			Expect(err).NotTo(HaveOccurred())

			By("Creating an OLSConfig")
			olsConfig = &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: OLSConfigName,
				},
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{
							{
								Name: "openai",
								Type: "openai",
								URL:  "https://api.openai.com/v1",
								CredentialsSecretRef: corev1.LocalObjectReference{
									Name: "openai-secret-2",
								},
								Models: []olsv1alpha1.ModelSpec{
									{
										Name: "gpt-3.5-turbo",
									},
								},
							},
						},
					},
					OLSConfig: olsv1alpha1.OLSSpec{
						DefaultModel:    "openai",
						DefaultProvider: "openai",
						LogLevel:        "INFO",
						ConversationCache: olsv1alpha1.ConversationCacheSpec{
							Type: "postgres",
						},
					},
				},
			}

			err = k8sClient.Create(ctx, olsConfig)
			Expect(err).NotTo(HaveOccurred())

			// Create a secret for the provider
			secret, _ = generateRandomSecret()
			secret.Name = "openai-secret-2"
			err = k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Triggering reconciliation")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: OLSConfigName,
				},
			}

			// The reconciliation should succeed and detect ROSA
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify ROSA detection was logged (we can't easily test the log output in unit tests,
			// but we can verify the reconciliation completed successfully)
			By("Verifying OLSConfig status is updated")
			updatedOLSConfig := &olsv1alpha1.OLSConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: OLSConfigName}, updatedOLSConfig)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle ROSA detection errors without failing reconciliation", func() {
			By("Testing ROSA detection call directly")
			// This tests the integration point without relying on actual Console objects
			isROSA, err := reconciler.detectROSAEnvironment(ctx)
			// Should not error when Console object doesn't exist
			Expect(err).NotTo(HaveOccurred())
			Expect(isROSA).To(BeFalse())
		})
	})
})

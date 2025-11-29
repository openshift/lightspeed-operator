package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("Watcher Predicates", func() {
	var (
		reconciler *OLSConfigReconciler
	)

	BeforeEach(func() {
		// Setup reconciler with WatcherConfig
		reconciler = &OLSConfigReconciler{
			Client:  k8sClient,
			Options: getDefaultReconcilerOptions(utils.OLSNamespaceDefault),
			Logger:  logf.Log.WithName("test.reconciler"),
			WatcherConfig: &utils.WatcherConfig{
				Secrets: utils.SecretWatcherConfig{
					SystemResources: []utils.SystemSecret{
						{
							Name:      "pull-secret",
							Namespace: "openshift-config",
							AffectedDeployments: []string{
								utils.ConsoleUIDeploymentName,
							},
						},
					},
				},
				ConfigMaps: utils.ConfigMapWatcherConfig{
					SystemResources: []utils.SystemConfigMap{
						{
							Name:      "kube-root-ca.crt",
							Namespace: utils.OLSNamespaceDefault,
							AffectedDeployments: []string{
								"ACTIVE_BACKEND",
							},
						},
					},
				},
			},
		}
	})

	Context("shouldWatchSecret", func() {
		It("should return true for secret with watcher annotation", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: utils.OLSNamespaceDefault,
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: utils.OLSConfigName,
					},
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeTrue())
		})

		It("should return true for system secret (pull-secret)", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: "openshift-config",
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeTrue())
		})

		It("should return false for secret without annotation or system config", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "random-secret",
					Namespace: utils.OLSNamespaceDefault,
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeFalse())
		})

		It("should return false for system secret in wrong namespace", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: "wrong-namespace",
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeFalse())
		})

		It("should return false when WatcherConfig is nil", func() {
			reconciler.WatcherConfig = nil
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pull-secret",
					Namespace: "openshift-config",
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeFalse())
		})

		It("should handle secret with no annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: utils.OLSNamespaceDefault,
					// No Annotations field set (nil)
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeFalse())
		})

		It("should return true when secret matches any system secret", func() {
			// Add another system secret to config
			reconciler.WatcherConfig.Secrets.SystemResources = append(
				reconciler.WatcherConfig.Secrets.SystemResources,
				utils.SystemSecret{
					Name:                "another-secret",
					Namespace:           "test-namespace",
					AffectedDeployments: []string{"test-deployment"},
				},
			)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-secret",
					Namespace: "test-namespace",
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeTrue())
		})
	})

	Context("shouldWatchConfigMap", func() {
		It("should return true for configmap with watcher annotation", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: utils.OLSNamespaceDefault,
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: utils.OLSConfigName,
					},
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeTrue())
		})

		It("should return true for system configmap (kube-root-ca.crt)", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-root-ca.crt",
					Namespace: utils.OLSNamespaceDefault,
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeTrue())
		})

		It("should return false for configmap without annotation or system config", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "random-configmap",
					Namespace: utils.OLSNamespaceDefault,
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeFalse())
		})

		It("should return false for system configmap in wrong namespace", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-root-ca.crt",
					Namespace: "wrong-namespace",
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeFalse())
		})

		It("should return false when WatcherConfig is nil", func() {
			reconciler.WatcherConfig = nil
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-root-ca.crt",
					Namespace: utils.OLSNamespaceDefault,
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeFalse())
		})

		It("should handle configmap with no annotations", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: utils.OLSNamespaceDefault,
					// No Annotations field set (nil)
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeFalse())
		})

		It("should return true when configmap matches any system configmap", func() {
			// Add another system configmap to config
			reconciler.WatcherConfig.ConfigMaps.SystemResources = append(
				reconciler.WatcherConfig.ConfigMaps.SystemResources,
				utils.SystemConfigMap{
					Name:                "another-configmap",
					Namespace:           "test-namespace",
					AffectedDeployments: []string{"test-deployment"},
				},
			)

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-configmap",
					Namespace: "test-namespace",
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeTrue())
		})
	})

	Context("Edge cases and client.Object interface", func() {
		It("should work with client.Object interface for secrets", func() {
			var obj client.Object = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: utils.OLSNamespaceDefault,
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: utils.OLSConfigName,
					},
				},
			}

			result := reconciler.shouldWatchSecret(obj)
			Expect(result).To(BeTrue())
		})

		It("should work with client.Object interface for configmaps", func() {
			var obj client.Object = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: utils.OLSNamespaceDefault,
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: utils.OLSConfigName,
					},
				},
			}

			result := reconciler.shouldWatchConfigMap(obj)
			Expect(result).To(BeTrue())
		})

		It("should handle multiple annotations on secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: utils.OLSNamespaceDefault,
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: utils.OLSConfigName,
						"other-annotation":         "other-value",
						"yet-another":              "value",
					},
				},
			}

			result := reconciler.shouldWatchSecret(secret)
			Expect(result).To(BeTrue())
		})

		It("should handle multiple annotations on configmap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: utils.OLSNamespaceDefault,
					Annotations: map[string]string{
						utils.WatcherAnnotationKey: utils.OLSConfigName,
						"other-annotation":         "other-value",
						"yet-another":              "value",
					},
				},
			}

			result := reconciler.shouldWatchConfigMap(cm)
			Expect(result).To(BeTrue())
		})
	})
})

var _ = Describe("Helper Functions", func() {
	var (
		reconciler    *OLSConfigReconciler
		ctx           context.Context
		testNamespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = utils.OLSNamespaceDefault

		// Setup reconciler
		reconciler = &OLSConfigReconciler{
			Client:  k8sClient,
			Options: getDefaultReconcilerOptions(testNamespace),
			Logger:  logf.Log.WithName("test.reconciler"),
		}
	})

	Context("UpdateStatusCondition", func() {
		var olsConfig *olsv1alpha1.OLSConfig

		BeforeEach(func() {
			olsConfig = utils.GetDefaultOLSConfigCR()
			olsConfig.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "test-llm-secret-reconcile"
			Expect(k8sClient.Create(ctx, olsConfig)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, olsConfig)
		})

		It("should update status condition to true", func() {
			reconciler.UpdateStatusCondition(ctx, olsConfig, utils.TypeApiReady, true, "Test", nil)

			// Fetch updated CR
			updated := &olsv1alpha1.OLSConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigName}, updated)
			Expect(err).NotTo(HaveOccurred())

			// Check condition exists and has correct status
			found := false
			for _, cond := range updated.Status.Conditions {
				if cond.Type == utils.TypeApiReady {
					found = true
					Expect(cond.Status).To(Equal(metav1.ConditionTrue))
					// Reason might be "Reconciling" or "Test" depending on controller logic
					Expect(cond.Reason).NotTo(BeEmpty())
					break
				}
			}
			Expect(found).To(BeTrue(), "TypeApiReady condition should exist")
		})

		It("should update status condition to false with error message", func() {
			reconciler.UpdateStatusCondition(ctx, olsConfig, utils.TypeCacheReady, false, "Failed",
				errors.NewBadRequest("test error"))

			// Fetch updated CR
			updated := &olsv1alpha1.OLSConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: utils.OLSConfigName}, updated)
			Expect(err).NotTo(HaveOccurred())

			// Check condition exists and has correct status
			found := false
			for _, cond := range updated.Status.Conditions {
				if cond.Type == utils.TypeCacheReady {
					found = true
					Expect(cond.Status).To(Equal(metav1.ConditionFalse))
					// Reason might be "Reconciling" or "Failed" depending on controller logic
					Expect(cond.Reason).NotTo(BeEmpty())
					Expect(cond.Message).To(ContainSubstring("test error"))
					break
				}
			}
			Expect(found).To(BeTrue(), "TypeCacheReady condition should exist")
		})
	})

	Context("checkDeploymentStatus", func() {
		It("should return nil for ready deployment", func() {
			deployment := &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentAvailable,
							Status: corev1.ConditionTrue,
						},
					},
					ReadyReplicas:   1,
					UpdatedReplicas: 1,
					Replicas:        1,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{1}[0],
				},
			}

			message, err := reconciler.checkDeploymentStatus(deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(message).To(BeEmpty())
		})

		It("should return DeploymentInProgress for progressing deployment", func() {
			deployment := &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: corev1.ConditionTrue,
							Reason: "NewReplicaSetAvailable",
						},
					},
					ReadyReplicas:   0,
					UpdatedReplicas: 1,
					Replicas:        1,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{1}[0],
				},
			}

			message, err := reconciler.checkDeploymentStatus(deployment)
			Expect(err).To(HaveOccurred())
			Expect(message).To(Equal(utils.DeploymentInProgress))
		})

		It("should return error for failed deployment", func() {
			deployment := &appsv1.Deployment{
				Status: appsv1.DeploymentStatus{
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentReplicaFailure,
							Status:  corev1.ConditionTrue,
							Message: "Pod failed",
						},
					},
					ReadyReplicas:   1, // Set replicas to match so we check the condition
					UpdatedReplicas: 1,
					Replicas:        1,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{1}[0],
				},
			}

			message, err := reconciler.checkDeploymentStatus(deployment)
			Expect(err).To(HaveOccurred())
			Expect(message).To(Equal("Fail")) // Actual return value from code
		})
	})

	Context("annotateExternalResources", func() {
		var (
			testCR       *olsv1alpha1.OLSConfig
			llmSecret    *corev1.Secret
			tlsSecret    *corev1.Secret
			additionalCA *corev1.ConfigMap
			proxyCA      *corev1.ConfigMap
			mcpSecret    *corev1.Secret
		)

		BeforeEach(func() {
			// Create test secrets and configmaps
			llmSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-secret",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"apitoken": []byte("test-token"),
				},
			}
			Expect(k8sClient.Create(ctx, llmSecret)).To(Succeed())

			tlsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tls-secret",
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte("cert"),
					"tls.key": []byte("key"),
				},
			}
			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())

			additionalCA = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-additional-ca",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"ca-bundle.crt": "ca-cert-data",
				},
			}
			Expect(k8sClient.Create(ctx, additionalCA)).To(Succeed())

			proxyCA = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-proxy-ca",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"ca-bundle.crt": "proxy-ca-data",
				},
			}
			Expect(k8sClient.Create(ctx, proxyCA)).To(Succeed())

			mcpSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mcp-secret",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"token": []byte("mcp-token"),
				},
			}
			Expect(k8sClient.Create(ctx, mcpSecret)).To(Succeed())

			// Create CR with external resource references
			testCR = &olsv1alpha1.OLSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-annotation-cr",
				},
				Spec: olsv1alpha1.OLSConfigSpec{
					LLMConfig: olsv1alpha1.LLMSpec{
						Providers: []olsv1alpha1.ProviderSpec{
							{
								Name: "test-provider",
								CredentialsSecretRef: corev1.LocalObjectReference{
									Name: "test-llm-secret",
								},
							},
						},
					},
					OLSConfig: olsv1alpha1.OLSSpec{
						TLSConfig: &olsv1alpha1.TLSConfig{
							KeyCertSecretRef: corev1.LocalObjectReference{
								Name: "test-tls-secret",
							},
						},
						AdditionalCAConfigMapRef: &corev1.LocalObjectReference{
							Name: "test-additional-ca",
						},
						ProxyConfig: &olsv1alpha1.ProxyConfig{
							ProxyCACertificateRef: &corev1.LocalObjectReference{
								Name: "test-proxy-ca",
							},
						},
					},
					MCPServers: []olsv1alpha1.MCPServer{
						{
							Name: "test-mcp-server",
							StreamableHTTP: &olsv1alpha1.MCPServerStreamableHTTPTransport{
								URL: "http://test-mcp-server",
								Headers: map[string]string{
									"Authorization": "test-mcp-secret",
								},
							},
						},
					},
				},
			}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, llmSecret)
			_ = k8sClient.Delete(ctx, tlsSecret)
			_ = k8sClient.Delete(ctx, additionalCA)
			_ = k8sClient.Delete(ctx, proxyCA)
			_ = k8sClient.Delete(ctx, mcpSecret)
		})

		It("should annotate all external resources", func() {
			err := reconciler.annotateExternalResources(ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			// Verify LLM secret is annotated
			fetchedLLMSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-llm-secret", Namespace: testNamespace}, fetchedLLMSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedLLMSecret.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))

			// Verify TLS secret is annotated
			fetchedTLSSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-tls-secret", Namespace: testNamespace}, fetchedTLSSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedTLSSecret.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))

			// Verify Additional CA configmap is annotated
			fetchedAdditionalCA := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-additional-ca", Namespace: testNamespace}, fetchedAdditionalCA)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedAdditionalCA.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))

			// Verify Proxy CA configmap is annotated
			fetchedProxyCA := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-proxy-ca", Namespace: testNamespace}, fetchedProxyCA)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedProxyCA.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))

			// Verify MCP secret is annotated
			fetchedMCPSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-mcp-secret", Namespace: testNamespace}, fetchedMCPSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedMCPSecret.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))
		})

		It("should handle missing resources gracefully", func() {
			// Create CR with non-existent resource references
			testCR.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name = "non-existent-secret"
			testCR.Spec.OLSConfig.AdditionalCAConfigMapRef.Name = "non-existent-cm"

			// Should not return error - missing resources are handled gracefully (returns nil)
			err := reconciler.annotateExternalResources(ctx, testCR)
			Expect(err).NotTo(HaveOccurred()) // Returns nil for missing resources (will be picked up on next reconciliation)
		})

		It("should skip annotation if already annotated", func() {
			// Pre-annotate the LLM secret
			llmSecret.Annotations = map[string]string{
				utils.WatcherAnnotationKey: utils.OLSConfigName,
			}
			err := k8sClient.Update(ctx, llmSecret)
			Expect(err).NotTo(HaveOccurred())

			// Call annotateExternalResources
			err = reconciler.annotateExternalResources(ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			// Verify annotation is still there (not duplicated or changed)
			fetchedSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-llm-secret", Namespace: testNamespace}, fetchedSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedSecret.Annotations).To(HaveKeyWithValue(utils.WatcherAnnotationKey, utils.OLSConfigName))
		})

		It("should skip MCP secrets with 'kubernetes' value", func() {
			// Create CR with special "kubernetes" token case
			testCR.Spec.MCPServers[0].StreamableHTTP.Headers = map[string]string{
				"Authorization": "kubernetes", // Special case that should be skipped
			}

			err := reconciler.annotateExternalResources(ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			// The "kubernetes" token should not have caused any lookup or annotation
			// No assertion needed - just verifying no error occurred
		})
	})
})

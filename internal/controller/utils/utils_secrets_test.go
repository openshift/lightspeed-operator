package utils

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Secret Functions", func() {
	var testClient client.Client
	var testSecret *corev1.Secret
	var ctx context.Context

	BeforeEach(func() {
		testClient = k8sClient
		ctx = context.Background() // Use Background context for K8s operations
		testSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-utils",
				Namespace: OLSNamespaceDefault,
			},
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
				"apitoken": []byte("token456"),
			},
		}
		Expect(testClient.Create(ctx, testSecret)).To(Succeed())
	})

	AfterEach(func() {
		_ = testClient.Delete(ctx, testSecret)
	})

	Describe("GetSecretContent", func() {
		It("should retrieve specified fields from secret", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{"username", "password"}

			result, err := GetSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result["username"]).To(Equal("admin"))
			Expect(result["password"]).To(Equal("secret123"))
		})

		It("should return error for non-existent secret", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{"username"}

			_, err := GetSecretContent(testClient, "non-existent", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret not found"))
		})

		It("should return error for missing field", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{"missing-field"}

			_, err := GetSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not present in the secret"))
		})

		It("should handle empty field list", func() {
			foundSecret := &corev1.Secret{}
			fields := []string{}

			result, err := GetSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, fields, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Describe("GetAllSecretContent", func() {
		It("should retrieve all fields from secret", func() {
			foundSecret := &corev1.Secret{}

			result, err := GetAllSecretContent(testClient, "test-secret-utils", OLSNamespaceDefault, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(3))
			Expect(result["username"]).To(Equal("admin"))
			Expect(result["password"]).To(Equal("secret123"))
			Expect(result["apitoken"]).To(Equal("token456"))
		})

		It("should return error for non-existent secret", func() {
			foundSecret := &corev1.Secret{}

			_, err := GetAllSecretContent(testClient, "non-existent", OLSNamespaceDefault, foundSecret)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret not found"))
		})

		It("should handle empty secret", func() {
			emptySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-secret",
					Namespace: OLSNamespaceDefault,
				},
				Data: map[string][]byte{},
			}
			Expect(testClient.Create(ctx, emptySecret)).To(Succeed())
			defer testClient.Delete(ctx, emptySecret)

			foundSecret := &corev1.Secret{}
			result, err := GetAllSecretContent(testClient, "empty-secret", OLSNamespaceDefault, foundSecret)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Describe("AnnotateSecretWatcher", func() {
		It("should add watcher annotation to secret with nil annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations).NotTo(BeNil())
			Expect(annotations).To(HaveLen(1))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should add watcher annotation to secret with existing annotations", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"existing-key": "existing-value",
					},
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations).To(HaveLen(2))
			Expect(annotations["existing-key"]).To(Equal("existing-value"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should overwrite existing watcher annotation", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						WatcherAnnotationKey: "old-value",
					},
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should preserve other annotations when adding watcher annotation", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"annotation1": "value1",
						"annotation2": "value2",
						"annotation3": "value3",
					},
				},
			}

			AnnotateSecretWatcher(secret)

			annotations := secret.GetAnnotations()
			Expect(annotations).To(HaveLen(4))
			Expect(annotations["annotation1"]).To(Equal("value1"))
			Expect(annotations["annotation2"]).To(Equal("value2"))
			Expect(annotations["annotation3"]).To(Equal("value3"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})
	})

	Describe("AnnotateConfigMapWatcher", func() {
		It("should add watcher annotation to configmap with nil annotations", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations).NotTo(BeNil())
			Expect(annotations).To(HaveLen(1))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should add watcher annotation to configmap with existing annotations", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"existing-key": "existing-value",
					},
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations).To(HaveLen(2))
			Expect(annotations["existing-key"]).To(Equal("existing-value"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should overwrite existing watcher annotation", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						WatcherAnnotationKey: "old-value",
					},
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})

		It("should preserve other annotations when adding watcher annotation", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"annotation1": "value1",
						"annotation2": "value2",
						"annotation3": "value3",
					},
				},
			}

			AnnotateConfigMapWatcher(cm)

			annotations := cm.GetAnnotations()
			Expect(annotations).To(HaveLen(4))
			Expect(annotations["annotation1"]).To(Equal("value1"))
			Expect(annotations["annotation2"]).To(Equal("value2"))
			Expect(annotations["annotation3"]).To(Equal("value3"))
			Expect(annotations[WatcherAnnotationKey]).To(Equal(OLSConfigName))
		})
	})
})

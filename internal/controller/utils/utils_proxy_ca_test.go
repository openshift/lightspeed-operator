package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("GetProxyCACertHash", func() {
	var (
		ctx               context.Context
		testReconciler    *TestReconciler
		testOLSConfig     *olsv1alpha1.OLSConfig
		testConfigMap     *corev1.ConfigMap
		testCertContent   string
		testConfigMapName string
		testCertKey       string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testCertContent = "-----BEGIN CERTIFICATE-----\nMIIDXTCCAkWgAwIBAgIJAKZ7VZ\n-----END CERTIFICATE-----"
		testConfigMapName = "test-proxy-ca-cm"
		testCertKey = ProxyCACertFileName

		testReconciler = NewTestReconciler(k8sClient, logf.Log, scheme.Scheme, OLSNamespaceDefault)

		testOLSConfig = &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-olsconfig",
			},
			Spec: olsv1alpha1.OLSConfigSpec{
				OLSConfig: olsv1alpha1.OLSSpec{
					ProxyConfig: &olsv1alpha1.ProxyConfig{
						ProxyURL: "https://proxy.example.com:8443",
						ProxyCACertificateRef: &olsv1alpha1.ProxyCACertConfigMapRef{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testConfigMapName,
							},
							Key: testCertKey,
						},
					},
				},
			},
		}

		testConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testConfigMapName,
				Namespace: OLSNamespaceDefault,
			},
			Data: map[string]string{
				testCertKey: testCertContent,
			},
		}
	})

	AfterEach(func() {
		if testConfigMap != nil {
			_ = k8sClient.Delete(ctx, testConfigMap)
		}
	})

	Context("when proxy CA is configured", func() {
		It("should return the SHA256 hash of the certificate content", func() {
			Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())

			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).NotTo(HaveOccurred())

			expectedHash := sha256.Sum256([]byte(testCertContent))
			expectedHashString := hex.EncodeToString(expectedHash[:])

			Expect(hash).To(Equal(expectedHashString))
		})

		It("should return consistent hash for same certificate content", func() {
			Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())

			hash1, err1 := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)
			hash2, err2 := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).To(Equal(hash2))
			Expect(hash1).NotTo(BeEmpty())
		})

		It("should return different hash when certificate content changes", func() {
			Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())

			hash1, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)
			Expect(err).NotTo(HaveOccurred())

			testConfigMap.Data[testCertKey] = "-----BEGIN CERTIFICATE-----\nDIFFERENT_CERT\n-----END CERTIFICATE-----"
			Expect(k8sClient.Update(ctx, testConfigMap)).To(Succeed())

			hash2, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(hash1).NotTo(Equal(hash2))
			Expect(hash1).NotTo(BeEmpty())
			Expect(hash2).NotTo(BeEmpty())
		})

		It("should return error when ConfigMap does not exist", func() {
			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(hash).To(BeEmpty())
		})

		It("should return error when certificate key is missing from ConfigMap", func() {
			testConfigMap.Data = map[string]string{
				"wrong-key": testCertContent,
			}
			Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())

			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in ConfigMap"))
			Expect(hash).To(BeEmpty())
		})

		It("should use custom key when specified in ProxyCACertificateRef", func() {
			customKey := "custom-ca.crt"
			testOLSConfig.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Key = customKey
			testConfigMap.Data = map[string]string{
				customKey: testCertContent,
			}
			Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())

			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(hash).NotTo(BeEmpty())

			expectedHash := sha256.Sum256([]byte(testCertContent))
			expectedHashString := hex.EncodeToString(expectedHash[:])
			Expect(hash).To(Equal(expectedHashString))
		})
	})

	Context("when proxy is not configured", func() {
		It("should return empty string when ProxyConfig is nil", func() {
			testOLSConfig.Spec.OLSConfig.ProxyConfig = nil

			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("should return empty string when ProxyCACertificateRef is nil", func() {
			testOLSConfig.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef = nil

			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("should return empty string when ConfigMap name is empty", func() {
			testOLSConfig.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name = ""

			hash, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})
	})

	Context("hash validation for reconciliation behavior", func() {
		It("should detect actual certificate changes (not just ResourceVersion changes)", func() {
			// This validates the main purpose: deployments only restart when certificate
			// content changes, not when ConfigMap ResourceVersion changes (e.g., service-ca updates)

			Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())

			hash1, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash1).NotTo(BeEmpty())

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: testConfigMapName, Namespace: OLSNamespaceDefault}, cm)).To(Succeed())

			if cm.Annotations == nil {
				cm.Annotations = make(map[string]string)
			}
			cm.Annotations["test-annotation"] = "test-value"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			hash2, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)
			Expect(err).NotTo(HaveOccurred())

			// Hash should be identical because certificate content didn't change
			Expect(hash2).To(Equal(hash1))

			cm.Data[testCertKey] = "-----BEGIN CERTIFICATE-----\nNEW_CERT\n-----END CERTIFICATE-----"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			hash3, err := GetProxyCACertHash(testReconciler, ctx, testOLSConfig)
			Expect(err).NotTo(HaveOccurred())

			// Hash should be different because certificate content changed
			Expect(hash3).NotTo(Equal(hash1))
			Expect(hash3).NotTo(Equal(hash2))
		})
	})
})

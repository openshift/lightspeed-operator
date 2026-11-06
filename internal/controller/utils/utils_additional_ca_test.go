package utils

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("GetAdditionalCAHash", func() {
	var (
		ctx            context.Context
		testReconciler *TestReconciler
		testOLSConfig  *olsv1alpha1.OLSConfig
		testConfigMap  *corev1.ConfigMap
	)

	BeforeEach(func() {
		ctx = context.Background()
		testReconciler = NewTestReconciler(k8sClient, logf.Log, scheme.Scheme, OLSNamespaceDefault)
		testOLSConfig = &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-olsconfig",
			},
		}
		testConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-additional-ca-cm",
				Namespace: OLSNamespaceDefault,
			},
			Data: map[string]string{
				"ca-1.crt": TestCACert,
				"ca-2.crt": TestCACert,
			},
		}
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, testConfigMap)
	})

	It("should return an empty hash when no additional CA is configured", func() {
		hash, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)

		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(BeEmpty())
	})

	It("should return an empty hash when the configmap name is empty", func() {
		testOLSConfig.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{}

		hash, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)

		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(BeEmpty())
	})

	It("should return a consistent hash for the same content", func() {
		Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())
		testOLSConfig.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{Name: testConfigMap.Name}

		hash1, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)
		Expect(err).NotTo(HaveOccurred())
		hash2, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(hash1).NotTo(BeEmpty())
		Expect(hash1).To(Equal(hash2))
	})

	It("should return a different hash when certificate content changes without key changes", func() {
		Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())
		testOLSConfig.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{Name: testConfigMap.Name}

		hash1, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)
		Expect(err).NotTo(HaveOccurred())

		testConfigMap.Data["ca-1.crt"] = TestCACert + "\n" + TestCACert
		Expect(k8sClient.Update(ctx, testConfigMap)).To(Succeed())

		hash2, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(hash1).NotTo(Equal(hash2))
	})

	It("should return a different hash when a key is added", func() {
		Expect(k8sClient.Create(ctx, testConfigMap)).To(Succeed())
		testOLSConfig.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{Name: testConfigMap.Name}

		hash1, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)
		Expect(err).NotTo(HaveOccurred())

		testConfigMap.Data["ca-3.crt"] = TestCACert
		Expect(k8sClient.Update(ctx, testConfigMap)).To(Succeed())

		hash2, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(hash1).NotTo(Equal(hash2))
	})

	It("should return a NotFound error when the configmap does not exist", func() {
		testOLSConfig.Spec.OLSConfig.AdditionalCAConfigMapRef = &corev1.LocalObjectReference{Name: testConfigMap.Name}

		hash, err := GetAdditionalCAHash(testReconciler, ctx, testOLSConfig)

		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		Expect(hash).To(BeEmpty())
	})
})

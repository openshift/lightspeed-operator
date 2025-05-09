package e2e

import (
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "End-to-End Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	err := olsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	conditionTimeout := DefaultPollTimeout
	conditionTimeoutSecondsStr := os.Getenv(ConditionTimeoutEnvVar)
	if conditionTimeoutSecondsStr != "" {
		conditionTimeoutSeconds, err := strconv.Atoi(conditionTimeoutSecondsStr)
		Expect(err).NotTo(HaveOccurred())
		Expect(conditionTimeoutSeconds).To(BeNumerically(">", 0))
		conditionTimeout = time.Duration(conditionTimeoutSeconds) * time.Second
	}

	client, err := GetClient(&ClientOptions{
		conditionCheckTimeout: conditionTimeout,
	})
	if err != nil {
		Fail("Failed to create client")
	}

	By("Check the operator is ready")
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OperatorDeploymentName,
			Namespace: OLSNameSpace,
		},
	}
	err = client.WaitForDeploymentRollout(deployment)
	if err != nil && errors.IsNotFound(err) {
		Fail("Operator deployment not found")
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	By("Create 2 LLM token secrets")
	secret, err := generateLLMTokenSecret(LLMTokenFirstSecretName)
	Expect(err).NotTo(HaveOccurred())
	err = client.Create(secret)
	if errors.IsAlreadyExists(err) {
		err = client.Update(secret)
	}
	Expect(err).NotTo(HaveOccurred())

	secret, err = generateLLMTokenSecret(LLMTokenSecondSecretName)
	Expect(err).NotTo(HaveOccurred())
	err = client.Create(secret)
	if errors.IsAlreadyExists(err) {
		err = client.Update(secret)
	}
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	client, err := GetClient(nil)
	if err != nil {
		Fail("Failed to create client")
	}
	err = mustGather("suite")
	Expect(err).NotTo(HaveOccurred())
	By("Delete the 2 LLM token Secrets")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LLMTokenFirstSecretName,
			Namespace: OLSNameSpace,
		},
	}
	err = client.Delete(secret)
	if !errors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LLMTokenSecondSecretName,
			Namespace: OLSNameSpace,
		},
	}
	err = client.Delete(secret)
	if !errors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
	}
})

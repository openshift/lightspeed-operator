package e2e

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TLS activation", Ordered, func() {
	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var forwardHost string

	BeforeAll(func() {
		client, err = GetClient()
		Expect(err).NotTo(HaveOccurred())
		By("Create 1 LLM token secrets")
		secret, err := generateLLMTokenSecret(LLMTokenFirstSecretName)
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(secret)
		if errors.IsAlreadyExists(err) {
			err = client.Update(secret)
		}
		Expect(err).NotTo(HaveOccurred())

		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())

		By("wait for application server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("forwarding the HTTPS port to a local port")
		var cleanUp func()
		forwardHost, cleanUp, err = client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

	})

	AfterAll(func() {
		for _, cleanUp := range cleanUpFuncs {
			cleanUp()
		}

		client, err = GetClient()
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		Expect(cr).NotTo(BeNil())
		err = client.Delete(cr)
		Expect(err).NotTo(HaveOccurred())

		By("Delete the 1 LLM token Secret")
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

	})

	It("should activate TLS on service HTTPS port", func() {

		By("Wait for the application service created")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())

		By("check the secret holding TLS ceritificates is created")
		secretName, ok := service.ObjectMeta.Annotations[serviceAnnotationKeyTLSSecret]
		Expect(ok).To(BeTrue())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForSecretCreated(secret)
		Expect(err).NotTo(HaveOccurred())

		By("check the deployment has the certificate secret mounted")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(deployment)
		Expect(err).NotTo(HaveOccurred())
		var secretVolumeDefaultMode = int32(420)
		Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
			Name: "secret-" + AppServerTLSSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &secretVolumeDefaultMode,
				},
			},
		}))

		By("check HTTPS Get on /metrics endpoint")
		const inClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"
		certificate, ok := secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())

		httpsClient := NewHTTPSClient(forwardHost, inClusterHost, certificate)

		err = httpsClient.waitForHTTPSGetStatus("/metrics", http.StatusOK)
		Expect(err).NotTo(HaveOccurred())

		By("check HTTPS Post on /v1/query endpoint")
		reqBody := []byte(`{"query": "write a deployment yaml for the mongodb image"}`)
		var resp *http.Response
		resp, err = httpsClient.PostJson("/v1/query", reqBody)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).NotTo(BeEmpty())

	})
})

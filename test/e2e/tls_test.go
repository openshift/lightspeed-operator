package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = Describe("TLS activation", Ordered, func() {
	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client

	BeforeAll(func() {
		client, err = GetClient()
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		client, err = GetClient()
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		Expect(cr).NotTo(BeNil())
		err = client.Delete(cr)
		Expect(err).NotTo(HaveOccurred())

	})

	It("should activate TLS on service HTTPS port", func() {
		By("wait for application server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

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

		By("check the deploy has the certificate secret mounted")
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

		By("check HTTPS connection on forwarded port")
		const inClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"
		certificate, ok := secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(certificate)

		var rt http.RoundTripper = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caCertPool,
				ServerName: inClusterHost,
				// InsecureSkipVerify: true,
			},
		}

		var resp *http.Response
		var lastErr error
		err = wait.PollUntilContextTimeout(client.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
			By("Forward the HTTPS port to a local port")
			var cleanUp func()
			var forwardHost string
			forwardHost, cleanUp, err = client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
			Expect(err).NotTo(HaveOccurred())
			defer cleanUp()

			By("send HTTPS request to forwared port")
			u, err := url.Parse("/metrics")
			Expect(err).NotTo(HaveOccurred())
			u.Host = forwardHost
			u.Scheme = "https"
			var body []byte = make([]byte, 1024)
			req, err := http.NewRequest(http.MethodGet, u.String(), bytes.NewBuffer(body))
			Expect(err).NotTo(HaveOccurred())
			req.Host = inClusterHost
			resp, lastErr = (&http.Client{Transport: rt}).Do(req)
			if lastErr != nil {
				return false, nil
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
				return false, nil
			}
			return true, nil
		})
		Expect(lastErr).NotTo(HaveOccurred())
		Expect(err).NotTo(HaveOccurred())

	})
})

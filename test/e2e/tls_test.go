package e2e

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TLS activation - application", Ordered, func() {
	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	const testSAName = "test-sa"
	const testSAOutsiderName = "test-sa-outsider"
	const queryAccessClusterRole = "lightspeed-operator-query-access"
	const appMetricsAccessClusterRole = "lightspeed-operator-ols-metrics-reader"
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var forwardHost string
	var saToken string
	var saOutsiderToken string

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())

		var cleanUp func()

		By("create a service account for OLS user")
		cleanUp, err := client.CreateServiceAccount(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("Create a service account for an outsider")
		cleanUp, err = client.CreateServiceAccount(OLSNameSpace, testSAOutsiderName)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("create a role binding for OLS user accessing query API")
		cleanUp, err = client.CreateClusterRoleBinding(OLSNameSpace, testSAName, queryAccessClusterRole)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("create a role binding for OLS user accessing application metrics")
		cleanUp, err = client.CreateClusterRoleBinding(OLSNameSpace, testSAName, appMetricsAccessClusterRole)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("fetch the service account tokens")
		saToken, err = client.GetServiceAccountToken(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())
		saOutsiderToken, err = client.GetServiceAccountToken(OLSNameSpace, testSAOutsiderName)
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

		forwardHost, cleanUp, err = client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)
	})

	AfterAll(func() {
		err = mustGather("tls_test")
		Expect(err).NotTo(HaveOccurred())
		for _, cleanUp := range cleanUpFuncs {
			cleanUp()
		}

		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		Expect(cr).NotTo(BeNil())
		err = client.Delete(cr)
		Expect(err).NotTo(HaveOccurred())
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

		By("check the secret holding TLS certificates is created")
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
		secretVolumeDefaultMode := int32(420)
		Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
			Name: "secret-" + AppServerTLSSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &secretVolumeDefaultMode,
				},
			},
		}))

		By("check HTTPS Get on /metrics endpoint by OLS user")
		const inClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"
		certificate, ok := secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
		httpsClient := NewHTTPSClient(forwardHost, inClusterHost, certificate, nil, nil)
		authHeader := map[string]string{"Authorization": "Bearer " + saToken}
		err = httpsClient.waitForHTTPSGetStatus("/metrics", http.StatusOK, authHeader)
		Expect(err).NotTo(HaveOccurred())

		By("check HTTPS Post on /v1/query endpoint by OLS user")
		reqBody := []byte(`{"query": "write a deployment yaml for the mongodb image"}`)
		var resp *http.Response
		resp, err = httpsClient.PostJson("/v1/query", reqBody, authHeader)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).NotTo(BeEmpty())

		By("check HTTPS Get on /metrics endpoint by an outsider")
		authHeader = map[string]string{"Authorization": "Bearer " + saOutsiderToken}
		err = httpsClient.waitForHTTPSGetStatus("/metrics", http.StatusForbidden, authHeader)
		Expect(err).NotTo(HaveOccurred())

		By("check HTTPS Post on /v1/query endpoint by an outsider")
		err = httpsClient.waitForHTTPSPostStatus("/v1/query", reqBody, http.StatusForbidden, authHeader)
		Expect(err).NotTo(HaveOccurred())

	})
})

var _ = Describe("TLS activation - operator", Ordered, func() {
	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	const operatorServiceName = "lightspeed-operator-controller-manager-service"
	const operatorServicePort = 8443
	const operatorDeploymentName = "lightspeed-operator-controller-manager"
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var forwardHost string

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		By("wait for operator deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("forwarding the HTTPS port to a local port")
		var cleanUp func()
		forwardHost, cleanUp, err = client.ForwardPort(operatorServiceName, OLSNameSpace, operatorServicePort)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

	})

	AfterAll(func() {
		for _, cleanUp := range cleanUpFuncs {
			cleanUp()
		}
	})

	It("should activate TLS on service HTTPS port", func() {
		By("Wait for the operator service created")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorServiceName,
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
				Name:      "lightspeed-operator-controller-manager",
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(deployment)
		Expect(err).NotTo(HaveOccurred())
		var secretVolumeDefaultMode = int32(420)
		Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
			Name: "controller-manager-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &secretVolumeDefaultMode,
				},
			},
		}))

		By("check HTTPS Get on /metrics endpoint")
		const inClusterHost = "lightspeed-operator-controller-manager-service.openshift-lightspeed.svc"
		prometheusTLSSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics-client-certs",
				Namespace: "openshift-monitoring",
			},
		}
		err = client.Get(&prometheusTLSSecret)
		Expect(err).NotTo(HaveOccurred())
		clientCert, ok := prometheusTLSSecret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
		clientKey, ok := prometheusTLSSecret.Data["tls.key"]
		Expect(ok).To(BeTrue())
		prometheusTLSCABundle := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "serving-certs-ca-bundle",
				Namespace: "openshift-monitoring",
			},
		}
		err = client.Get(&prometheusTLSCABundle)
		Expect(err).NotTo(HaveOccurred())
		caCert, ok := prometheusTLSCABundle.Data["service-ca.crt"]
		Expect(ok).To(BeTrue())

		httpsClient := NewHTTPSClient(forwardHost, inClusterHost, []byte(caCert), clientCert, clientKey)

		err = httpsClient.waitForHTTPSGetStatus("/metrics", http.StatusOK)
		Expect(err).NotTo(HaveOccurred())

	})

})

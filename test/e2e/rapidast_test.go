package e2e

import (
	"fmt"
	"io"
	"net/http"

	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TLS activation - application", Ordered, Label("Rapidast"), func() {
	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	const testSAName = "test-sa"
	const testSAOutsiderName = "test-sa-outsider"
	const queryAccessClusterRole = "lightspeed-operator-query-access"
	const appMetricsAccessClusterRole = "lightspeed-operator-ols-metrics-reader"
	const olsRouteName = "ols-route"
	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var forwardHost string
	var saToken string

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

		By("create a role binding for OLS user accessing query API")
		cleanUp, err = client.CreateClusterRoleBinding(OLSNameSpace, testSAName, queryAccessClusterRole)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("fetch the service account tokens")
		saToken, err = client.GetServiceAccountToken(OLSNameSpace, testSAName)
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

		By("check HTTPS Post on /v1/query endpoint by OLS user")
		const inClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"
		certificate, ok := secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
		httpsClient := NewHTTPSClient(forwardHost, inClusterHost, certificate, nil, nil)
		authHeader := map[string]string{"Authorization": "Bearer " + saToken}
		reqBody := []byte(`{"query": "write a deployment yaml for the mongodb image"}`)
		var resp *http.Response
		resp, err = httpsClient.PostJson("/v1/query", reqBody, authHeader)
		fmt.Println("httpsClient.PostJson", authHeader)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		fmt.Println(string(body))

		Expect(err).NotTo(HaveOccurred())
		Expect(body).NotTo(BeEmpty())

		// Get console url from Environment
		consoleURL := os.Getenv("CONSOLE_URL")
		// TODO - Update the consoleURL after testing in konflux
		consoleURL = "https://console-openshift-console.apps.sno-416-.dev.red-chesterfield.com"
		Expect(consoleURL).To(ContainSubstring(".apps"))
		//create ols application host url
		//trim everything before the first dot and append it to ols-route
		index := strings.Index(consoleURL, ".apps")
		hostURL := olsRouteName + consoleURL[index:]

		By("Create a route for the OLS application")
		_, err = client.createRoute(olsRouteName, OLSNameSpace, hostURL)
		Expect(err).NotTo(HaveOccurred())

		By("Update ols-rapidast-config-updated.yaml with host and token")
		config_content, err := os.ReadFile("../../ols-rapidast-config.yaml")
		if err != nil {
			fmt.Println("Error reading file:", err)
			return
		}

		newContent := strings.Replace(string(config_content), "$HOST", hostURL, -1)
		newContent = strings.Replace(newContent, "$BEARER_TOKEN", saToken, -1)
		err = os.WriteFile("../../ols-rapidast-config-updated.yaml", []byte(newContent), 0644)
		if err != nil {
			fmt.Println("Error writing file:", err)
			return
		}

	})
})

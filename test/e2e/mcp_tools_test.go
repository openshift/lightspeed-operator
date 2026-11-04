package e2e

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MCP tool exposure", Label("MCP"), Ordered, FlakeAttempts(5), func() {
	var client *Client
	var cr *olsv1alpha1.OLSConfig
	var err error

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		By("Creating an OLSConfig CR with introspection enabled")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
		Expect(client.Create(cr)).To(Succeed())

		By("Waiting for the openshift-mcp-server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.OpenShiftMCPServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		Eventually(func() error {
			return client.Get(deployment)
		}, DefaultPollTimeout, 5*time.Second).Should(Succeed())
		Expect(client.WaitForDeploymentRollout(deployment)).To(Succeed())
	})

	AfterAll(func() {
		err = mustGather("mcp_tools_test")
		Expect(err).NotTo(HaveOccurred())
		Expect(cr).NotTo(BeNil())
		By("Deleting the OLSConfig CR and waiting for cleanup")
		Expect(client.DeleteAndWait(cr, 3*time.Minute)).To(Succeed())
	})

	It("should advertise tools that the tool filters would hide", func() {
		By("Port-forwarding to the OpenShift MCP server")
		host, stopForward, err := client.ForwardPort(utils.OpenShiftMCPServerServiceName, OLSNameSpace, utils.OpenShiftMCPServerHTTPSPort)
		Expect(err).NotTo(HaveOccurred())
		defer stopForward()

		// The Service uses a service-ca certificate the test does not trust.
		httpClient := &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				// #nosec G402 -- cluster-internal service-ca certificate.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		var tools []mcpTool
		Eventually(func() error {
			tools, err = newMCPServerClient(httpClient, "https://"+host).listTools()
			return err
		}, time.Minute, 5*time.Second).Should(Succeed())

		destructive, nonReadOnly := toolHintCounts(tools)
		_, _ = fmt.Fprintf(GinkgoWriter, "openshift-mcp-server advertised %d tools (%d destructiveHint=true, %d readOnlyHint=false)\n", len(tools), destructive, nonReadOnly)

		// Assert on the tools annotations, not on an inventory: names and counts
		// change with every openshift-mcp-server bump, the hint semantics do not.
		Expect(nonReadOnly).To(BeNumerically(">", 0),
			"every advertised tool is readOnlyHint=true, so read_only is filtering write tools again; tools: %v",
			toolNames(tools))
		Expect(destructive).To(BeNumerically(">", 0),
			"no advertised tool is destructiveHint=true, so disable_destructive is filtering write tools again; tools: %v",
			toolNames(tools))
	})
})

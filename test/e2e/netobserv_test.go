package e2e

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/ocpmcp"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// This suite observes whatever NetObserv API/backend state the cluster already
// has. Lifecycle mutations normally belong to envtest; the separately opted-in
// dedicated-cluster fixture never deletes or replaces an existing product CRD.
var _ = Describe("NetObserv default enablement", Ordered, Serial, Label("NetObserv"), func() {
	var c *Client
	var cr *olsv1alpha1.OLSConfig
	var present bool
	var deployment *appsv1.Deployment

	BeforeAll(func() {
		var err error
		c, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
		err = c.Create(cr)
		if apierrors.IsAlreadyExists(err) {
			Skip("an existing OLSConfig is present; this suite only modifies its own fixture")
		}
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(c.DeleteAndWait(cr, 3*time.Minute)).To(Succeed()) })

		config := rest.CopyConfig(c.config)
		config.Timeout = DefaultClientTimeout
		d, err := discovery.NewDiscoveryClientForConfig(config)
		Expect(err).NotTo(HaveOccurred())
		present, err = ocpmcp.DetectFlowCollectorAPI(d)
		Expect(err).NotTo(HaveOccurred(), "discovery uncertainty must not be mistaken for API absence")
		deployment = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerDeploymentName, Namespace: OLSNameSpace}}
		Expect(c.WaitForDeploymentRollout(deployment)).To(Succeed())
	})

	It("renders NetObserv exactly once iff the FlowCollector API is served, preserving resource denials", func() {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: OLSNameSpace}}
		Eventually(func(g Gomega) {
			g.Expect(c.Get(cm)).To(Succeed())
			var parsed struct {
				Toolsets        []string                       `toml:"toolsets"`
				DeniedResources []struct{ Group, Kind string } `toml:"denied_resources"`
			}
			_, err := toml.Decode(cm.Data[utils.OpenShiftMCPServerConfigFilename], &parsed)
			g.Expect(err).NotTo(HaveOccurred())
			count := 0
			for _, toolset := range parsed.Toolsets {
				if toolset == "netobserv" {
					count++
				}
			}
			if present {
				g.Expect(count).To(Equal(1))
			} else {
				g.Expect(count).To(BeZero())
			}
			g.Expect(parsed.DeniedResources).To(HaveLen(2))
			g.Expect(parsed.DeniedResources[0].Kind).To(Equal("Secret"))
			g.Expect(parsed.DeniedResources[1].Group).To(Equal("rbac.authorization.k8s.io"))
			g.Expect(c.Get(deployment)).To(Succeed())
			g.Expect(deployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]).To(Equal(cm.ResourceVersion))
		}, DefaultPollTimeout, DefaultPollInterval).Should(Succeed())
	})

	It("starts and lists the corresponding tools without requiring backend health", func() {
		Expect(c.WaitForDeploymentRollout(deployment)).To(Succeed())
		names := netObservMCPToolNames(c)
		Expect(names).To(ContainElements("namespaces_list", "pods_list"))
		if present {
			Expect(names).To(ContainElements("netobserv_export_flows", "netobserv_get_flow_metrics", "netobserv_list_flows"))
		} else {
			for _, name := range names {
				Expect(name).NotTo(HavePrefix("netobserv_"))
			}
		}
	})

	It("does not grant the MCP ServiceAccount Secret or RBAC privileges", func() {
		user := fmt.Sprintf("system:serviceaccount:%s:%s", OLSNameSpace, utils.OpenShiftMCPServerServiceAccountName)
		for _, attributes := range []authorizationv1.ResourceAttributes{
			{Namespace: OLSNameSpace, Verb: "get", Resource: "secrets"},
			{Namespace: OLSNameSpace, Verb: "create", Group: "rbac.authorization.k8s.io", Resource: "rolebindings"},
		} {
			review, err := c.clientset.AuthorizationV1().SubjectAccessReviews().Create(context.Background(), &authorizationv1.SubjectAccessReview{
				Spec: authorizationv1.SubjectAccessReviewSpec{User: user, Groups: []string{"system:authenticated", "system:serviceaccounts", "system:serviceaccounts:" + OLSNameSpace}, ResourceAttributes: &attributes},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(review.Status.Allowed).To(BeFalse())
		}
	})

	It("removes the managed MCP resources when introspection is disabled", func() {
		Expect(c.Update(cr, func(obj ctrlclient.Object) error {
			obj.(*olsv1alpha1.OLSConfig).Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
			return nil
		})).To(Succeed())
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: OLSNameSpace}}
		service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerServiceName, Namespace: OLSNameSpace}}
		Eventually(func(g Gomega) {
			g.Expect(apierrors.IsNotFound(c.Get(deployment))).To(BeTrue(), "MCP Deployment should be removed")
			g.Expect(apierrors.IsNotFound(c.Get(cm))).To(BeTrue(), "MCP ConfigMap should be removed")
			g.Expect(apierrors.IsNotFound(c.Get(service))).To(BeTrue(), "MCP Service should be removed")
		}, DefaultPollTimeout, DefaultPollInterval).Should(Succeed())
	})
})

// netObservMCPToolNames initializes the shipped server using caller credentials.
func netObservMCPToolNames(c *Client) []string {
	// Port-forward only to the managed MCP Service and use the kubeconfig's
	// caller credentials. No FlowCollector instances or backend endpoints
	// are created, read, modified, or supplied by this test.
	address, stop, err := c.ForwardPort(utils.OpenShiftMCPServerServiceName, OLSNameSpace, utils.OpenShiftMCPServerHTTPSPort)
	Expect(err).NotTo(HaveOccurred())
	defer stop()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerCertsSecretName, Namespace: OLSNameSpace}}
	Expect(c.Get(secret)).To(Succeed())
	roots := x509.NewCertPool()
	Expect(roots.AppendCertsFromPEM(secret.Data["tls.crt"])).To(BeTrue())
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12, RootCAs: roots,
		ServerName: fmt.Sprintf("%s.%s.svc", utils.OpenShiftMCPServerServiceName, OLSNameSpace),
	}}
	defer transport.CloseIdleConnections()
	authenticatedTransport, err := rest.HTTPWrappersForConfig(c.config, transport)
	Expect(err).NotTo(HaveOccurred())
	mcp := &netObservMCPClient{client: &http.Client{Transport: authenticatedTransport, Timeout: DefaultClientTimeout}, endpoint: "https://" + address + "/mcp", protocol: "2025-03-26"}
	result, err := mcp.request("initialize", map[string]any{
		"protocolVersion": mcp.protocol, "capabilities": map[string]any{},
		"clientInfo": map[string]string{"name": "ols-netobserv-e2e", "version": "1"},
	}, false)
	Expect(err).NotTo(HaveOccurred())
	var initialized struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	Expect(json.Unmarshal(result, &initialized)).To(Succeed())
	Expect(initialized.ProtocolVersion).NotTo(BeEmpty())
	mcp.protocol = initialized.ProtocolVersion
	_, err = mcp.request("notifications/initialized", map[string]any{}, true)
	Expect(err).NotTo(HaveOccurred())
	var names []string
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err = mcp.request("tools/list", params, false)
		Expect(err).NotTo(HaveOccurred())
		var listed struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		Expect(json.Unmarshal(result, &listed)).To(Succeed())
		for _, tool := range listed.Tools {
			names = append(names, tool.Name)
		}
		cursor = listed.NextCursor
		if cursor == "" {
			break
		}
	}
	return names
}

// Minimal streamable-HTTP client for tools/list. Handles both JSON and SSE
// responses and forwards caller authentication through client-go's transport.
type netObservMCPClient struct {
	client                      *http.Client
	endpoint, session, protocol string
	id                          int
}

func (c *netObservMCPClient) request(method string, params any, notification bool) (json.RawMessage, error) {
	message := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if !notification {
		c.id++
		message["id"] = c.id
	}
	body, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocol)
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("MCP %s returned HTTP %d", method, response.StatusCode)
	}
	if session := response.Header.Get("Mcp-Session-Id"); session != "" {
		c.session = session
	}
	if notification {
		return nil, nil
	}
	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data:"); ok {
				if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &rpc); err != nil {
					return nil, err
				}
				if rpc.Result != nil || rpc.Error != nil {
					break
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	} else if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
		return nil, err
	}
	if len(rpc.Error) != 0 && string(rpc.Error) != "null" {
		return nil, fmt.Errorf("MCP %s returned JSON-RPC error: %s", method, rpc.Error)
	}
	if len(rpc.Result) == 0 {
		return nil, fmt.Errorf("MCP %s returned no result", method)
	}
	return rpc.Result, nil
}

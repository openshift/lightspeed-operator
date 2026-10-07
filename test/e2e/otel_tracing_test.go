package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const jaegerTestNamespace = OLSNameSpace

// This suite uses a real Jaeger backend so the trace can be attributed to the
// chat request, rather than to unrelated activity at the collector receiver.
var _ = Describe("Classic chat tracing", Ordered, func() {
	var env *OLSTestEnvironment
	var tlsSecret *corev1.Secret
	var jaegerStarted bool

	BeforeAll(func() {
		By("Deploying the Jaeger OTLP TLS backend")
		jaegerStarted = true // Clean up even if deployment only partially succeeds.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		output, err := exec.CommandContext(ctx, "bash", "../../hack/deploy-jaeger-tls.sh", jaegerTestNamespace).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "deploying Jaeger: %s", output)

		By("Configuring collector trace forwarding to Jaeger")
		env, err = SetupOLSTestEnvironment(func(cr *olsv1alpha1.OLSConfig) {
			cr.Spec.Audit.TracingEndpoint = fmt.Sprintf("jaeger-otlp-grpc.%s.svc:4317", jaegerTestNamespace)
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		tlsSecret, err = TestOLSServiceActivation(env)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		var environmentErr error
		if env != nil {
			environmentErr = CleanupOLSTestEnvironmentWithCRDeletion(env, "otel_tracing_test")
		}
		if jaegerStarted {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			output, err := exec.CommandContext(ctx, "bash", "../../hack/deploy-jaeger-tls.sh", "--delete", jaegerTestNamespace).CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "removing Jaeger: %s", output)
		}
		Expect(environmentErr).NotTo(HaveOccurred(), "cleaning up OLS test environment")
	})

	It("forwards a classic chat trace through the collector to Jaeger", FlakeAttempts(5), func() {
		client, err := GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		forwardHost, cleanup, err := client.ForwardPort("jaeger-query", jaegerTestNamespace, 16686)
		Expect(err).NotTo(HaveOccurred())
		defer cleanup()

		By("Sending a classic chat request with a unique UUID")
		conversationID := string(uuid.NewUUID())
		since := time.Now().Add(-time.Minute)
		requestBody := []byte(fmt.Sprintf(`{"query":"What is OpenShift?","conversation_id":%q}`, conversationID))
		resp, body, err := TestHTTPSQueryEndpointWithTimeout(env, tlsSecret, requestBody, 30*time.Second)
		CheckEOFAndRestartPortForwarding(env, err)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "chat response: %s", body)

		By("Finding the conversation span in Jaeger")
		jaegerURL := "http://" + forwardHost
		queryClient := &http.Client{Timeout: 5 * time.Second}
		Eventually(func(g Gomega) {
			found, err := jaegerHasConversationSpan(queryClient, jaegerURL, conversationID, since)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue(), "Jaeger has no trace for conversation %s", conversationID)
		}, 90*time.Second, 2*time.Second).Should(Succeed())
	})
})

// jaegerHasConversationSpan confirms the conversation attribute in the returned
// span as well as in the search filter, so unrelated traces cannot satisfy it.
func jaegerHasConversationSpan(client *http.Client, baseURL, conversationID string, since time.Time) (bool, error) {
	attributes, err := json.Marshal(map[string]string{"gen_ai.conversation.id": conversationID})
	if err != nil {
		return false, err
	}
	query := url.Values{
		"query.serviceName":  {utils.OtelAppServerServiceName},
		"query.attributes":   {string(attributes)},
		"query.startTimeMin": {since.UTC().Format(time.RFC3339Nano)},
		"query.startTimeMax": {time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)},
		"query.searchDepth":  {"20"},
	}
	response, err := client.Get(baseURL + "/api/v3/traces?" + query.Encode())
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if strings.Contains(string(message), "No traces found") {
			return false, nil
		}
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Jaeger trace search returned %d", response.StatusCode)
	}
	var result struct {
		Result struct {
			ResourceSpans []struct {
				ScopeSpans []struct {
					Spans []struct {
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								StringValue string `json:"stringValue"`
							} `json:"value"`
						} `json:"attributes"`
					} `json:"spans"`
				} `json:"scopeSpans"`
			} `json:"resourceSpans"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return false, fmt.Errorf("decoding Jaeger traces: %w", err)
	}
	for _, resource := range result.Result.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				for _, attribute := range span.Attributes {
					if attribute.Key == "gen_ai.conversation.id" && attribute.Value.StringValue == conversationID {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

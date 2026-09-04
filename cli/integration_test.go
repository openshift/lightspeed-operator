package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Integration", func() {
	// createKubeconfig writes a minimal kubeconfig file with token auth
	// pointing at the given server URL, returning the path.
	createKubeconfig := func(serverURL, token string) string {
		f, err := os.CreateTemp("", "kubeconfig-*.yaml")
		Expect(err).NotTo(HaveOccurred())

		content := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-ctx
users:
- name: test-user
  user:
    token: %s
`, serverURL, token)

		_, err = f.WriteString(content)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.Close()).To(Succeed())
		return f.Name()
	}

	// buildEndEvent builds an end event in the real server format.
	buildEndEvent := func(docs []ReferencedDocument) string {
		payload := map[string]interface{}{
			"referenced_documents": docs,
			"truncated":            false,
			"input_tokens":         100,
			"output_tokens":        50,
			"reasoning_tokens":     0,
		}
		return sseEndEvent(payload)
	}

	Describe("full command flow via default mode", func() {
		It("sends query and streams response through root command dispatch", func() {
			var capturedMethod string
			var capturedPath string
			var capturedAuth string
			var capturedBody []byte

			docs := []ReferencedDocument{
				{DocTitle: "Pods Guide", DocURL: "https://docs.example.com/pods"},
			}

			sseBody := sseEvent(EventStart, map[string]interface{}{"conversation_id": "conv-integration-1"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "The pod is"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 1, "token": " crashing because"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 2, "token": " of OOM."}) +
				buildEndEvent(docs)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedAuth = r.Header.Get("Authorization")
				capturedBody, _ = io.ReadAll(r.Body)

				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, sseBody)
			}))
			defer server.Close()

			kubeconfigPath := createKubeconfig("https://k8s.example.com", "test-bearer-token")
			defer os.Remove(kubeconfigPath)

			streams, out, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			cmd.SetArgs([]string{
				"--kubeconfig", kubeconfigPath,
				"--endpoint", server.URL,
				"--insecure-skip-tls-verify",
				"why is my pod crashing",
			})

			Expect(cmd.Execute()).To(Succeed())

			// Verify correct HTTP request was sent
			Expect(capturedMethod).To(Equal("POST"))
			Expect(capturedPath).To(Equal("/v1/streaming_query"))
			Expect(capturedAuth).To(Equal("Bearer test-bearer-token"))

			// Verify request body
			var reqBody LLMRequest
			Expect(json.Unmarshal(capturedBody, &reqBody)).To(Succeed())
			Expect(reqBody.Query).To(Equal("why is my pod crashing"))
			Expect(reqBody.Mode).To(Equal("ask"))
			Expect(reqBody.MediaType).To(Equal("application/json"))

			// Verify tokens appeared on stdout
			Expect(out.String()).To(ContainSubstring("The pod is"))
			Expect(out.String()).To(ContainSubstring("crashing because"))
			Expect(out.String()).To(ContainSubstring("of OOM."))

			// Verify referenced documents on stderr
			Expect(out.String()).To(ContainSubstring("References:"))
			Expect(out.String()).To(ContainSubstring("Pods Guide"))
			Expect(out.String()).To(ContainSubstring("https://docs.example.com/pods"))
		})

		It("works with explicit ask subcommand", func() {
			body := sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "answer"}) +
				buildEndEvent([]ReferencedDocument{})

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, body)
			}))
			defer server.Close()

			kubeconfigPath := createKubeconfig("https://k8s.example.com", "token-123")
			defer os.Remove(kubeconfigPath)

			streams, out, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			cmd.SetArgs([]string{
				"--kubeconfig", kubeconfigPath,
				"--endpoint", server.URL,
				"--insecure-skip-tls-verify",
				"ask", "what is a deployment",
			})

			Expect(cmd.Execute()).To(Succeed())
			Expect(out.String()).To(ContainSubstring("answer"))
		})

		It("returns error on HTTP 401 through full dispatch", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()

			kubeconfigPath := createKubeconfig("https://k8s.example.com", "expired-token")
			defer os.Remove(kubeconfigPath)

			streams, _, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			cmd.SetArgs([]string{
				"--kubeconfig", kubeconfigPath,
				"--endpoint", server.URL,
				"--insecure-skip-tls-verify",
				"check my cluster",
			})

			err := cmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrAuthFailed))
		})

		It("returns error when no query is provided via ask subcommand", func() {
			kubeconfigPath := createKubeconfig("https://k8s.example.com", "token")
			defer os.Remove(kubeconfigPath)

			streams, _, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			cmd.SetArgs([]string{
				"--kubeconfig", kubeconfigPath,
				"--endpoint", "https://ols.example.com",
				"ask",
			})

			err := cmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrQueryEmpty))
		})
	})
})

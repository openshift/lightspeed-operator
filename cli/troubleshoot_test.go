package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TroubleshootCmd", func() {
	sseServer := func(sseBody string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, sseBody)
		}))
	}

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

	Describe("queryRun with troubleshooting mode", func() {
		It("streams token events to stdout and extracts conversation_id", func() {
			body := sseEvent(EventStart, map[string]interface{}{"conversation_id": "conv-ts-1"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "Check"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 1, "token": " logs"}) +
				buildEndEvent([]ReferencedDocument{})

			server := sseServer(body)
			defer server.Close()

			streams, out, _ := fakeStreams()
			o := &commandOptions{
				streams:           streams,
				query:             "pod keeps crashing",
				endpoint:          server.URL,
				insecureAllowHTTP: true,
				kubeConfig:        &KubeConfig{BearerToken: "test-token"},
			}

			cmd := NewTroubleshootCmd(streams)
			Expect(queryRun(cmd, o, "troubleshooting")).To(Succeed())
			Expect(out.String()).To(Equal("Check logs\n"))
			Expect(o.conversationID).To(Equal("conv-ts-1"))
		})

		It("displays referenced documents on stdout", func() {
			docs := []ReferencedDocument{
				{DocTitle: "Troubleshooting Guide", DocURL: "https://docs.example.com/troubleshoot"},
			}
			body := sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "answer"}) +
				buildEndEvent(docs)

			server := sseServer(body)
			defer server.Close()

			streams, out, _ := fakeStreams()
			o := &commandOptions{
				streams:           streams,
				query:             "test",
				endpoint:          server.URL,
				insecureAllowHTTP: true,
				kubeConfig:        &KubeConfig{BearerToken: "test-token"},
			}

			cmd := NewTroubleshootCmd(streams)
			Expect(queryRun(cmd, o, "troubleshooting")).To(Succeed())
			Expect(out.String()).To(ContainSubstring("References:"))
			Expect(out.String()).To(ContainSubstring("Troubleshooting Guide"))
		})

		It("propagates HTTP errors from SSEClient", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()

			streams, _, _ := fakeStreams()
			o := &commandOptions{
				streams:           streams,
				query:             "test",
				endpoint:          server.URL,
				insecureAllowHTTP: true,
				kubeConfig:        &KubeConfig{BearerToken: "bad-token"},
			}

			cmd := NewTroubleshootCmd(streams)
			err := queryRun(cmd, o, "troubleshooting")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrAuthFailed))
		})

		It("captures reasoning and tool events without displaying them", func() {
			body := sseEvent(EventStart, map[string]interface{}{"conversation_id": "conv-ts-2"}) +
				sseEvent(EventReasoning, map[string]interface{}{"content": "analyzing..."}) +
				sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "fixed"}) +
				sseEvent(EventToolCall, map[string]interface{}{"name": "kubectl", "args": map[string]string{"cmd": "get pods"}, "id": "call_1", "type": "tool_call"}) +
				buildEndEvent([]ReferencedDocument{})

			server := sseServer(body)
			defer server.Close()

			streams, out, errOut := fakeStreams()
			o := &commandOptions{
				streams:           streams,
				query:             "test",
				endpoint:          server.URL,
				insecureAllowHTTP: true,
				kubeConfig:        &KubeConfig{BearerToken: "test-token"},
			}

			cmd := NewTroubleshootCmd(streams)
			Expect(queryRun(cmd, o, "troubleshooting")).To(Succeed())

			Expect(out.String()).To(Equal("fixed\n"))
			Expect(errOut.String()).NotTo(ContainSubstring("analyzing"))
			Expect(errOut.String()).NotTo(ContainSubstring("kubectl"))

			// start + reasoning + tool_call = 3 captured events
			Expect(o.capturedEvents).To(HaveLen(3))
			Expect(o.capturedEvents[0].Type).To(Equal(EventStart))
			Expect(o.capturedEvents[1].Type).To(Equal(EventReasoning))
			Expect(o.capturedEvents[2].Type).To(Equal(EventToolCall))
		})

		It("returns error when stream has no end event", func() {
			body := sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "partial"})

			server := sseServer(body)
			defer server.Close()

			streams, _, errOut := fakeStreams()
			o := &commandOptions{
				streams:           streams,
				query:             "test",
				endpoint:          server.URL,
				insecureAllowHTTP: true,
				kubeConfig:        &KubeConfig{BearerToken: "test-token"},
			}

			cmd := NewTroubleshootCmd(streams)
			err := queryRun(cmd, o, "troubleshooting")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrMissingEnd))
			Expect(errOut.String()).To(ContainSubstring(ErrStreamIncomplete))
		})
	})

	Describe("NewTroubleshootCmd", func() {
		It("creates command with correct use string", func() {
			streams, _, _ := fakeStreams()
			cmd := NewTroubleshootCmd(streams)
			Expect(cmd.Use).To(Equal("troubleshoot [question]"))
		})

		It("accepts arbitrary args", func() {
			streams, _, _ := fakeStreams()
			cmd := NewTroubleshootCmd(streams)
			Expect(cmd.Args).NotTo(BeNil())
		})
	})
})

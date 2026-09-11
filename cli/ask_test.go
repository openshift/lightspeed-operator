package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AskCmd", func() {
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

	Describe("queryRun with ask mode", func() {
		It("streams token events to stdout and extracts conversation_id", func() {
			body := sseEvent(EventStart, map[string]interface{}{"conversation_id": "conv-123"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "Hello"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 1, "token": " world"}) +
				buildEndEvent([]ReferencedDocument{})

			server := sseServer(body)
			defer server.Close()

			streams, out, _ := fakeStreams()
			o := &commandOptions{
				streams:           streams,
				query:             "test question",
				endpoint:          server.URL,
				insecureAllowHTTP: true,
				kubeConfig:        &KubeConfig{BearerToken: "test-token"},
			}

			cmd := NewAskCmd(streams)
			Expect(queryRun(cmd, o, "ask")).To(Succeed())
			Expect(out.String()).To(Equal("Hello world\n"))
			Expect(o.conversationID).To(Equal("conv-123"))
		})

		It("displays referenced documents on stdout", func() {
			docs := []ReferencedDocument{
				{DocTitle: "Pod Debugging", DocURL: "https://docs.example.com/pods"},
				{DocTitle: "Logs Guide", DocURL: "https://docs.example.com/logs"},
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

			cmd := NewAskCmd(streams)
			Expect(queryRun(cmd, o, "ask")).To(Succeed())
			Expect(out.String()).To(ContainSubstring("References:"))
			Expect(out.String()).To(ContainSubstring("Pod Debugging"))
			Expect(out.String()).To(ContainSubstring("https://docs.example.com/pods"))
		})

		It("handles stream with no referenced documents", func() {
			body := sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "simple answer"}) +
				buildEndEvent([]ReferencedDocument{})

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

			cmd := NewAskCmd(streams)
			Expect(queryRun(cmd, o, "ask")).To(Succeed())
			Expect(out.String()).To(ContainSubstring("simple answer"))
			Expect(out.String()).NotTo(ContainSubstring("References:"))
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

			cmd := NewAskCmd(streams)
			err := queryRun(cmd, o, "ask")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrAuthFailed))
		})

		It("captures reasoning, tool_call, and tool_result events without displaying them", func() {
			body := sseEvent(EventStart, map[string]interface{}{"conversation_id": "conv-abc"}) +
				sseEvent(EventReasoning, map[string]interface{}{"content": "thinking..."}) +
				sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "result"}) +
				sseEvent(EventToolCall, map[string]interface{}{"name": "search", "args": map[string]string{"q": "test"}, "id": "call_1", "type": "tool_call"}) +
				sseEvent(EventToolResult, map[string]interface{}{"id": "call_1", "name": "search", "status": "success", "content": "found it", "type": "tool_result", "round": 1}) +
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

			cmd := NewAskCmd(streams)
			Expect(queryRun(cmd, o, "ask")).To(Succeed())

			Expect(out.String()).To(Equal("result\n"))
			Expect(errOut.String()).NotTo(ContainSubstring("thinking"))
			Expect(errOut.String()).NotTo(ContainSubstring("search"))
			Expect(errOut.String()).NotTo(ContainSubstring("found it"))

			// start + reasoning + tool_call + tool_result = 4 captured events
			Expect(o.capturedEvents).To(HaveLen(4))
			Expect(o.capturedEvents[0].Type).To(Equal(EventStart))
			Expect(o.capturedEvents[1].Type).To(Equal(EventReasoning))
			Expect(o.capturedEvents[2].Type).To(Equal(EventToolCall))
			Expect(o.capturedEvents[3].Type).To(Equal(EventToolResult))
		})

		It("warns on malformed end event JSON", func() {
			body := sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "answer"}) +
				sseEvent(EventEnd, "not-a-json-object")

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

			cmd := NewAskCmd(streams)
			err := queryRun(cmd, o, "ask")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrMalformedEnd))
			Expect(errOut.String()).To(ContainSubstring(ErrMalformedEnd))
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

			cmd := NewAskCmd(streams)
			err := queryRun(cmd, o, "ask")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrMissingEnd))
			Expect(errOut.String()).To(ContainSubstring(ErrStreamIncomplete))
		})

		It("does not print trailing newline when no tokens were emitted", func() {
			body := buildEndEvent([]ReferencedDocument{})

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

			cmd := NewAskCmd(streams)
			Expect(queryRun(cmd, o, "ask")).To(Succeed())
			Expect(out.String()).To(Equal(""))
		})
	})

	Describe("validate", func() {
		It("rejects empty query", func() {
			o := &commandOptions{query: "", endpoint: "https://example.com"}
			Expect(o.validate()).To(MatchError(ContainSubstring(ErrQueryEmpty)))
		})

		It("rejects whitespace-only query", func() {
			o := &commandOptions{query: "   ", endpoint: "https://example.com"}
			Expect(o.validate()).To(MatchError(ContainSubstring(ErrQueryEmpty)))
		})

		It("rejects missing endpoint", func() {
			o := &commandOptions{query: "test question", endpoint: ""}
			Expect(o.validate()).To(MatchError(ContainSubstring(ErrNoEndpoint)))
		})

		It("accepts valid options", func() {
			o := &commandOptions{query: "why is my pod crashing", endpoint: "https://ols.example.com"}
			Expect(o.validate()).To(Succeed())
		})

		It("rejects cleartext HTTP endpoint by default", func() {
			o := &commandOptions{query: "test", endpoint: "http://ols.example.com"}
			Expect(o.validate()).To(MatchError(ContainSubstring("cleartext HTTP")))
		})

		It("allows cleartext HTTP with insecureAllowHTTP", func() {
			o := &commandOptions{query: "test", endpoint: "http://ols.example.com", insecureAllowHTTP: true}
			Expect(o.validate()).To(Succeed())
		})

		It("rejects endpoint without host", func() {
			o := &commandOptions{query: "test", endpoint: "https:///v1"}
			Expect(o.validate()).To(MatchError(ContainSubstring("host is required")))
		})

		It("rejects unsupported scheme", func() {
			o := &commandOptions{query: "test", endpoint: "ftp://ols.example.com"}
			Expect(o.validate()).To(MatchError(ContainSubstring("unsupported endpoint scheme")))
		})
	})

	Describe("NewAskCmd", func() {
		It("creates command with correct use string", func() {
			streams, _, _ := fakeStreams()
			cmd := NewAskCmd(streams)
			Expect(cmd.Use).To(Equal("ask [question]"))
		})

		It("accepts arbitrary args", func() {
			streams, _, _ := fakeStreams()
			cmd := NewAskCmd(streams)
			Expect(cmd.Args).NotTo(BeNil())
		})
	})
})

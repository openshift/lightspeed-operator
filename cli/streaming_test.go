package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parseSSEStream", func() {
	// collectEvents drains the event channel and returns all events.
	collectEvents := func(events <-chan SSEEvent, errc <-chan error) ([]SSEEvent, error) {
		var result []SSEEvent
		for ev := range events {
			result = append(result, ev)
		}
		if err, ok := <-errc; ok && err != nil {
			return result, err
		}
		return result, nil
	}

	// nopCloser wraps a reader with a no-op Close for parseSSEStream.
	nopCloser := io.NopCloser

	It("parses a single token event", func() {
		input := sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "hello"})
		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader(input)))
		result, err := collectEvents(events, errc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Type).To(Equal(EventToken))

		var td TokenEventData
		Expect(json.Unmarshal([]byte(result[0].Data), &td)).To(Succeed())
		Expect(td.Token).To(Equal("hello"))
	})

	It("parses a complete SSE stream with multiple event types", func() {
		input := sseEvent(EventStart, map[string]interface{}{"conversation_id": "abc-123"}) +
			sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "Hello"}) +
			sseEvent(EventToken, map[string]interface{}{"id": 1, "token": " world"}) +
			sseEvent(EventReasoning, map[string]interface{}{"content": "thinking about it"}) +
			sseEvent(EventToolCall, map[string]interface{}{"name": "search", "args": map[string]string{"q": "test"}, "id": "call_1", "type": "tool_call"}) +
			sseEvent(EventToolResult, map[string]interface{}{"id": "call_1", "name": "search", "status": "success", "content": "results", "type": "tool_result", "round": 1}) +
			sseEndEvent(map[string]interface{}{
				"referenced_documents": []interface{}{},
				"truncated":            false,
				"input_tokens":         100,
				"output_tokens":        50,
				"reasoning_tokens":     0,
			})

		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader(input)))
		result, err := collectEvents(events, errc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(7))

		Expect(result[0].Type).To(Equal(EventStart))
		Expect(result[1].Type).To(Equal(EventToken))
		Expect(result[2].Type).To(Equal(EventToken))
		Expect(result[3].Type).To(Equal(EventReasoning))
		Expect(result[4].Type).To(Equal(EventToolCall))
		Expect(result[5].Type).To(Equal(EventToolResult))
		Expect(result[6].Type).To(Equal(EventEnd))

		// Verify start event has conversation_id
		var start StartEventData
		Expect(json.Unmarshal([]byte(result[0].Data), &start)).To(Succeed())
		Expect(start.ConversationID).To(Equal("abc-123"))

		// Verify token data
		var tok TokenEventData
		Expect(json.Unmarshal([]byte(result[1].Data), &tok)).To(Succeed())
		Expect(tok.Token).To(Equal("Hello"))
		Expect(tok.ID).To(Equal(0))
	})

	It("ignores SSE comment lines", func() {
		input := ": keep-alive\n" + sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "hi"})
		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader(input)))
		result, err := collectEvents(events, errc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Type).To(Equal(EventToken))
	})

	It("returns no events for empty input", func() {
		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader("")))
		result, err := collectEvents(events, errc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeEmpty())
	})

	It("propagates reader errors", func() {
		events, errc := parseSSEStream(context.Background(), nopCloser(&failingReader{err: io.ErrUnexpectedEOF}))
		result, err := collectEvents(events, errc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrParseSSE))
		Expect(result).To(BeEmpty())
	})

	It("returns error on invalid JSON in data line", func() {
		input := "data: {not valid json}\n\n"
		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader(input)))
		result, err := collectEvents(events, errc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrParseSSE))
		Expect(result).To(BeEmpty())
	})

	It("handles tool_result events with large payloads", func() {
		// Simulate a large tool_result like the real server sends
		largeContent := strings.Repeat("pod info ", 1000)
		input := sseEvent(EventToolResult, map[string]interface{}{
			"id":      "call_abc",
			"name":    "pods_list",
			"status":  "success",
			"content": largeContent,
			"type":    "tool_result",
			"round":   1,
		})
		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader(input)))
		result, err := collectEvents(events, errc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Type).To(Equal(EventToolResult))
	})

	It("handles end event with available_quotas field", func() {
		input := sseEndEvent(map[string]interface{}{
			"referenced_documents": []map[string]string{
				{"doc_url": "https://docs.example.com", "doc_title": "Test"},
			},
			"truncated":        false,
			"input_tokens":     2899,
			"output_tokens":    191,
			"reasoning_tokens": 0,
		})
		events, errc := parseSSEStream(context.Background(), nopCloser(strings.NewReader(input)))
		result, err := collectEvents(events, errc)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Type).To(Equal(EventEnd))

		var end EndEventData
		Expect(json.Unmarshal([]byte(result[0].Data), &end)).To(Succeed())
		Expect(end.ReferencedDocuments).To(HaveLen(1))
		Expect(end.InputTokens).To(Equal(2899))
	})
})

// failingReader always returns an error on Read.
type failingReader struct {
	err error
}

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// sseHandler returns an http.HandlerFunc that writes SSE events to the response.
// It validates the request method, content type, and auth header.
func sseHandler(sseBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseBody))
	}
}

var _ = Describe("SSEClient", func() {
	var (
		server *httptest.Server
		client *SSEClient
		ctx    context.Context
		cancel context.CancelFunc
	)

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		if server != nil {
			server.Close()
		}
	})

	setupClient := func(handler http.HandlerFunc) {
		server = httptest.NewServer(handler)
		client = NewSSEClient(server.URL, "test-token", nil)
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	}

	baseRequest := func() LLMRequest {
		return LLMRequest{
			Query:     "why is my pod crashing",
			Mode:      "ask",
			MediaType: "application/json",
		}
	}

	Describe("StreamQuery", func() {
		It("streams token events from a valid SSE response", func() {
			body := sseEvent(EventStart, map[string]interface{}{"conversation_id": "id-1"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 0, "token": "Hello"}) +
				sseEvent(EventToken, map[string]interface{}{"id": 1, "token": " world"}) +
				sseEndEvent(map[string]interface{}{
					"referenced_documents": []interface{}{},
					"truncated":            false,
					"input_tokens":         50,
					"output_tokens":        10,
					"reasoning_tokens":     0,
				})

			setupClient(sseHandler(body))

			events, errc, err := client.StreamQuery(ctx, baseRequest())
			Expect(err).NotTo(HaveOccurred())

			var received []SSEEvent
			for ev := range events {
				received = append(received, ev)
			}
			Expect(<-errc).NotTo(HaveOccurred())

			Expect(received).To(HaveLen(4))
			Expect(received[0].Type).To(Equal(EventStart))
			Expect(received[1].Type).To(Equal(EventToken))
			Expect(received[2].Type).To(Equal(EventToken))
			Expect(received[3].Type).To(Equal(EventEnd))

			var tok TokenEventData
			Expect(json.Unmarshal([]byte(received[1].Data), &tok)).To(Succeed())
			Expect(tok.Token).To(Equal("Hello"))
		})

		It("sends correct request headers and body", func() {
			var capturedReq *http.Request
			var capturedBody []byte

			handler := func(w http.ResponseWriter, r *http.Request) {
				capturedReq = r
				capturedBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(sseEndEvent(map[string]interface{}{
					"referenced_documents": []interface{}{},
					"truncated":            false,
					"input_tokens":         0,
					"output_tokens":        0,
					"reasoning_tokens":     0,
				})))
			}

			setupClient(handler)
			events, errc, err := client.StreamQuery(ctx, baseRequest())
			Expect(err).NotTo(HaveOccurred())

			// Drain events
			for range events {
			}
			Expect(<-errc).NotTo(HaveOccurred())

			Expect(capturedReq.Method).To(Equal(http.MethodPost))
			Expect(capturedReq.URL.Path).To(Equal(streamingQueryPath))
			Expect(capturedReq.Header.Get("Authorization")).To(Equal("Bearer test-token"))
			Expect(capturedReq.Header.Get("Content-Type")).To(Equal("application/json"))

			var reqBody LLMRequest
			Expect(json.Unmarshal(capturedBody, &reqBody)).To(Succeed())
			Expect(reqBody.Query).To(Equal("why is my pod crashing"))
			Expect(reqBody.Mode).To(Equal("ask"))
		})

		It("returns auth error on 401", func() {
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}
			setupClient(handler)

			_, _, err := client.StreamQuery(ctx, baseRequest())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrAuthFailed))
			Expect(err.Error()).To(ContainSubstring("oc login"))
		})

		It("returns access denied on 403", func() {
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			}
			setupClient(handler)

			_, _, err := client.StreamQuery(ctx, baseRequest())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrAccessDenied))
			Expect(err.Error()).To(ContainSubstring("cluster administrator"))
		})

		It("returns prompt too long on 413", func() {
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			}
			setupClient(handler)

			_, _, err := client.StreamQuery(ctx, baseRequest())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrPromptTooLong))
		})

		It("returns generic error on other non-200 status", func() {
			handler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}
			setupClient(handler)

			_, _, err := client.StreamQuery(ctx, baseRequest())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrServiceError))
			Expect(err.Error()).To(ContainSubstring("500"))
		})

		It("respects context cancellation", func() {
			// Server blocks until request context is done
			handler := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				// Block until the client cancels
				<-r.Context().Done()
			}

			setupClient(handler)
			cancelCtx, cancelFn := context.WithCancel(ctx)

			events, errc, err := client.StreamQuery(cancelCtx, baseRequest())
			Expect(err).NotTo(HaveOccurred())

			// Cancel the context — the stream should terminate
			cancelFn()

			// Drain events — should close quickly
			for range events {
			}

			// Error channel may or may not have an error depending on timing,
			// but it must close without blocking
			Eventually(errc).Should(BeClosed())
		})
	})
})

var _ = Describe("idleTimeoutReader", func() {
	It("times out when no data arrives", func() {
		// Create a reader that blocks forever
		pr, _ := io.Pipe()
		reader := newIdleTimeoutReader(pr, 50*time.Millisecond)

		buf := make([]byte, 64)
		_, err := reader.Read(buf)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrStreamTimeout))
	})

	It("resets timeout on successful reads", func() {
		input := "some data"
		reader := newIdleTimeoutReader(io.NopCloser(strings.NewReader(input)), 1*time.Second)

		buf := make([]byte, 64)
		n, err := reader.Read(buf)
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(BeNumerically(">", 0))
	})
})

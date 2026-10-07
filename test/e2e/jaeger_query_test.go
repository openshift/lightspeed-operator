package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJaegerTraceSearch(t *testing.T) {
	const conversationID = "123e4567-e89b-42d3-a456-426614174000"
	since := time.Now().Add(-time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		max, err := time.Parse(time.RFC3339Nano, query.Get("query.startTimeMax"))
		if err != nil || !max.After(since) {
			t.Errorf("invalid search end time: %q (%v)", query.Get("query.startTimeMax"), err)
		}
		if r.URL.Path != "/api/v3/traces" || query.Get("query.serviceName") != "lightspeed-service" ||
			query.Get("query.attributes") != fmt.Sprintf(`{"gen_ai.conversation.id":%q}`, conversationID) ||
			query.Get("query.startTimeMin") != since.UTC().Format(time.RFC3339Nano) {
			t.Errorf("unexpected Jaeger search: %s", r.URL.String())
		}
		_, _ = fmt.Fprint(w, `{"result":{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"gen_ai.conversation.id","value":{"stringValue":"other"}}]}]}]},{"scopeSpans":[{"spans":[{"attributes":[{"key":"gen_ai.conversation.id","value":{"stringValue":"`+conversationID+`"}}]}]}]}]}}`)
	}))
	defer server.Close()

	found, err := jaegerHasConversationSpan(server.Client(), server.URL, conversationID, since)
	if err != nil || !found {
		t.Fatalf("expected trace for conversation %s, got found=%t err=%v", conversationID, found, err)
	}
}

func TestJaegerTraceSearchIgnoresUnrelatedTraces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"result":{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"gen_ai.conversation.id","value":{"stringValue":"other"}}]}]}]}]}}`)
	}))
	defer server.Close()

	found, err := jaegerHasConversationSpan(server.Client(), server.URL, "123e4567-e89b-42d3-a456-426614174000", time.Now().Add(-time.Minute))
	if err != nil || found {
		t.Fatalf("unrelated trace must not match: found=%t err=%v", found, err)
	}
}

func TestJaegerTraceSearchNoTracesYet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"No traces found"}}`, http.StatusNotFound)
	}))
	defer server.Close()

	found, err := jaegerHasConversationSpan(server.Client(), server.URL, "123e4567-e89b-42d3-a456-426614174000", time.Now().Add(-time.Minute))
	if err != nil || found {
		t.Fatalf("empty search must be retried: found=%t err=%v", found, err)
	}
}

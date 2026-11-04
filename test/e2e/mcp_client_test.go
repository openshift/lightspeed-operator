package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeMCPResponse(t *testing.T) {
	sse := `event: message` + "\n" + `data: {"jsonrpc":"2.0","id":2,"result":{"tools":[]}}` + "\n\n"
	msg, err := decodeMCPResponse([]byte(sse))
	if err != nil {
		t.Fatalf("decode SSE: %v", err)
	}
	if string(msg.Result) != `{"tools":[]}` {
		t.Fatalf("unexpected result %s", msg.Result)
	}

	plain := []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"pods_list"}]}}`)
	msg, err = decodeMCPResponse(plain)
	if err != nil {
		t.Fatalf("decode plain JSON: %v", err)
	}
	if !strings.Contains(string(msg.Result), "pods_list") {
		t.Fatalf("unexpected result %s", msg.Result)
	}

	if _, err := decodeMCPResponse([]byte("event: message\n\n")); err == nil {
		t.Fatal("expected error for a response without a JSON-RPC payload")
	}
}

func TestMCPListToolsHandshake(t *testing.T) {
	var sawSession bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch payload.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `event: message`+"\n"+`data: {"jsonrpc":"2.0","id":1,"result":{}}`+"\n\n")
		case "notifications/initialized":
			sawSession = r.Header.Get("Mcp-Session-Id") == "test-session"
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `event: message`+"\n"+`data: {"jsonrpc":"2.0","id":2,"result":{"tools":[`)
			_, _ = fmt.Fprint(w, `{"name":"pods_list","annotations":{"readOnlyHint":true,"destructiveHint":false}},`)
			_, _ = fmt.Fprint(w, `{"name":"resources_create_or_update","annotations":{"readOnlyHint":false,"destructiveHint":true}}`)
			_, _ = fmt.Fprint(w, `]}}`+"\n\n")
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	tools, err := newMCPServerClient(server.Client(), server.URL).listTools()
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if !sawSession {
		t.Fatal("notifications/initialized must carry the session ID")
	}
	destructive, nonReadOnly := toolHintCounts(tools)
	if destructive != 1 || nonReadOnly != 1 {
		t.Fatalf("expected 1 destructive and 1 non-read-only tool, got %d and %d", destructive, nonReadOnly)
	}
}

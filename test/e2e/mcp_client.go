package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	mcpProtocolVersion = "2025-03-26"
	mcpEndpointPath    = "/mcp"
)

// mcpToolAnnotations is the subset of MCP tool annotations that the operator's
// generated config controls through read_only and disable_destructive.
type mcpToolAnnotations struct {
	ReadOnlyHint    *bool `json:"readOnlyHint"`
	DestructiveHint *bool `json:"destructiveHint"`
}

// mcpTool is the subset of a tools/list entry the tests rely on.
type mcpTool struct {
	Name        string              `json:"name"`
	Annotations *mcpToolAnnotations `json:"annotations"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpRPCResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *mcpRPCError    `json:"error"`
}

// mcpServerClient speaks just enough of the MCP Streamable HTTP transport to
// read tools/list from the OpenShift MCP server.
type mcpServerClient struct {
	http      *http.Client
	url       string
	sessionID string
}

func newMCPServerClient(httpClient *http.Client, baseURL string) *mcpServerClient {
	return &mcpServerClient{http: httpClient, url: strings.TrimSuffix(baseURL, "/") + mcpEndpointPath}
}

// post sends one JSON-RPC payload and returns the decoded response. Notifications
// (payloads without an id) are answered with an empty 202 response.
func (c *mcpServerClient) post(payload map[string]any) (*mcpRPCResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal MCP request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build MCP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The server rejects requests that do not accept both content types.
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send MCP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read MCP response: %w", err)
	}
	if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
		c.sessionID = sessionID
	}
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return &mcpRPCResponse{}, nil
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("MCP request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return &mcpRPCResponse{}, nil
	}
	msg, err := decodeMCPResponse(raw)
	if err != nil {
		return nil, err
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", msg.Error.Code, msg.Error.Message)
	}
	return msg, nil
}

// listTools runs the initialize handshake and returns the advertised tools.
func (c *mcpServerClient) listTools() ([]mcpTool, error) {
	if _, err := c.post(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]string{"name": "openshift-lightspeed-e2e", "version": "1"},
		},
	}); err != nil {
		return nil, err
	}
	if _, err := c.post(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		return nil, err
	}
	msg, err := c.post(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list result: %w", err)
	}
	return result.Tools, nil
}

// decodeMCPResponse returns the JSON-RPC response carried by a plain JSON body or
// by the text/event-stream frames of the MCP Streamable HTTP transport. Frames
// without an id are notifications and are skipped.
func decodeMCPResponse(raw []byte) (*mcpRPCResponse, error) {
	for _, line := range strings.Split(string(raw), "\n") {
		data, ok := strings.CutPrefix(strings.TrimSpace(line), "data:")
		if !ok {
			continue
		}
		var msg mcpRPCResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &msg); err != nil {
			return nil, err
		}
		if len(msg.ID) == 0 {
			// Notifications carry no id and may precede the response.
			continue
		}
		return &msg, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == byte(123) {
		var msg mcpRPCResponse
		if err := json.Unmarshal(trimmed, &msg); err != nil {
			return nil, fmt.Errorf("decode MCP response: %w", err)
		}
		if len(msg.ID) != 0 {
			return &msg, nil
		}
	}
	return nil, fmt.Errorf("no JSON-RPC response in MCP response")
}

func toolHintCounts(tools []mcpTool) (destructive, nonReadOnly int) {
	for _, tool := range tools {
		if tool.Annotations == nil {
			continue
		}
		if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			destructive++
		}
		if tool.Annotations.ReadOnlyHint != nil && !*tool.Annotations.ReadOnlyHint {
			nonReadOnly++
		}
	}
	return destructive, nonReadOnly
}

func toolNames(tools []mcpTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

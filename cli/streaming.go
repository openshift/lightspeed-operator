// Package cli implements the oc-ols kubectl plugin for querying OpenShift Lightspeed from the terminal.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// SSE event type constants matching lightspeed-service event types.
const (
	EventStart      = "start"
	EventToken      = "token"
	EventReasoning  = "reasoning"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventEnd        = "end"

	ErrParseSSE      = "failed to parse SSE stream"
	ErrSendRequest   = "failed to send request" //#nosec G101 -- error message, not a credential
	ErrAuthFailed    = "authentication failed"  //#nosec G101 -- error message, not a credential
	ErrAccessDenied  = "access denied"
	ErrPromptTooLong = "query exceeds maximum length"
	ErrServiceError  = "service error"
	ErrStreamTimeout = "stream idle timeout"

	streamingQueryPath = "/v1/streaming_query"

	// HTTP transport timeouts.
	connectTimeout        = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second

	// IdleTimeout is the maximum duration to wait for the next SSE event
	// before considering the stream stalled.
	IdleTimeout = 120 * time.Second
)

// SSEEvent represents a single parsed Server-Sent Event from the
// lightspeed-service streaming response.
type SSEEvent struct {
	Type string
	Data string
}

// SSEClient sends queries to the lightspeed-service and streams back SSE events.
type SSEClient struct {
	httpClient *http.Client
	endpoint   string
	token      string
}

// NewSSEClient creates an SSE client configured with the given endpoint,
// bearer token, and TLS settings.
func NewSSEClient(endpoint, token string, tlsConfig *tls.Config) *SSEClient {
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout: connectTimeout,
		}).DialContext,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}

	return &SSEClient{
		httpClient: &http.Client{Transport: transport},
		endpoint:   endpoint,
		token:      token,
	}
}

// StreamQuery sends a query to lightspeed-service and returns a channel of
// SSE events. The caller must drain the events channel. Immediate failures
// (connection errors, non-200 status) are returned as the error value.
// Stream-level errors (parse failures, idle timeout) are sent on the error
// channel.
func (c *SSEClient) StreamQuery(ctx context.Context, req LLMRequest) (<-chan SSEEvent, <-chan error, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ErrSendRequest, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+streamingQueryPath, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ErrSendRequest, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ErrSendRequest, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, nil, httpStatusError(resp.StatusCode)
	}

	// Wrap the response body with an idle timeout so a stalled stream
	// does not block forever.
	reader := newIdleTimeoutReader(resp.Body, IdleTimeout)

	events, errc := parseSSEStream(ctx, reader)
	return events, errc, nil
}

// httpStatusError maps HTTP status codes to user-facing error messages
// matching the spec's error handling table.
func httpStatusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s: is your login session active? Try: oc login", ErrAuthFailed)
	case http.StatusForbidden:
		return fmt.Errorf("%s: contact your cluster administrator to grant OLS access", ErrAccessDenied)
	case http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%s: try a shorter question or fewer attachments", ErrPromptTooLong)
	default:
		return fmt.Errorf("%s: server returned %d", ErrServiceError, status)
	}
}

// idleTimeoutReader wraps an io.ReadCloser and enforces a maximum idle
// duration between reads. If no data arrives within the timeout window,
// subsequent reads return an error.
type idleTimeoutReader struct {
	rc      io.ReadCloser
	timer   *time.Timer
	timeout time.Duration
	failed  error
}

func newIdleTimeoutReader(rc io.ReadCloser, timeout time.Duration) *idleTimeoutReader {
	return &idleTimeoutReader{
		rc:      rc,
		timer:   time.NewTimer(timeout),
		timeout: timeout,
	}
}

// Close stops the idle timer and closes the underlying reader.
func (r *idleTimeoutReader) Close() error {
	r.timer.Stop()
	return r.rc.Close()
}

// readResult holds the outcome of a goroutine-based read.
type readResult struct {
	n   int
	err error
}

// Read implements io.Reader. Each successful read resets the idle timer.
// If the timer fires before data arrives, the underlying reader is closed
// and an error is returned.
func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	if r.failed != nil {
		return 0, r.failed
	}

	// Race the underlying read against the idle timer.
	// A goroutine is needed because r.rc.Read may block indefinitely
	// (e.g., waiting for the next SSE event from the network).
	// The goroutine reads into a buffer it owns so it never touches p
	// after this call returns. One goroutine per Read is acceptable
	// because SSE streams have low read frequency (one per event frame).
	buf := make([]byte, len(p))
	ch := make(chan readResult, 1)
	go func() {
		n, err := r.rc.Read(buf)
		ch <- readResult{n, err}
	}()

	select {
	case res := <-ch:
		copy(p, buf[:res.n])
		if res.n > 0 {
			// Reset timer on successful read
			if !r.timer.Stop() {
				select {
				case <-r.timer.C:
				default:
				}
			}
			r.timer.Reset(r.timeout)
		}
		if res.err != nil {
			r.timer.Stop()
		}
		return res.n, res.err
	case <-r.timer.C:
		_ = r.rc.Close()
		r.failed = fmt.Errorf("%s: no data received for %s", ErrStreamTimeout, r.timeout)
		return 0, r.failed
	}
}

// parseSSEStream reads SSE frames from r and sends parsed events to the
// returned channel. Each frame is delimited by a blank line. The channel
// is closed when the reader is exhausted or an error occurs.
//
// lightspeed-service sends events as JSON envelopes inside SSE data lines:
//
//	data: {"event": "<type>", "data": <payload>}
//	<blank line>
//
// The parser extracts the event type and re-serializes the inner data
// as the SSEEvent.Data string.
func parseSSEStream(ctx context.Context, rc io.ReadCloser) (<-chan SSEEvent, <-chan error) {
	events := make(chan SSEEvent)
	errc := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errc)
		defer func() { _ = rc.Close() }()

		scanner := bufio.NewScanner(rc)
		// Increase scanner buffer for large tool_result payloads.
		// Default is 64KB; tool_result events with resource dumps
		// (e.g. pods_list) can reach ~35KB observed, ~500KB extreme.
		scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)
		var dataLines []string

		for scanner.Scan() {
			line := scanner.Text()

			// Blank line = end of frame, emit event if we have data
			if line == "" {
				if len(dataLines) > 0 {
					raw := strings.Join(dataLines, "\n")
					ev, err := parseSSEEnvelope(raw)
					if err != nil {
						errc <- fmt.Errorf("%s: %w", ErrParseSSE, err)
						return
					}
					select {
					case events <- ev:
					case <-ctx.Done():
						return
					}
					dataLines = nil
				}
				continue
			}

			// SSE comment lines (starting with :) are ignored
			if strings.HasPrefix(line, ":") {
				continue
			}

			if strings.HasPrefix(line, "data:") {
				// SSE spec: strip exactly one leading space after "data:"
				value := strings.TrimPrefix(line, "data:")
				value = strings.TrimPrefix(value, " ")
				dataLines = append(dataLines, value)
			}
		}

		if err := scanner.Err(); err != nil {
			errc <- fmt.Errorf("%s: %w", ErrParseSSE, err)
		}
	}()

	return events, errc
}

// parseSSEEnvelope extracts the event type and inner data from a
// lightspeed-service JSON envelope: {"event": "<type>", "data": <payload>}.
// The inner data is passed through as-is (json.RawMessage) to preserve
// numeric precision.
func parseSSEEnvelope(raw string) (SSEEvent, error) {
	var envelope sseEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return SSEEvent{}, fmt.Errorf("invalid JSON envelope: %w", err)
	}

	return SSEEvent{
		Type: envelope.Event,
		Data: string(envelope.Data),
	}, nil
}

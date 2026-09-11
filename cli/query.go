package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// commandOptions holds shared configuration for commands that send
// streaming queries to OLS (ask, troubleshoot).
type commandOptions struct {
	streams           genericclioptions.IOStreams
	query             string
	endpoint          string
	kubeConfig        *KubeConfig
	insecureAllowHTTP bool

	// conversationID is extracted from the start event during streaming.
	// Available after queryRun completes for conversation persistence (OLS-3636).
	conversationID string

	// capturedEvents accumulates non-token, non-end events (start, reasoning,
	// tool_call, tool_result). Not displayed in default mode but available
	// for --output json (OLS-3639).
	capturedEvents []SSEEvent
}

// complete resolves the query string, kubeconfig, and endpoint from
// command args and flags.
func (o *commandOptions) complete(cmd *cobra.Command, args []string) error {
	o.query = strings.Join(args, " ")

	kubeconfigPath, _ := cmd.Flags().GetString("kubeconfig")
	contextName, _ := cmd.Flags().GetString("context")
	insecureSkipTLS, _ := cmd.Flags().GetBool("insecure-skip-tls-verify")
	caCertPath, _ := cmd.Flags().GetString("ca-cert")
	insecureAllowHTTP, _ := cmd.Flags().GetBool("insecure-allow-http")

	kc, err := LoadKubeConfig(kubeconfigPath, contextName, insecureSkipTLS, caCertPath)
	if err != nil {
		return err
	}
	o.kubeConfig = kc
	o.insecureAllowHTTP = insecureAllowHTTP

	endpoint, err := ResolveEndpoint(cmd, kc.ContextName)
	if err != nil {
		return err
	}
	o.endpoint = endpoint

	return nil
}

// validate checks that required fields are populated and the endpoint
// URL is well-formed with a valid scheme.
func (o *commandOptions) validate() error {
	if strings.TrimSpace(o.query) == "" {
		return fmt.Errorf("%s: provide a question as arguments", ErrQueryEmpty)
	}
	if o.endpoint == "" {
		return errors.New(ErrNoEndpoint)
	}

	parsed, err := url.Parse(o.endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if parsed.Host == "" {
		return fmt.Errorf("invalid endpoint URL %q: host is required", o.endpoint)
	}
	// Reject anything that isn't HTTPS (or HTTP with explicit opt-in)
	// to prevent sending the bearer token over an insecure transport.
	switch parsed.Scheme {
	case "https":
		// OK
	case "http":
		if !o.insecureAllowHTTP {
			return fmt.Errorf("cleartext HTTP endpoint %q is not allowed: bearer token would be sent unencrypted. Use https:// or reconfigure with: oc ols config set-endpoint", o.endpoint)
		}
	default:
		return fmt.Errorf("unsupported endpoint scheme %q in %q: use https://", parsed.Scheme, o.endpoint)
	}
	return nil
}

// queryRun executes a streaming query against the OLS service. It handles
// SSE event parsing, token output, end event validation, and reference
// display. Both ask and troubleshoot commands delegate to this function.
func queryRun(cmd *cobra.Command, opts *commandOptions, mode string) error {
	client := NewSSEClient(opts.endpoint, opts.kubeConfig.BearerToken, opts.kubeConfig.TLSConfig)

	req := LLMRequest{
		Query:     opts.query,
		Mode:      mode,
		MediaType: "application/json",
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	events, errc, err := client.StreamQuery(ctx, req)
	if err != nil {
		return err
	}

	opts.capturedEvents = nil
	var endData *EndEventData
	var hasTokens bool
	var endParseErr error

	for ev := range events {
		switch ev.Type {
		case EventToken:
			var td TokenEventData
			if err := json.Unmarshal([]byte(ev.Data), &td); err != nil {
				// If token data isn't JSON, use raw string as fallback.
				td.Token = ev.Data
			}
			hasTokens = true
			if _, err := fmt.Fprint(opts.streams.Out, td.Token); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}
		case EventStart:
			// Extract conversation_id for persistence (OLS-3636).
			var sd StartEventData
			if err := json.Unmarshal([]byte(ev.Data), &sd); err == nil {
				opts.conversationID = sd.ConversationID
			}
			opts.capturedEvents = append(opts.capturedEvents, ev)
		case EventEnd:
			var ed EndEventData
			if err := json.Unmarshal([]byte(ev.Data), &ed); err != nil {
				endParseErr = err
			} else {
				endData = &ed
			}
		case EventReasoning, EventToolCall, EventToolResult:
			// Captured but not displayed in default mode.
			// Available via capturedEvents for --output json (OLS-3639).
			opts.capturedEvents = append(opts.capturedEvents, ev)
		default:
			// Unknown event types are silently ignored.
		}
	}

	// Check for stream-level errors.
	if streamErr := <-errc; streamErr != nil {
		if _, err := fmt.Fprintf(opts.streams.ErrOut, "Warning: %s\n", ErrStreamIncomplete); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		return streamErr
	}

	// Print trailing newline only if tokens were emitted.
	if hasTokens {
		if _, err := fmt.Fprintln(opts.streams.Out); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
	}

	// Malformed or missing end event means the stream was not fully valid.
	if endParseErr != nil {
		if _, err := fmt.Fprintf(opts.streams.ErrOut, "Warning: %s: %v\n", ErrMalformedEnd, endParseErr); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		return fmt.Errorf("%s: %w", ErrMalformedEnd, endParseErr)
	}

	if endData == nil {
		if _, err := fmt.Fprintf(opts.streams.ErrOut, "Warning: %s\n", ErrStreamIncomplete); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		return errors.New(ErrMissingEnd)
	}

	// Display referenced documents on stdout.
	if len(endData.ReferencedDocuments) > 0 {
		if _, err := fmt.Fprintf(opts.streams.Out, "\nReferences:\n"); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		for _, doc := range endData.ReferencedDocuments {
			if _, err := fmt.Fprintf(opts.streams.Out, "  - %s: %s\n", doc.DocTitle, doc.DocURL); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}
		}
	}

	return nil
}

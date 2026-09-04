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

const (
	ErrQueryEmpty       = "no query provided"
	ErrNoEndpoint       = "endpoint not resolved"
	ErrStreamIncomplete = "response may be incomplete (stream interrupted)"
	ErrMalformedEnd     = "failed to parse end event"
	ErrMissingEnd       = "stream ended without end event"
)

// AskOptions holds the configuration for the ask command.
type AskOptions struct {
	streams          genericclioptions.IOStreams
	query            string
	endpoint         string
	kubeConfig       *KubeConfig
	mode             string
	insecureAllowHTTP bool
}

// NewAskCmd creates the "ask" subcommand that sends a question to OLS
// and streams back the response.
func NewAskCmd(streams genericclioptions.IOStreams) *cobra.Command {
	o := &AskOptions{
		streams: streams,
		mode:    "ask",
	}

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask OpenShift Lightspeed a question",
		Long:  "Send a question to OpenShift Lightspeed and stream the response.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(cmd)
		},
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
	}

	return cmd
}

// Complete resolves the query string, kubeconfig, and endpoint.
func (o *AskOptions) Complete(cmd *cobra.Command, args []string) error {
	o.query = strings.Join(args, " ")

	kubeconfigPath, _ := cmd.Flags().GetString("kubeconfig")
	contextName, _ := cmd.Flags().GetString("context")
	insecureSkipTLS, _ := cmd.Flags().GetBool("insecure-skip-tls-verify")
	caCertPath, _ := cmd.Flags().GetString("ca-cert")

	kc, err := LoadKubeConfig(kubeconfigPath, contextName, insecureSkipTLS, caCertPath)
	if err != nil {
		return err
	}
	o.kubeConfig = kc
	o.insecureAllowHTTP = insecureSkipTLS

	endpoint, err := ResolveEndpoint(cmd, kc.ContextName)
	if err != nil {
		return err
	}
	o.endpoint = endpoint

	return nil
}

// Validate checks that required fields are populated.
func (o *AskOptions) Validate() error {
	if strings.TrimSpace(o.query) == "" {
		return fmt.Errorf("%s: provide a question as arguments", ErrQueryEmpty)
	}
	if o.endpoint == "" {
		return errors.New(ErrNoEndpoint)
	}
	// Reject cleartext HTTP to prevent sending bearer token unencrypted.
	parsed, err := url.Parse(o.endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if parsed.Scheme == "http" && !o.insecureAllowHTTP {
		return fmt.Errorf("cleartext HTTP endpoint %q is not allowed: bearer token would be sent unencrypted. Use https:// or reconfigure with: oc ols config set-endpoint", o.endpoint)
	}
	return nil
}

// Run executes the ask command: sends the query via SSE and streams
// token data to stdout. Referenced documents from the end event are
// printed to stdout along with the response.
func (o *AskOptions) Run(cmd *cobra.Command) error {
	client := NewSSEClient(o.endpoint, o.kubeConfig.BearerToken, o.kubeConfig.TLSConfig)

	req := LLMRequest{
		Query:     o.query,
		Mode:      o.mode,
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

	var endData *EndEventData
	var hasTokens bool
	var endParseErr error

	for ev := range events {
		switch ev.Type {
		case EventToken:
			var td TokenEventData
			if err := json.Unmarshal([]byte(ev.Data), &td); err != nil {
				// If token data isn't JSON, use raw string as fallback
				td.Token = ev.Data
			}
			hasTokens = true
			if _, err := fmt.Fprint(o.streams.Out, td.Token); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}
		case EventEnd:
			var ed EndEventData
			if err := json.Unmarshal([]byte(ev.Data), &ed); err != nil {
				endParseErr = err
			} else {
				endData = &ed
			}
		// start: conversation_id bookkeeping (used in PR5/OLS-3636)
		// reasoning, tool_call, tool_result: not displayed in default mode.
		// TODO(OLS-3639): accumulate for --output json
		default:
		}
	}

	// Check for stream-level errors
	if streamErr := <-errc; streamErr != nil {
		if _, err := fmt.Fprintf(o.streams.ErrOut, "Warning: %s\n", ErrStreamIncomplete); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		return streamErr
	}

	// Print trailing newline only if tokens were emitted
	if hasTokens {
		if _, err := fmt.Fprintln(o.streams.Out); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
	}

	// Malformed or missing end event means the stream was not fully valid
	if endParseErr != nil {
		if _, err := fmt.Fprintf(o.streams.ErrOut, "Warning: %s: %v\n", ErrMalformedEnd, endParseErr); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		return fmt.Errorf("%s: %w", ErrMalformedEnd, endParseErr)
	}

	if endData == nil {
		if _, err := fmt.Fprintf(o.streams.ErrOut, "Warning: %s\n", ErrStreamIncomplete); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		return errors.New(ErrMissingEnd)
	}

	// Display referenced documents on stdout
	if endData != nil && len(endData.ReferencedDocuments) > 0 {
		if _, err := fmt.Fprintf(o.streams.Out, "\nReferences:\n"); err != nil {
			return fmt.Errorf("%s: %w", ErrWriteOutput, err)
		}
		for _, doc := range endData.ReferencedDocuments {
			if _, err := fmt.Fprintf(o.streams.Out, "  - %s: %s\n", doc.DocTitle, doc.DocURL); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}
		}
	}

	return nil
}

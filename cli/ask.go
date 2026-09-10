package cli

import (
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

// NewAskCmd creates the "ask" subcommand that sends a question to OLS
// and streams back the response.
func NewAskCmd(streams genericclioptions.IOStreams) *cobra.Command {
	o := &commandOptions{streams: streams}

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask OpenShift Lightspeed a question",
		Long:  "Send a question to OpenShift Lightspeed and stream the response.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.complete(cmd, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return queryRun(cmd, o, "ask")
		},
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
	}

	return cmd
}

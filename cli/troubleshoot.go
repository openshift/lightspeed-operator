package cli

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewTroubleshootCmd creates the "troubleshoot" subcommand that sends a
// troubleshooting query to OLS and streams back the response.
func NewTroubleshootCmd(streams genericclioptions.IOStreams) *cobra.Command {
	o := &commandOptions{streams: streams}

	cmd := &cobra.Command{
		Use:   "troubleshoot [question]",
		Short: "Troubleshoot an OpenShift issue with Lightspeed",
		Long:  "Send a troubleshooting question to OpenShift Lightspeed and stream the response.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.complete(cmd, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return queryRun(cmd, o, "troubleshooting")
		},
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
	}

	return cmd
}

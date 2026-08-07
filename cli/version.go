package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Version is overridden at build time via ldflags.
var Version = "dev"

// NewVersionCmd returns a command that prints the CLI version.
func NewVersionCmd(streams genericclioptions.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the plugin version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := fmt.Fprintf(streams.Out, "oc-ols %s\n", Version); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}
			return nil
		},
	}
}

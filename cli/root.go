// Package cli implements the oc-ols kubectl plugin for querying OpenShift Lightspeed from the terminal.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	ErrWriteOutput = "failed to write output"
)

// NewRootCmd creates the root oc-ols command and registers subcommands.
func NewRootCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oc-ols [command]",
		Short: "CLI for OpenShift Lightspeed",
		Long:  "Ask questions and troubleshoot OpenShift clusters using OpenShift Lightspeed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if _, err := fmt.Fprintf(streams.ErrOut, "ask command not yet implemented\n"); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}
			return nil
		},
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
	}

	cmd.SetIn(streams.In)
	cmd.SetOut(streams.Out)
	cmd.SetErr(streams.ErrOut)

	cmd.PersistentFlags().String("kubeconfig", "",
		"Path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	cmd.PersistentFlags().Bool("insecure-skip-tls-verify", false,
		"Skip TLS certificate verification")
	cmd.PersistentFlags().String("context", "",
		"Kubeconfig context to use")
	cmd.PersistentFlags().String("ca-cert", "",
		"Path to CA certificate for TLS verification")

	cmd.AddCommand(NewVersionCmd(streams))

	return cmd
}

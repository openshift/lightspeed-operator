// Package cli implements the oc-ols kubectl plugin for querying OpenShift Lightspeed from the terminal.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/lightspeed-operator/cli/config"
)

const (
	ErrWriteOutput = "failed to write output"
)

// NewRootCmd creates the root oc-ols command and registers subcommands.
func NewRootCmd(streams genericclioptions.IOStreams) *cobra.Command {
	askOpts := &AskOptions{
		streams: streams,
		mode:    "ask",
	}

	cmd := &cobra.Command{
		Use:   "oc-ols [command]",
		Short: "CLI for OpenShift Lightspeed",
		Long:  "Ask questions and troubleshoot OpenShift clusters using OpenShift Lightspeed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			// Default mode: dispatch unrecognized args to ask
			if err := askOpts.Complete(cmd, args); err != nil {
				return err
			}
			if err := askOpts.Validate(); err != nil {
				return err
			}
			return askOpts.Run(cmd)
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

	cmd.PersistentFlags().String("endpoint", "",
		"OLS service endpoint URL (overrides persisted endpoint)")

	cmd.AddCommand(NewVersionCmd(streams))
	cmd.AddCommand(config.NewConfigCmd(streams))
	cmd.AddCommand(NewAskCmd(streams))

	return cmd
}

// ResolveEndpoint determines the OLS service endpoint using the resolution order:
// 1. --endpoint flag (highest priority)
// 2. Persisted endpoint for the current kubeconfig context
// 3. Error with guidance
func ResolveEndpoint(cmd *cobra.Command, contextName string) (string, error) {
	endpoint, err := cmd.Flags().GetString("endpoint")
	if err != nil {
		return "", err
	}
	if endpoint != "" {
		return endpoint, nil
	}

	store, err := config.NewContextStore()
	if err != nil {
		return "", err
	}

	endpoint, err = store.LoadEndpoint(contextName)
	if err == nil {
		return endpoint, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf(
			"no endpoint configured for context %q. Run: oc ols config set-endpoint <URL>",
			contextName,
		)
	}

	return "", err
}

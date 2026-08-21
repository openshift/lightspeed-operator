package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	ErrInvalidURL     = "invalid endpoint URL"
	ErrHTTPNotAllowed = "cleartext HTTP endpoints are not allowed (bearer token would be sent unencrypted). " +
		"Use https:// or pass --insecure-allow-http for development"
	ErrResolveContext = "failed to resolve kubeconfig context"
	ErrWriteOutput    = "failed to write output"
)

// ValidateEndpointURL checks that a URL is valid, has a host, and uses HTTPS
// (or HTTP if allowHTTP is true).
func ValidateEndpointURL(endpoint string, allowHTTP bool) error {
	parsedURL, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return fmt.Errorf("%s %q: %w", ErrInvalidURL, endpoint, err)
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("%s %q: missing host", ErrInvalidURL, endpoint)
	}

	if parsedURL.Scheme == "http" && !allowHTTP {
		return fmt.Errorf("%s", ErrHTTPNotAllowed)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("%s %q: scheme must be http or https", ErrInvalidURL, endpoint)
	}

	return nil
}

// NewSetEndpointCmd creates the "config set-endpoint" subcommand.
func NewSetEndpointCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-endpoint URL",
		Short: "Set the OLS service endpoint for the current kubeconfig context",
		Long: "Set the OLS service endpoint URL for the current kubeconfig context. " +
			"HTTPS is required by default since the bearer token is sent in the Authorization header.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint := strings.TrimSpace(args[0])

			insecureAllowHTTP, err := cmd.Flags().GetBool("insecure-allow-http")
			if err != nil {
				return err
			}

			if err := ValidateEndpointURL(endpoint, insecureAllowHTTP); err != nil {
				return err
			}

			contextName, err := cmd.Flags().GetString("context")
			if err != nil {
				return err
			}
			if contextName == "" {
				contextName, err = resolveCurrentContext(cmd)
				if err != nil {
					return err
				}
			}

			store, err := NewContextStore()
			if err != nil {
				return err
			}

			if err := store.SaveEndpoint(contextName, endpoint); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(streams.Out, "Endpoint set for context %q: %s\n", contextName, endpoint); err != nil {
				return fmt.Errorf("%s: %w", ErrWriteOutput, err)
			}

			return nil
		},
	}

	cmd.Flags().Bool("insecure-allow-http", false,
		"Allow cleartext HTTP endpoints (development only)")

	return cmd
}

// NewConfigCmd creates the "config" subcommand group.
func NewConfigCmd(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage oc-ols configuration",
	}

	cmd.AddCommand(NewSetEndpointCmd(streams))

	return cmd
}

// resolveCurrentContext reads the kubeconfig to determine the active context name.
func resolveCurrentContext(cmd *cobra.Command) (string, error) {
	kubeconfigPath, err := cmd.Flags().GetString("kubeconfig")
	if err != nil {
		return "", err
	}

	rawConfig, err := loadRawKubeConfig(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrResolveContext, err)
	}

	if rawConfig.CurrentContext == "" {
		return "", fmt.Errorf("%s: no current context set in kubeconfig", ErrResolveContext)
	}

	return rawConfig.CurrentContext, nil
}

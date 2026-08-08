package cli

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

const (
	ErrLoadKubeConfig = "failed to load kubeconfig"
	ErrResolveContext = "failed to resolve kubeconfig context"
	ErrReadTokenFile  = "failed to read bearer token file"
	ErrReadCACert     = "failed to read CA certificate"
	ErrInvalidCACert  = "CA certificate contains no valid certificates"
	ErrInvalidCAData  = "kubeconfig CA data contains no valid certificates"
	ErrNoBearerToken  = "kubeconfig context does not provide a bearer token" //#nosec G101 -- error message, not a credential
)

// KubeConfig holds the resolved authentication and TLS configuration
// extracted from a kubeconfig file.
type KubeConfig struct {
	BearerToken string
	TLSConfig   *tls.Config
	ContextName string
}

// LoadKubeConfig reads a kubeconfig file, resolves the current context,
// and extracts the bearer token and TLS settings. oc-ols requires
// token-based auth — client-certificate-only contexts are rejected.
func LoadKubeConfig(kubeconfigPath string, contextName string, insecureSkipTLS bool, caCertPath string) (*KubeConfig, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrLoadKubeConfig, err)
	}

	resolvedContext := rawConfig.CurrentContext
	if contextName != "" {
		resolvedContext = contextName
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", ErrResolveContext, resolvedContext, err)
	}

	token := strings.TrimSpace(restConfig.BearerToken)
	if token == "" && restConfig.BearerTokenFile != "" {
		tokenBytes, err := os.ReadFile(restConfig.BearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ErrReadTokenFile, err)
		}
		token = strings.TrimSpace(string(tokenBytes))
	}

	if token == "" {
		return nil, fmt.Errorf(
			"%s %q: oc-ols requires token-based authentication",
			ErrNoBearerToken, resolvedContext,
		)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipTLS, //#nosec G402 -- user-controlled via --insecure-skip-tls-verify flag
		ServerName:         restConfig.ServerName,
	}

	if !insecureSkipTLS {
		if caCertPath != "" {
			pool, err := loadCACertPool(caCertPath)
			if err != nil {
				return nil, err
			}
			tlsConfig.RootCAs = pool
		} else if len(restConfig.CAData) > 0 {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(restConfig.CAData) {
				return nil, fmt.Errorf("%s", ErrInvalidCAData)
			}
			tlsConfig.RootCAs = pool
		} else if restConfig.CAFile != "" {
			pool, err := loadCACertPool(restConfig.CAFile)
			if err != nil {
				return nil, err
			}
			tlsConfig.RootCAs = pool
		}
	}

	return &KubeConfig{
		BearerToken: token,
		TLSConfig:   tlsConfig,
		ContextName: resolvedContext,
	}, nil
}

// loadCACertPool reads a PEM-encoded CA certificate file and returns a certificate pool.
func loadCACertPool(path string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(path) //#nosec G304 -- path is user-controlled via --ca-cert flag or kubeconfig CAFile
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", ErrReadCACert, path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("%s %q", ErrInvalidCACert, path)
	}
	return pool, nil
}

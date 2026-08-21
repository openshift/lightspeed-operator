package config

import (
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// loadRawKubeConfig loads the raw kubeconfig to resolve context names.
// This is intentionally minimal — full kubeconfig processing (token extraction,
// TLS config) lives in cli.LoadKubeConfig.
func loadRawKubeConfig(kubeconfigPath string) (clientcmdapi.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{})

	return clientConfig.RawConfig()
}

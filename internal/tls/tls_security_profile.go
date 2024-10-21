package tls

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	APIServerName = "cluster"
)

var (
	// DefaultTLSProfileType is the intermediate profile type
	DefaultTLSProfileType = configv1.TLSProfileIntermediateType
	// DefaultTLSCiphers are the default TLS ciphers for API servers
	DefaultTLSCiphers = configv1.TLSProfiles[DefaultTLSProfileType].Ciphers
	// DefaultMinTLSVersion is the default minimum TLS version for API servers
	DefaultMinTLSVersion = configv1.TLSProfiles[DefaultTLSProfileType].MinTLSVersion
)

// FetchAPIServerTlsProfile fetches tlsSecurityProfile configured in APIServer
func FetchAPIServerTlsProfile(k8sClient client.Client) (*configv1.TLSSecurityProfile, error) {
	apiServer := &configv1.APIServer{}
	key := client.ObjectKey{Name: APIServerName}
	if err := k8sClient.Get(context.TODO(), key, apiServer); err != nil {
		return nil, err
	}
	return apiServer.Spec.TLSSecurityProfile, nil
}

// TLSCiphers returns the TLS ciphers for the
// TLS security profile defined in the APIServerConfig.
func TLSCiphers(profile configv1.TLSProfileSpec) []string {
	if len(profile.Ciphers) == 0 {
		return DefaultTLSCiphers
	}
	return profile.Ciphers
}

// MinTLSVersion returns the minimum TLS version for the
// TLS security profile defined in the APIServerConfig.
func MinTLSVersion(profile configv1.TLSProfileSpec) string {
	if profile.MinTLSVersion == "" {
		return string(DefaultMinTLSVersion)
	}
	return string(profile.MinTLSVersion)
}

// GetTLSProfileSpec returns TLSProfileSpec
func GetTLSProfileSpec(profile *configv1.TLSSecurityProfile) configv1.TLSProfileSpec {
	defaultProfile := *configv1.TLSProfiles[DefaultTLSProfileType]
	if profile == nil || profile.Type == "" {
		return defaultProfile
	}
	profileType := profile.Type

	if profileType != configv1.TLSProfileCustomType {
		if tlsConfig, ok := configv1.TLSProfiles[profileType]; ok {
			return *tlsConfig
		}
		return defaultProfile
	}

	if profile.Custom != nil {
		return profile.Custom.TLSProfileSpec
	}

	return defaultProfile
}

package tls

import (
	"context"
	"crypto/tls"

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

// VersionName returns the code for the provided TLS version name
func VersionCode(version configv1.TLSProtocolVersion) uint16 {
	switch version {
	case configv1.VersionTLS10:
		return tls.VersionTLS10
	case configv1.VersionTLS11:
		return tls.VersionTLS11
	case configv1.VersionTLS12:
		return tls.VersionTLS12
	case configv1.VersionTLS13:
		return tls.VersionTLS13
	default:
		defaultProfile := configv1.TLSProfiles[DefaultTLSProfileType]
		return VersionCode(defaultProfile.MinTLSVersion)
	}
}

var CiphersToCodes = map[string]uint16{
	"TLS_AES_128_GCM_SHA256":        tls.TLS_AES_128_GCM_SHA256,
	"TLS_AES_256_GCM_SHA384":        tls.TLS_AES_256_GCM_SHA384,
	"TLS_CHACHA20_POLY1305_SHA256":  tls.TLS_CHACHA20_POLY1305_SHA256,
	"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-ECDSA-AES256-GCM-SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-AES256-GCM-SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-ECDSA-CHACHA20-POLY1305": tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	"ECDHE-RSA-CHACHA20-POLY1305":   tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	"ECDHE-ECDSA-AES128-SHA256":     tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	"ECDHE-RSA-AES128-SHA256":       tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	"ECDHE-ECDSA-AES128-SHA":        tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"ECDHE-RSA-AES128-SHA":          tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"ECDHE-ECDSA-AES256-SHA384":     tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-RSA-AES256-SHA384":       tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"ECDHE-ECDSA-AES256-SHA":        tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"ECDHE-RSA-AES256-SHA":          tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"AES128-GCM-SHA256":             tls.TLS_AES_128_GCM_SHA256,
	"AES256-GCM-SHA384":             tls.TLS_AES_256_GCM_SHA384,
	"AES128-SHA256":                 tls.TLS_AES_128_GCM_SHA256,
	// Please refer to https://pkg.go.dev/crypto/tls#pkg-constants for more information.
	// These ciphers are not supported by Go's TLS implementation:
	// "DHE-RSA-AES128-GCM-SHA256",
	// "DHE-RSA-AES256-GCM-SHA384",
	// "DHE-RSA-CHACHA20-POLY1305",
	// "DHE-RSA-AES128-SHA256",
	// "DHE-RSA-AES256-SHA256",
	// "AES256-SHA256",
	// "AES128-SHA",
	// "AES256-SHA",
	// "DES-CBC3-SHA".
}

func CipherCode(cipher string) uint16 {
	if code, ok := CiphersToCodes[cipher]; ok {
		return code
	}
	return 0
}

func CipherCodes(ciphers []string) (cipherCodes []uint16, unsupportedCiphers []string) {
	for _, cipher := range ciphers {
		code := CipherCode(cipher)
		if code == 0 {
			unsupportedCiphers = append(unsupportedCiphers, cipher)
			continue
		}
		cipherCodes = append(cipherCodes, code)
	}
	return cipherCodes, unsupportedCiphers
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

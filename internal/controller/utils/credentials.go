package utils

import (
	"path"
	"strings"
)

// ProviderSubMountDir returns a filesystem-safe subdirectory name under APIKeyMountRoot
// for mounting a single provider's Vertex credential secret.
func ProviderSubMountDir(providerName string) string {
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		case r == '_':
			return '-'
		default:
			return '-'
		}
	}, providerName)
	s = strings.ToLower(strings.Trim(s, "-"))
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if s == "" {
		return "vertex-provider"
	}
	if len(s) > 63 {
		return s[:63]
	}
	return s
}

// CredentialsVolumeName returns a unique Kubernetes volume name for a provider's
// Vertex credential secret (DNS-1123 label, max 63 characters).
func CredentialsVolumeName(providerName string) string {
	const prefix = "creds-"
	suffix := ProviderSubMountDir(providerName)
	maxSuffix := 63 - len(prefix)
	if len(suffix) > maxSuffix {
		suffix = suffix[:maxSuffix]
	}
	return prefix + suffix
}

// ProviderCredentialsFilePath is the absolute path to the credential file inside
// the LCore container for the given provider name and secret data key (credentialKey).
func ProviderCredentialsFilePath(providerName, credentialKey string) string {
	if credentialKey == "" {
		credentialKey = DefaultCredentialKey
	}
	return path.Join(APIKeyMountRoot, ProviderSubMountDir(providerName), credentialKey)
}

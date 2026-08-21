// Package config implements per-kubeconfig-context local file storage for the oc-ols CLI.
package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	appDirName   = "oc-ols"
	contextDir   = "contexts"
	endpointFile = "endpoint"

	dirPermissions  os.FileMode = 0700
	filePermissions os.FileMode = 0600

	ErrInvalidContextName = "invalid context name"
	ErrSaveEndpoint       = "failed to save endpoint"
	ErrLoadEndpoint       = "failed to load endpoint"
	ErrCreateConfigDir    = "failed to create config directory"
	ErrConfigDir          = "failed to determine config directory"
)

// unsafeChars matches any character not in [a-zA-Z0-9._-].
var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// ContextStore manages per-kubeconfig-context file storage under the user's config directory.
type ContextStore struct {
	baseDir string
}

// NewContextStore creates a ContextStore rooted at <UserConfigDir>/oc-ols/contexts.
func NewContextStore() (*ContextStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrConfigDir, err)
	}
	return &ContextStore{
		baseDir: filepath.Join(configDir, appDirName, contextDir),
	}, nil
}

// NewContextStoreWithBase creates a ContextStore with a custom base directory (for testing).
func NewContextStoreWithBase(baseDir string) *ContextStore {
	return &ContextStore{baseDir: baseDir}
}

// SanitizeContextName replaces unsafe characters with underscores and rejects
// empty names and path traversal attempts. When sanitization modifies the name,
// a short hash of the original is appended to prevent collisions between names
// that differ only in replaced characters (e.g., "a/b" vs "a:b").
func SanitizeContextName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("%s: %q", ErrInvalidContextName, name)
	}
	sanitized := unsafeChars.ReplaceAllString(name, "_")
	if sanitized != name {
		hash := sha256.Sum256([]byte(name))
		sanitized = fmt.Sprintf("%s_%x", sanitized, hash[:4])
	}
	return sanitized, nil
}

// SaveEndpoint persists an endpoint URL for the given kubeconfig context.
// Writes are atomic (temp file + rename) to prevent partial writes.
func (s *ContextStore) SaveEndpoint(contextName string, endpoint string) error {
	dir, err := s.contextDir(contextName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("%s: %w", ErrCreateConfigDir, err)
	}

	target := filepath.Join(dir, endpointFile)

	tmp, err := os.CreateTemp(dir, endpointFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("%s: %w", ErrSaveEndpoint, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(endpoint + "\n"); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", ErrSaveEndpoint, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", ErrSaveEndpoint, err)
	}

	if err := os.Chmod(tmpName, filePermissions); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", ErrSaveEndpoint, err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", ErrSaveEndpoint, err)
	}

	return nil
}

// LoadEndpoint reads the persisted endpoint URL for the given kubeconfig context.
// Returns empty string and an error if no endpoint is configured.
func (s *ContextStore) LoadEndpoint(contextName string) (string, error) {
	dir, err := s.contextDir(contextName)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Join(dir, endpointFile)) //#nosec G304 -- path derived from user's kubeconfig context name
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrLoadEndpoint, err)
	}

	return strings.TrimSpace(string(data)), nil
}

// contextDir returns the storage directory path for a given context name.
func (s *ContextStore) contextDir(contextName string) (string, error) {
	sanitized, err := SanitizeContextName(contextName)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.baseDir, sanitized), nil
}

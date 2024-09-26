package e2e

import (
	"crypto/sha256"
	"fmt"
)

func Ptr[T any](v T) *T { return &v }

func hashBytes(sourceStr []byte) (string, error) { // nolint:unused
	hashFunc := sha256.New()
	_, err := hashFunc.Write(sourceStr)
	if err != nil {
		return "", fmt.Errorf("failed to generate hash %w", err)
	}
	return fmt.Sprintf("%x", hashFunc.Sum(nil)), nil
}

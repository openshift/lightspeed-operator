package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestKubeconfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const testKubeconfigWithToken = `
apiVersion: v1
kind: Config
current-context: test-ctx
clusters:
- cluster:
    server: https://api.test.example.com:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
    namespace: test-ns
  name: test-ctx
users:
- name: test-user
  user:
    token: sha256~testtoken123
`

const testKubeconfigNoToken = `
apiVersion: v1
kind: Config
current-context: cert-ctx
clusters:
- cluster:
    server: https://api.test.example.com:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: cert-user
  name: cert-ctx
users:
- name: cert-user
  user:
    client-certificate-data: dGVzdA==
    client-key-data: dGVzdA==
`

func TestLoadKubeConfig_ExtractsToken(t *testing.T) {
	path := writeTestKubeconfig(t, testKubeconfigWithToken)
	kc, err := LoadKubeConfig(path, "", false, "")
	if err != nil {
		t.Fatalf("LoadKubeConfig: %v", err)
	}
	if kc.BearerToken != "sha256~testtoken123" {
		t.Errorf("expected token 'sha256~testtoken123', got %q", kc.BearerToken)
	}
	if kc.ContextName != "test-ctx" {
		t.Errorf("expected context 'test-ctx', got %q", kc.ContextName)
	}
}

func TestLoadKubeConfig_NoTokenErrors(t *testing.T) {
	path := writeTestKubeconfig(t, testKubeconfigNoToken)
	_, err := LoadKubeConfig(path, "", false, "")
	if err == nil {
		t.Fatal("expected error for kubeconfig without bearer token")
	}
}

func TestLoadKubeConfig_InsecureSkipTLS(t *testing.T) {
	path := writeTestKubeconfig(t, testKubeconfigWithToken)
	kc, err := LoadKubeConfig(path, "", true, "")
	if err != nil {
		t.Fatalf("LoadKubeConfig: %v", err)
	}
	if !kc.TLSConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true")
	}
}

func TestLoadKubeConfig_InvalidPath(t *testing.T) {
	_, err := LoadKubeConfig("/nonexistent/kubeconfig", "", false, "")
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig")
	}
}

func TestLoadKubeConfig_CustomCACert(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")

	// Generate a fresh self-signed CA cert for testing
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(caPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}

	path := writeTestKubeconfig(t, testKubeconfigWithToken)
	kc, err := LoadKubeConfig(path, "", false, caPath)
	if err != nil {
		t.Fatalf("LoadKubeConfig: %v", err)
	}
	if kc.TLSConfig.RootCAs == nil {
		t.Error("expected RootCAs to be set when CA cert provided")
	}
}

func TestLoadKubeConfig_TokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-based-token"), 0600); err != nil {
		t.Fatal(err)
	}

	kubeconfig := `
apiVersion: v1
kind: Config
current-context: sa-ctx
clusters:
- cluster:
    server: https://api.test.example.com:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: sa-user
  name: sa-ctx
users:
- name: sa-user
  user:
    tokenFile: ` + tokenPath + `
`
	path := writeTestKubeconfig(t, kubeconfig)
	kc, err := LoadKubeConfig(path, "", false, "")
	if err != nil {
		t.Fatalf("LoadKubeConfig: %v", err)
	}
	if kc.BearerToken != "file-based-token" {
		t.Errorf("expected token 'file-based-token', got %q", kc.BearerToken)
	}
}

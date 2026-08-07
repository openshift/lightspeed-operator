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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func writeTestKubeconfig(content string) string {
	dir := GinkgoT().TempDir()
	path := filepath.Join(dir, "kubeconfig")
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
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

var _ = Describe("LoadKubeConfig", func() {
	It("extracts the bearer token and context name", func() {
		path := writeTestKubeconfig(testKubeconfigWithToken)
		kc, err := LoadKubeConfig(path, "", false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(kc.BearerToken).To(Equal("sha256~testtoken123"))
		Expect(kc.ContextName).To(Equal("test-ctx"))
	})

	It("returns an error for kubeconfig without bearer token", func() {
		path := writeTestKubeconfig(testKubeconfigNoToken)
		_, err := LoadKubeConfig(path, "", false, "")
		Expect(err).To(HaveOccurred())
	})

	It("sets InsecureSkipVerify when requested", func() {
		path := writeTestKubeconfig(testKubeconfigWithToken)
		kc, err := LoadKubeConfig(path, "", true, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(kc.TLSConfig.InsecureSkipVerify).To(BeTrue())
	})

	It("returns an error for nonexistent kubeconfig", func() {
		_, err := LoadKubeConfig("/nonexistent/kubeconfig", "", false, "")
		Expect(err).To(HaveOccurred())
	})

	It("loads a custom CA certificate", func() {
		dir := GinkgoT().TempDir()
		caPath := filepath.Join(dir, "ca.crt")

		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).NotTo(HaveOccurred())
		template := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{Organization: []string{"Test"}},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
		}
		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		Expect(err).NotTo(HaveOccurred())
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		Expect(os.WriteFile(caPath, certPEM, 0600)).To(Succeed())

		path := writeTestKubeconfig(testKubeconfigWithToken)
		kc, err := LoadKubeConfig(path, "", false, caPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(kc.TLSConfig.RootCAs).NotTo(BeNil())
	})

	It("reads token from file and trims whitespace", func() {
		dir := GinkgoT().TempDir()
		tokenPath := filepath.Join(dir, "token")
		Expect(os.WriteFile(tokenPath, []byte("file-based-token\n"), 0600)).To(Succeed())

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
		path := writeTestKubeconfig(kubeconfig)
		kc, err := LoadKubeConfig(path, "", false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(kc.BearerToken).To(Equal("file-based-token"))
	})

	It("preserves tls-server-name from kubeconfig", func() {
		kubeconfig := `
apiVersion: v1
kind: Config
current-context: sni-ctx
clusters:
- cluster:
    server: https://api.test.example.com:6443
    tls-server-name: custom-sni.example.com
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: sni-ctx
users:
- name: test-user
  user:
    token: sha256~testtoken123
`
		path := writeTestKubeconfig(kubeconfig)
		kc, err := LoadKubeConfig(path, "", false, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(kc.TLSConfig.ServerName).To(Equal("custom-sni.example.com"))
	})
})

package config

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func fakeStreams() (genericclioptions.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	streams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    out,
		ErrOut: errOut,
	}
	return streams, out, errOut
}

// writeKubeconfig creates a minimal kubeconfig file with the given context name.
func writeKubeconfig(dir, contextName string) string {
	path := filepath.Join(dir, "kubeconfig")
	content := `apiVersion: v1
kind: Config
current-context: ` + contextName + `
contexts:
- name: ` + contextName + `
  context:
    cluster: test-cluster
clusters:
- name: test-cluster
  cluster:
    server: https://api.test.example.com:6443
`
	Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
	return path
}

var _ = Describe("SetEndpointCmd", func() {
	var (
		tmpDir string
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "oc-ols-endpoint-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("saves an HTTPS endpoint and prints confirmation", func() {
		kubeconfigPath := writeKubeconfig(tmpDir, "my-cluster")
		streams, out, _ := fakeStreams()
		cmd := NewConfigCmd(streams)

		// Inherit kubeconfig flag from a parent (simulating root command)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		// Override config dir to use temp directory
		origUserConfigDir, hadConfigDir := os.LookupEnv("XDG_CONFIG_HOME")
		Expect(os.Setenv("XDG_CONFIG_HOME", tmpDir)).To(Succeed())
		defer func() {
			if !hadConfigDir {
				Expect(os.Unsetenv("XDG_CONFIG_HOME")).To(Succeed())
			} else {
				Expect(os.Setenv("XDG_CONFIG_HOME", origUserConfigDir)).To(Succeed())
			}
		}()

		cmd.SetArgs([]string{"set-endpoint", "https://ols.example.com", "--kubeconfig", kubeconfigPath})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring(`Endpoint set for context "my-cluster"`))
		Expect(out.String()).To(ContainSubstring("https://ols.example.com"))
	})

	It("rejects HTTP endpoints without --insecure-allow-http", func() {
		kubeconfigPath := writeKubeconfig(tmpDir, "my-cluster")
		streams, _, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		cmd.SetArgs([]string{"set-endpoint", "http://ols.example.com", "--kubeconfig", kubeconfigPath})
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrHTTPNotAllowed))
	})

	It("allows HTTP endpoints with --insecure-allow-http", func() {
		kubeconfigPath := writeKubeconfig(tmpDir, "my-cluster")
		streams, out, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		origUserConfigDir, hadConfigDir := os.LookupEnv("XDG_CONFIG_HOME")
		Expect(os.Setenv("XDG_CONFIG_HOME", tmpDir)).To(Succeed())
		defer func() {
			if !hadConfigDir {
				Expect(os.Unsetenv("XDG_CONFIG_HOME")).To(Succeed())
			} else {
				Expect(os.Setenv("XDG_CONFIG_HOME", origUserConfigDir)).To(Succeed())
			}
		}()

		cmd.SetArgs([]string{"set-endpoint", "http://localhost:8080", "--insecure-allow-http", "--kubeconfig", kubeconfigPath})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("http://localhost:8080"))
	})

	It("rejects invalid URLs", func() {
		streams, _, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		cmd.SetArgs([]string{"set-endpoint", "not-a-url"})
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrInvalidURL))
	})

	It("rejects URLs with empty host", func() {
		streams, _, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		cmd.SetArgs([]string{"set-endpoint", "https://"})
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrInvalidURL))
		Expect(err.Error()).To(ContainSubstring("missing host"))
	})

	It("rejects non-HTTP schemes", func() {
		streams, _, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		cmd.SetArgs([]string{"set-endpoint", "ftp://ols.example.com"})
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(ErrInvalidURL))
	})

	It("uses --context flag when provided", func() {
		kubeconfigPath := writeKubeconfig(tmpDir, "default-ctx")
		streams, out, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		origUserConfigDir, hadConfigDir := os.LookupEnv("XDG_CONFIG_HOME")
		Expect(os.Setenv("XDG_CONFIG_HOME", tmpDir)).To(Succeed())
		defer func() {
			if !hadConfigDir {
				Expect(os.Unsetenv("XDG_CONFIG_HOME")).To(Succeed())
			} else {
				Expect(os.Setenv("XDG_CONFIG_HOME", origUserConfigDir)).To(Succeed())
			}
		}()

		cmd.SetArgs([]string{"set-endpoint", "https://ols.example.com", "--kubeconfig", kubeconfigPath, "--context", "override-ctx"})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring(`Endpoint set for context "override-ctx"`))
	})

	It("requires exactly one argument", func() {
		streams, _, _ := fakeStreams()
		cmd := NewConfigCmd(streams)
		cmd.PersistentFlags().String("kubeconfig", "", "")
		cmd.PersistentFlags().String("context", "", "")

		cmd.SetArgs([]string{"set-endpoint"})
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
	})
})

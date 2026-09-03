package cli

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/lightspeed-operator/cli/config"
)

var _ = Describe("RootCmd", func() {
	It("routes the version subcommand", func() {
		streams, out, _ := fakeStreams()
		cmd := NewRootCmd(streams)
		cmd.SetArgs([]string{"version"})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("oc-ols"))
	})

	It("shows help when no args are given", func() {
		streams, out, _ := fakeStreams()
		cmd := NewRootCmd(streams)
		cmd.SetArgs([]string{})
		_ = cmd.Execute()
		Expect(out.Len()).NotTo(BeZero())
	})

	It("registers global flags", func() {
		streams, _, _ := fakeStreams()
		cmd := NewRootCmd(streams)
		for _, name := range []string{"kubeconfig", "context", "insecure-skip-tls-verify", "ca-cert", "endpoint"} {
			Expect(cmd.PersistentFlags().Lookup(name)).NotTo(BeNil(), "expected persistent flag %q", name)
		}
	})

	It("routes the config subcommand", func() {
		streams, out, _ := fakeStreams()
		cmd := NewRootCmd(streams)
		cmd.SetArgs([]string{"config", "--help"})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("set-endpoint"))
	})

	Describe("ResolveEndpoint", func() {
		It("resolves endpoint from --endpoint flag", func() {
			streams, _, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			// Execute help to trigger flag merging, then test resolution
			cmd.SetArgs([]string{"--endpoint", "https://flag.example.com"})
			_ = cmd.Execute()

			endpoint, err := ResolveEndpoint(cmd, "any-context")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("https://flag.example.com"))
		})

		It("resolves endpoint from persisted store", func() {
			tmpDir, err := os.MkdirTemp("", "oc-ols-resolve-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(os.RemoveAll(tmpDir)).To(Succeed()) }()

			// Save endpoint under the path that NewContextStore() will look for:
			// <XDG_CONFIG_HOME>/oc-ols/contexts/<context-name>/endpoint
			store := config.NewContextStoreWithBase(tmpDir + "/oc-ols/contexts")
			Expect(store.SaveEndpoint("test-ctx", "https://persisted.example.com")).To(Succeed())

			origConfigDir, hadConfigDir := os.LookupEnv("XDG_CONFIG_HOME")
			Expect(os.Setenv("XDG_CONFIG_HOME", tmpDir)).To(Succeed())
			defer func() {
				if !hadConfigDir {
					Expect(os.Unsetenv("XDG_CONFIG_HOME")).To(Succeed())
				} else {
					Expect(os.Setenv("XDG_CONFIG_HOME", origConfigDir)).To(Succeed())
				}
			}()

			streams, _, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			cmd.SetArgs([]string{})
			_ = cmd.Execute()

			endpoint, err := ResolveEndpoint(cmd, "test-ctx")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("https://persisted.example.com"))
		})

		It("returns guidance error when no endpoint is configured", func() {
			streams, _, _ := fakeStreams()
			cmd := NewRootCmd(streams)
			cmd.SetArgs([]string{})
			_ = cmd.Execute()

			_, err := ResolveEndpoint(cmd, "unconfigured-ctx")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`no endpoint configured for context "unconfigured-ctx"`))
			Expect(err.Error()).To(ContainSubstring("Run: oc ols config set-endpoint <URL>"))
		})
	})

	It("dispatches unrecognized args to ask mode", func() {
		// Point to a non-existent kubeconfig so the test doesn't pick up
		// the real one from the environment.
		streams, _, _ := fakeStreams()
		cmd := NewRootCmd(streams)
		cmd.SetArgs([]string{"--kubeconfig", "/nonexistent/kubeconfig", "why is my pod crashing"})
		// Without a valid kubeconfig, this will fail at Complete() —
		// but it proves dispatch happened (not "unknown command" error)
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		// Should be a kubeconfig/auth error, not an "unknown command" error
		Expect(err.Error()).NotTo(ContainSubstring("unknown command"))
	})
})

package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		for _, name := range []string{"kubeconfig", "insecure-skip-tls-verify", "ca-cert"} {
			Expect(cmd.PersistentFlags().Lookup(name)).NotTo(BeNil(), "expected persistent flag %q", name)
		}
	})

	It("dispatches unrecognized args to the default mode stub", func() {
		streams, _, errOut := fakeStreams()
		cmd := NewRootCmd(streams)
		cmd.SetArgs([]string{"why is my pod crashing"})
		Expect(cmd.Execute()).To(Succeed())
		Expect(errOut.String()).To(ContainSubstring("not yet implemented"))
	})
})

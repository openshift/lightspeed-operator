package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("VersionCmd", func() {
	It("prints the default version", func() {
		streams, out, _ := fakeStreams()
		cmd := NewVersionCmd(streams)
		cmd.SetArgs([]string{})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("oc-ols dev"))
	})

	It("prints an injected version", func() {
		original := Version
		defer func() { Version = original }()

		Version = "v1.2.3-abc"
		streams, out, _ := fakeStreams()
		cmd := NewVersionCmd(streams)
		cmd.SetArgs([]string{})
		Expect(cmd.Execute()).To(Succeed())
		Expect(out.String()).To(ContainSubstring("oc-ols v1.2.3-abc"))
	})
})

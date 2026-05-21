package appserver

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("RHOKP sidecar assets", func() {
	It("should disable Apache Listen 8443 before starting the RHOKP entrypoint", func() {
		Expect(rhokpContainerCommand()).To(Equal([]string{"/bin/sh", "-c"}))
		Expect(rhokpContainerArgs()[0]).To(ContainSubstring(utils.RHOOKPHTTPDSSLConfPath))
		Expect(rhokpContainerArgs()[0]).To(ContainSubstring("Listen 0.0.0.0:8443"))
		Expect(rhokpContainerArgs()[0]).To(ContainSubstring(utils.RHOOKPContainerEntrypoint))
		Expect(rhokpContainerArgs()[0]).To(ContainSubstring(utils.RHOOKPMainCommand))
	})

	It("should wire optional ACCESS_KEY from rhokp-access-key secret", func() {
		env := generateRHOOKPEnv()
		Expect(env).To(HaveLen(1))
		Expect(env[0].Name).To(Equal("ACCESS_KEY"))
		Expect(env[0].ValueFrom.SecretKeyRef.Name).To(Equal(utils.RHOOKPAccessKeySecretName))
		Expect(env[0].ValueFrom.SecretKeyRef.Key).To(Equal(utils.RHOOKPAccessKeySecretKey))
		Expect(*env[0].ValueFrom.SecretKeyRef.Optional).To(BeTrue())
	})
})

package appserver

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("RHOKP sidecar assets", func() {
	It("should remap RHOKP Apache ports before starting the RHOKP entrypoint", func() {
		Expect(rhokpContainerCommand()).To(Equal([]string{"/bin/sh", "-c"}))
		script := rhokpContainerArgs()[0]
		Expect(script).To(ContainSubstring(utils.RHOOKPHTTPDConfPath))
		Expect(script).To(ContainSubstring(utils.RHOOKPHTTPDSSLConfPath))
		Expect(script).To(ContainSubstring(fmt.Sprintf("Listen %d/Listen %d/", utils.RHOOKPImageHTTPPort, utils.RHOOKPHTTPPort)))
		Expect(script).To(ContainSubstring(fmt.Sprintf("0.0.0.0:%d https/Listen 0.0.0.0:%d https/", utils.RHOOKPImageHTTPSPort, utils.RHOOKPHTTPSPort)))
		Expect(script).To(ContainSubstring(fmt.Sprintf("_default_:%d/_default_:%d/", utils.RHOOKPImageHTTPSPort, utils.RHOOKPHTTPSPort)))
		Expect(script).To(ContainSubstring(utils.RHOOKPContainerEntrypoint))
		Expect(script).To(ContainSubstring(utils.RHOOKPMainCommand))
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

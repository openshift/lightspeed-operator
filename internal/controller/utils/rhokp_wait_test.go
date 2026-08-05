package utils

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("RHOKP wait", func() {

	Describe("GenerateRHOKPWaitInitContainer", func() {
		const testNS = "openshift-lightspeed"

		It("uses the provided image and generates correct container spec", func() {
			c := GenerateRHOKPWaitInitContainer(OLSAppServerImageDefault, testNS)
			Expect(c.Name).To(Equal(RHOKPWaitInitContainerName))
			Expect(c.Image).To(Equal(OLSAppServerImageDefault))
			Expect(c.SecurityContext).To(Equal(RestrictedContainerSecurityContext()))
			Expect(c.Resources.Requests).To(HaveKey(corev1.ResourceCPU))
			Expect(c.Resources.Requests).To(HaveKey(corev1.ResourceMemory))
			Expect(c.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("10m")))
			Expect(c.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("32Mi")))
			Expect(c.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(c.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("64Mi")))
			Expect(c.Command).To(HaveLen(3))
		})

		It("mounts the RHOKP CA certificate volume", func() {
			c := GenerateRHOKPWaitInitContainer(OLSAppServerImageDefault, testNS)
			Expect(c.VolumeMounts).To(HaveLen(1))
			Expect(c.VolumeMounts[0].Name).To(Equal(AppRHOKPCACertVolumeName))
			Expect(c.VolumeMounts[0].MountPath).To(Equal(path.Join(OLSAppCertsMountRoot, AppRHOKPCACertDir)))
			Expect(c.VolumeMounts[0].ReadOnly).To(BeTrue())
		})

		It("contains curl-based readiness check using admin/ping endpoint", func() {
			c := GenerateRHOKPWaitInitContainer(OLSAppServerImageDefault, testNS)
			script := c.Command[2]

			By("using curl for connectivity check")
			Expect(script).To(ContainSubstring("curl"))
			Expect(script).To(ContainSubstring("--cacert"))

			By("targeting the RHOKP admin/ping endpoint")
			expectedURL := RHOKPServiceURL(testNS) + RHOOKPReadinessHTTPPath
			Expect(script).To(ContainSubstring(expectedURL))
		})

		It("guards against missing curl binary", func() {
			c := GenerateRHOKPWaitInitContainer(OLSAppServerImageDefault, testNS)
			script := c.Command[2]
			Expect(script).To(ContainSubstring("command -v curl"))
			Expect(script).To(ContainSubstring("curl not found in image"))
		})

		It("has timeout and backoff logic", func() {
			c := GenerateRHOKPWaitInitContainer(OLSAppServerImageDefault, testNS)
			script := c.Command[2]

			Expect(script).To(ContainSubstring(fmt.Sprintf("max_elapsed=%d", RHOKPWaitMaxSeconds)))
			Expect(script).To(ContainSubstring("backoff"))
			Expect(script).To(ContainSubstring("timed out"))
			Expect(script).To(ContainSubstring("not ready yet"))
			Expect(script).To(ContainSubstring("RHOKP/Solr is reachable"))
		})

		It("uses the correct namespace in the service URL", func() {
			customNS := "my-custom-namespace"
			c := GenerateRHOKPWaitInitContainer(OLSAppServerImageDefault, customNS)
			script := c.Command[2]
			Expect(script).To(ContainSubstring(fmt.Sprintf("%s.%s.svc", RHOKPServiceName, customNS)))
		})
	})
})

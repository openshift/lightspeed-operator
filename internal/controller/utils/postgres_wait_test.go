package utils

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("Postgres wait", func() {

	Describe("GeneratePostgresWaitInitContainer", func() {
		It("uses the provided image and generates correct container spec", func() {
			c := GeneratePostgresWaitInitContainer(PostgresServerImageDefault)
			Expect(c.Name).To(Equal(PostgresWaitInitContainerName))
			Expect(c.Image).To(Equal(PostgresServerImageDefault))
			Expect(c.SecurityContext).NotTo(BeNil())
			Expect(*c.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
			Expect(*c.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(c.Resources.Requests).To(HaveKey(corev1.ResourceCPU))
			Expect(c.Resources.Requests).To(HaveKey(corev1.ResourceMemory))
			Expect(c.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("10m")))
			Expect(c.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("32Mi")))
			Expect(c.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(c.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("64Mi")))
			Expect(c.VolumeMounts).To(BeEmpty())
			Expect(c.Command).To(HaveLen(3))
		})

		It("contains pg_isready based readiness check", func() {
			c := GeneratePostgresWaitInitContainer(PostgresServerImageDefault)
			script := c.Command[2]

			By("using pg_isready for connectivity check")
			Expect(script).To(ContainSubstring("pg_isready"))
			Expect(script).To(ContainSubstring("pg_isready -q"))

			By("checking for pg_isready availability")
			Expect(script).To(ContainSubstring("command -v pg_isready"))
			Expect(script).To(ContainSubstring("pg_isready not found in image"))

			By("using correct default connection parameters")
			Expect(script).To(ContainSubstring(PostgresServiceName))
			Expect(script).To(ContainSubstring(fmt.Sprintf("%d", PostgresServicePort)))
			Expect(script).To(ContainSubstring("PGUSER"))
			Expect(script).To(ContainSubstring("PGDATABASE"))
		})

		It("has timeout and backoff logic", func() {
			c := GeneratePostgresWaitInitContainer(PostgresServerImageDefault)
			script := c.Command[2]

			Expect(script).To(ContainSubstring(fmt.Sprintf("max_elapsed=%d", PostgresWaitMaxSeconds)))
			Expect(script).To(ContainSubstring("backoff"))
			Expect(script).To(ContainSubstring("timed out"))
			Expect(script).To(ContainSubstring("not ready yet"))
			Expect(script).To(ContainSubstring("accepting connections"))
		})
	})
})

package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SanitizeContextName", func() {
	DescribeTable("valid context names",
		func(input, expected string) {
			result, err := SanitizeContextName(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expected))
		},
		Entry("simple name", "my-cluster", "my-cluster"),
		Entry("name with dots", "api.cluster.example.com", "api.cluster.example.com"),
		Entry("name with colons", "admin:my-cluster", "admin_my-cluster_7b3dc15f"),
		Entry("name with slashes", "context/with/slashes", "context_with_slashes_f4581c7c"),
		Entry("path traversal attempt", "../../etc/passwd", ".._.._etc_passwd_3754d6cb"),
		Entry("unicode characters", "клáстер", "________6b6c5c25"),
		Entry("spaces replaced", "my cluster", "my_cluster_f3c5e5c4"),
		Entry("leading/trailing spaces trimmed", "  my-cluster  ", "my-cluster"),
	)

	It("produces different keys for names that differ only in replaced characters", func() {
		result1, err := SanitizeContextName("a/b")
		Expect(err).NotTo(HaveOccurred())
		result2, err := SanitizeContextName("a:b")
		Expect(err).NotTo(HaveOccurred())
		Expect(result1).NotTo(Equal(result2))
	})

	DescribeTable("invalid context names",
		func(input string) {
			_, err := SanitizeContextName(input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrInvalidContextName))
		},
		Entry("empty string", ""),
		Entry("whitespace only", "   "),
		Entry("single dot", "."),
		Entry("double dot", ".."),
	)
})

var _ = Describe("ContextStore", func() {
	var (
		tmpDir string
		store  *ContextStore
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "oc-ols-test-*")
		Expect(err).NotTo(HaveOccurred())
		store = NewContextStoreWithBase(tmpDir)
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Describe("SaveEndpoint and LoadEndpoint", func() {
		It("persists and retrieves an endpoint", func() {
			Expect(store.SaveEndpoint("my-cluster", "https://ols.example.com")).To(Succeed())

			endpoint, err := store.LoadEndpoint("my-cluster")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("https://ols.example.com"))
		})

		It("overwrites an existing endpoint", func() {
			Expect(store.SaveEndpoint("my-cluster", "https://old.example.com")).To(Succeed())
			Expect(store.SaveEndpoint("my-cluster", "https://new.example.com")).To(Succeed())

			endpoint, err := store.LoadEndpoint("my-cluster")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("https://new.example.com"))
		})

		It("isolates endpoints by context name", func() {
			Expect(store.SaveEndpoint("cluster-a", "https://a.example.com")).To(Succeed())
			Expect(store.SaveEndpoint("cluster-b", "https://b.example.com")).To(Succeed())

			endpointA, err := store.LoadEndpoint("cluster-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpointA).To(Equal("https://a.example.com"))

			endpointB, err := store.LoadEndpoint("cluster-b")
			Expect(err).NotTo(HaveOccurred())
			Expect(endpointB).To(Equal("https://b.example.com"))
		})

		It("returns an error when no endpoint is configured", func() {
			_, err := store.LoadEndpoint("unconfigured-context")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrLoadEndpoint))
		})

		It("returns an error for invalid context names", func() {
			err := store.SaveEndpoint("", "https://ols.example.com")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrInvalidContextName))

			_, err = store.LoadEndpoint("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(ErrInvalidContextName))
		})

		It("creates directories with restrictive permissions", func() {
			Expect(store.SaveEndpoint("secure-ctx", "https://ols.example.com")).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpDir, "secure-ctx"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(dirPermissions))
		})

		It("creates endpoint file with restrictive permissions", func() {
			Expect(store.SaveEndpoint("secure-ctx", "https://ols.example.com")).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpDir, "secure-ctx", endpointFile))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(filePermissions))
		})
	})
})

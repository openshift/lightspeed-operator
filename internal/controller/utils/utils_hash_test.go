package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hash Functions", func() {
	Describe("HashBytes", func() {
		It("should generate consistent hash for same input", func() {
			input := []byte("test-data")
			hash1, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash1).NotTo(BeEmpty())

			hash2, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash2).NotTo(BeEmpty())

			Expect(hash1).To(Equal(hash2))
		})

		It("should generate different hashes for different inputs", func() {
			input1 := []byte("test-data-1")
			hash1, err := HashBytes(input1)
			Expect(err).NotTo(HaveOccurred())

			input2 := []byte("test-data-2")
			hash2, err := HashBytes(input2)
			Expect(err).NotTo(HaveOccurred())

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should generate SHA256 hash with correct length", func() {
			input := []byte("test-data")
			hash, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())

			// SHA256 produces a 64-character hex string
			Expect(hash).To(HaveLen(64))
		})

		It("should handle empty input", func() {
			input := []byte("")
			hash, err := HashBytes(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).NotTo(BeEmpty())
		})
	})
})

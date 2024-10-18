package tls

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
)

var _ = Describe("TlsSecurityProfile", func() {
	Context("TLSCiphers", func() {
		It("should return the default ciphers when none are defined", func() {
			Expect(TLSCiphers(configv1.TLSProfileSpec{})).To(BeEquivalentTo(DefaultTLSCiphers))
		})
		It("should return the profile ciphers when they are defined", func() {
			Expect(TLSCiphers(configv1.TLSProfileSpec{Ciphers: []string{"a", "b"}})).To(Equal([]string{"a", "b"}))
		})
	})

	Context("MinTLSVersion", func() {
		It("should return the default min TLS version when not defined", func() {
			Expect(string(DefaultMinTLSVersion)).To(Equal(MinTLSVersion(configv1.TLSProfileSpec{})))
		})
		It("should return the profile min TLS version when defined", func() {
			Expect(string(configv1.VersionTLS13)).To(Equal(MinTLSVersion(configv1.TLSProfileSpec{MinTLSVersion: configv1.VersionTLS13})))
		})
	})

	Context("GetClusterTLSProfileSpec", func() {
		It("should return the default profile when no profile is defined", func() {
			Expect(GetTLSProfileSpec(nil)).To(Equal(*configv1.TLSProfiles[DefaultTLSProfileType]))
		})
		It("should return the default profile when the profile type is empty", func() {
			Expect(GetTLSProfileSpec(&configv1.TLSSecurityProfile{})).To(Equal(*configv1.TLSProfiles[DefaultTLSProfileType]))
		})
		It("should return the custom profile when the profile type is custom", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"ECDHE-ECDSA-CHACHA20-POLY1305", "ECDHE-RSA-CHACHA20-POLY1305"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}
			Expect(GetTLSProfileSpec(profile)).To(Equal(configv1.TLSProfileSpec{
				Ciphers:       []string{"ECDHE-ECDSA-CHACHA20-POLY1305", "ECDHE-RSA-CHACHA20-POLY1305"},
				MinTLSVersion: configv1.VersionTLS13,
			}))
		})
	})
})

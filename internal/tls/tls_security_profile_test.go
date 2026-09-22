package tls

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	Context("ResolveTLSProfile", func() {
		It("should use the explicitly configured profile", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS13,
					Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
				}},
			}

			resolved, err := ResolveTLSProfile(nil, profile)

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(&ResolvedTLSProfile{
				ProfileType:   "Custom",
				MinTLSVersion: string(configv1.VersionTLS13),
				Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
			}))
		})

		It("should use the APIServer profile when the configured profile is incomplete", func() {
			scheme := runtime.NewScheme()
			Expect(configv1.AddToScheme(scheme)).To(Succeed())
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: APIServerName},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()
			configured := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType}

			resolved, err := ResolveTLSProfile(k8sClient, configured)

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.ProfileType).To(Equal("ModernType"))
		})

		It("should use the APIServer profile when no profile is configured", func() {
			scheme := runtime.NewScheme()
			Expect(configv1.AddToScheme(scheme)).To(Succeed())
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: APIServerName},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

			resolved, err := ResolveTLSProfile(k8sClient, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.ProfileType).To(Equal("ModernType"))
		})

		It("should use the default profile when the APIServer profile is incomplete", func() {
			scheme := runtime.NewScheme()
			Expect(configv1.AddToScheme(scheme)).To(Succeed())
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: APIServerName},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

			resolved, err := ResolveTLSProfile(k8sClient, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.ProfileType).To(Equal("IntermediateType"))
		})

		It("should use the default profile when the APIServer profile cannot be fetched", func() {
			scheme := runtime.NewScheme()
			Expect(configv1.AddToScheme(scheme)).To(Succeed())
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			resolved, err := ResolveTLSProfile(k8sClient, nil)

			Expect(err).To(HaveOccurred())
			Expect(resolved.ProfileType).To(Equal("IntermediateType"))
		})
	})
})

package tls

import (
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("TlsSecurityProfile", func() {
	Context("FetchAPIServerTlsProfile", func() {
		var scheme *runtime.Scheme

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(configv1.AddToScheme(scheme)).To(Succeed())
		})

		It("returns the cluster API server's configured profile", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: APIServerName},
				Spec:       configv1.APIServerSpec{TLSSecurityProfile: profile},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

			got, err := FetchAPIServerTlsProfile(k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(profile))
		})

		It("returns a not-found error when the API server object is missing", func() {
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			got, err := FetchAPIServerTlsProfile(k8sClient)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(got).To(BeNil())
		})
	})

	Context("VersionCode", func() {
		DescribeTable("maps TLS versions to Go constants", func(version configv1.TLSProtocolVersion, want uint16) {
			Expect(VersionCode(version)).To(Equal(want))
		},
			Entry("TLS 1.0", configv1.VersionTLS10, uint16(tls.VersionTLS10)),
			Entry("TLS 1.1", configv1.VersionTLS11, uint16(tls.VersionTLS11)),
			Entry("TLS 1.2", configv1.VersionTLS12, uint16(tls.VersionTLS12)),
			Entry("TLS 1.3", configv1.VersionTLS13, uint16(tls.VersionTLS13)),
			Entry("unknown version uses the default profile", configv1.TLSProtocolVersion("unknown"), uint16(tls.VersionTLS12)),
		)
	})

	Context("CipherCodes", func() {
		It("maps supported cipher names and rejects unknown names", func() {
			Expect(CipherCode("TLS_AES_128_GCM_SHA256")).To(Equal(uint16(tls.TLS_AES_128_GCM_SHA256)))
			Expect(CipherCode("unsupported")).To(BeZero())
		})

		It("keeps supported ciphers in order and reports unsupported names", func() {
			codes, unsupported := CipherCodes([]string{
				"ECDHE-RSA-AES128-GCM-SHA256", "unsupported", "TLS_AES_128_GCM_SHA256", "also-unsupported",
			})
			Expect(codes).To(Equal([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_AES_128_GCM_SHA256}))
			Expect(unsupported).To(Equal([]string{"unsupported", "also-unsupported"}))
		})
	})

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
		It("should return the selected built-in profile", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
			Expect(GetTLSProfileSpec(profile)).To(Equal(*configv1.TLSProfiles[configv1.TLSProfileModernType]))
		})
		It("should return the default profile for an unknown profile type", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileType("unknown")}
			Expect(GetTLSProfileSpec(profile)).To(Equal(*configv1.TLSProfiles[DefaultTLSProfileType]))
		})
		It("should return the default profile when a custom profile has no spec", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType}
			Expect(GetTLSProfileSpec(profile)).To(Equal(*configv1.TLSProfiles[DefaultTLSProfileType]))
		})
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

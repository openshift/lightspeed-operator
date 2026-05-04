package tls

import (
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
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

		It("should return the built-in profile when type is old", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType}
			Expect(GetTLSProfileSpec(profile)).To(Equal(*configv1.TLSProfiles[configv1.TLSProfileOldType]))
		})

		It("should return default when custom type but Custom is nil", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType}
			Expect(GetTLSProfileSpec(profile)).To(Equal(*configv1.TLSProfiles[DefaultTLSProfileType]))
		})

		It("should return default when profile type is unknown", func() {
			profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileType("UnknownProfile")}
			Expect(GetTLSProfileSpec(profile)).To(Equal(*configv1.TLSProfiles[DefaultTLSProfileType]))
		})
	})

	Context("VersionCode", func() {
		It("maps known TLS protocol versions", func() {
			Expect(VersionCode(configv1.VersionTLS12)).To(Equal(uint16(tls.VersionTLS12)))
			Expect(VersionCode(configv1.VersionTLS13)).To(Equal(uint16(tls.VersionTLS13)))
		})

		It("falls back to default profile min version for unrecognized values", func() {
			def := *configv1.TLSProfiles[DefaultTLSProfileType]
			Expect(VersionCode(configv1.TLSProtocolVersion(""))).To(Equal(VersionCode(def.MinTLSVersion)))
		})
	})

	Context("CipherCode and CipherCodes", func() {
		It("maps a known OpenShift cipher name to a Go cipher suite id", func() {
			Expect(CipherCode("TLS_AES_128_GCM_SHA256")).To(Equal(uint16(tls.TLS_AES_128_GCM_SHA256)))
		})

		It("returns zero for unknown cipher names", func() {
			Expect(CipherCode("not-a-real-cipher-suite-name")).To(BeZero())
		})

		It("CipherCodes collects known ciphers and lists unsupported names", func() {
			codes, unsupported := CipherCodes([]string{"TLS_AES_128_GCM_SHA256", "not-a-real-cipher-suite-name"})
			Expect(codes).To(ConsistOf(uint16(tls.TLS_AES_128_GCM_SHA256)))
			Expect(unsupported).To(ConsistOf("not-a-real-cipher-suite-name"))
		})
	})

	Context("FetchAPIServerTlsProfile", func() {
		It("returns an error when APIServer does not exist", func() {
			sch := runtime.NewScheme()
			utilruntime.Must(configv1.AddToScheme(sch))
			k8s := fake.NewClientBuilder().WithScheme(sch).Build()
			_, err := FetchAPIServerTlsProfile(k8s)
			Expect(err).To(HaveOccurred())
		})

		It("returns the TLS profile from the cluster APIServer object", func() {
			sch := runtime.NewScheme()
			utilruntime.Must(configv1.AddToScheme(sch))
			want := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: APIServerName},
				Spec:       configv1.APIServerSpec{TLSSecurityProfile: want},
			}
			k8s := fake.NewClientBuilder().WithScheme(sch).WithObjects(apiServer).Build()
			got, err := FetchAPIServerTlsProfile(k8s)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).NotTo(BeNil())
			Expect(got.Type).To(Equal(configv1.TLSProfileModernType))
		})
	})
})

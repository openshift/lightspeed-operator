package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Resource defaults and test reconciler", func() {
	It("GetResourcesOrDefault returns custom resources when set", func() {
		custom := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
		}
		def := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
		}
		Expect(GetResourcesOrDefault(custom, def)).To(BeIdenticalTo(custom))
	})

	It("GetResourcesOrDefault returns defaults when custom is nil", func() {
		def := &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
		}
		Expect(GetResourcesOrDefault(nil, def)).To(BeIdenticalTo(def))
	})

	It("RestrictedContainerSecurityContext matches restricted pod security expectations", func() {
		sc := RestrictedContainerSecurityContext()
		Expect(sc).NotTo(BeNil())
		Expect(sc.AllowPrivilegeEscalation).NotTo(BeNil())
		Expect(*sc.AllowPrivilegeEscalation).To(BeFalse())
		Expect(sc.ReadOnlyRootFilesystem).NotTo(BeNil())
		Expect(*sc.ReadOnlyRootFilesystem).To(BeTrue())
		Expect(sc.RunAsNonRoot).NotTo(BeNil())
		Expect(*sc.RunAsNonRoot).To(BeTrue())
		Expect(sc.SeccompProfile).NotTo(BeNil())
		Expect(sc.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
		Expect(sc.Capabilities).NotTo(BeNil())
		Expect(sc.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
	})

	It("TestReconciler getters and watcher config reflect NewTestReconciler defaults", func() {
		sch := runtime.NewScheme()
		utilruntime.Must(corev1.AddToScheme(sch))
		log := logf.Log.WithName("utils-test")
		k8s := fake.NewClientBuilder().WithScheme(sch).Build()
		r := NewTestReconciler(k8s, log, sch, "test-ns")

		Expect(r.GetScheme()).To(Equal(sch))
		Expect(r.GetLogger()).To(Equal(log))
		Expect(r.GetNamespace()).To(Equal("test-ns"))
		Expect(r.GetPostgresImage()).To(Equal(PostgresServerImageDefault))
		Expect(r.GetConsoleUIImage()).To(Equal(ConsoleUIImageDefault))
		Expect(r.GetOpenShiftMajor()).To(Equal("123"))
		Expect(r.GetOpenshiftMinor()).To(Equal("456"))
		Expect(r.GetAppServerImage()).To(Equal(OLSAppServerImageDefault))
		Expect(r.GetOpenShiftMCPServerImage()).To(Equal(OLSAppServerImageDefault))
		Expect(r.GetDataverseExporterImage()).To(Equal(DataverseExporterImageDefault))
		Expect(r.IsPrometheusAvailable()).To(BeTrue())
		Expect(r.GetWatcherConfig()).To(BeNil())

		r.SetWatcherConfig(map[string]string{"k": "v"})
		cfg, ok := r.GetWatcherConfig().(map[string]string)
		Expect(ok).To(BeTrue())
		Expect(cfg["k"]).To(Equal("v"))
	})
})

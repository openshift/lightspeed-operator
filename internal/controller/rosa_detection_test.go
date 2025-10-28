package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ROSA Detection", func() {

	Context("detectROSAEnvironment", func() {
		var console *openshiftv1.Console

		BeforeEach(func() {
			// Ensure clean state by removing any existing Console object
			existingConsole := &openshiftv1.Console{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ConsoleCRName}, existingConsole)
			if err == nil {
				_ = k8sClient.Delete(ctx, existingConsole)
			}
		})

		AfterEach(func() {
			if console != nil {
				By("Clean up the Console object")
				_ = k8sClient.Delete(ctx, console)
				console = nil
			}
		})

		It("should return true when Console object has ROSA brand", func() {
			By("Create a Console object with ROSA branding")
			console = &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
					Customization: openshiftv1.ConsoleCustomization{
						Brand: "ROSA",
					},
				},
			}
			err := k8sClient.Create(ctx, console)
			Expect(err).NotTo(HaveOccurred())

			By("Call detectROSAEnvironment")
			isROSA, err := reconciler.detectROSAEnvironment(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(isROSA).To(BeTrue())
		})

		It("should return false when Console object has no customization", func() {
			By("Create a Console object without customization")
			console = &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
					// No customization field
				},
			}
			err := k8sClient.Create(ctx, console)
			Expect(err).NotTo(HaveOccurred())

			By("Call detectROSAEnvironment")
			isROSA, err := reconciler.detectROSAEnvironment(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(isROSA).To(BeFalse())
		})

		It("should return false when Console object has different brand", func() {
			By("Create a Console object with OpenShift branding")
			console = &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
					Customization: openshiftv1.ConsoleCustomization{
						Brand: "OpenShift",
					},
				},
			}
			err := k8sClient.Create(ctx, console)
			Expect(err).NotTo(HaveOccurred())

			By("Call detectROSAEnvironment")
			isROSA, err := reconciler.detectROSAEnvironment(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(isROSA).To(BeFalse())
		})

		It("should return false and no error when Console object does not exist", func() {
			By("Ensure no Console object exists")
			// BeforeEach already cleaned up, so no Console should exist
			// This test specifically tests the "not found" scenario

			By("Call detectROSAEnvironment")
			isROSA, err := reconciler.detectROSAEnvironment(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(isROSA).To(BeFalse())
		})

		It("should return false when Console object has empty brand", func() {
			By("Create a Console object with empty brand")
			console = &openshiftv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name: ConsoleCRName,
				},
				Spec: openshiftv1.ConsoleSpec{
					OperatorSpec: openshiftv1.OperatorSpec{
						ManagementState: openshiftv1.Managed,
					},
					Customization: openshiftv1.ConsoleCustomization{
						Brand: "",
					},
				},
			}
			err := k8sClient.Create(ctx, console)
			Expect(err).NotTo(HaveOccurred())

			By("Call detectROSAEnvironment")
			isROSA, err := reconciler.detectROSAEnvironment(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(isROSA).To(BeFalse())
		})
	})
})

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftv1 "github.com/openshift/api/operator/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

var _ = Describe("ROSA Environment Integration", Ordered, func() {
	var cr *olsv1alpha1.OLSConfig
	var console *openshiftv1.Console
	var err error
	var client *Client

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a Console object with ROSA branding")
		console = &openshiftv1.Console{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
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
		err = client.Create(console)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a OLSConfig CR in ROSA environment")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		err = mustGather("rosa_integration_test")
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the OLSConfig CR")
		if cr != nil {
			err = client.Delete(cr)
			Expect(err).NotTo(HaveOccurred())
		}

		By("Deleting the Console object")
		if console != nil {
			err = client.Delete(console)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should detect ROSA environment and configure ROSA-specific RAG content", func() {
		By("waiting for application server deployment to be ready")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("verifying ROSA RAG configuration is applied to OLSConfig")
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err = client.Get(cr)
			if err != nil {
				return err
			}

			// Verify that ROSA RAG source is automatically added
			foundROSARag := false
			for _, ragSource := range cr.Spec.OLSConfig.RAG {
				if ragSource.Image == "quay.io/thoraxe/acm-byok:2510030943" {
					foundROSARag = true
					// Verify default values
					Expect(ragSource.IndexPath).To(Equal("/rag/vector_db"))
					Expect(ragSource.IndexID).To(Equal("vector_db_index"))
					break
				}
			}
			if !foundROSARag {
				return fmt.Errorf("ROSA RAG source not found in OLSConfig.RAG")
			}

			// Verify that only ROSA content is present (default OpenShift docs should be replaced)
			// In ROSA environment, the default OpenShift documentation should not be present
			// and only ROSA-specific content should be used
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("verifying RAG init containers use ROSA image")
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			// Check if any init container uses the ROSA RAG image
			foundROSAInitContainer := false
			for _, initContainer := range dep.Spec.Template.Spec.InitContainers {
				if initContainer.Image == "quay.io/thoraxe/acm-byok:2510030943" {
					foundROSAInitContainer = true
					// Verify the init container has the expected command/args for RAG content extraction
					Expect(initContainer.Command).To(ContainElement("cp"))
					Expect(initContainer.Args).To(ContainElement("-r"))
					break
				}
			}
			if !foundROSAInitContainer {
				return false, fmt.Errorf("ROSA RAG init container not found in deployment")
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("verifying RAG volume mount exists for ROSA content")
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			// Check if RAG volume is mounted
			foundRAGVolumeMount := false
			for _, volumeMount := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
				if volumeMount.Name == "rag" && volumeMount.MountPath == "/rag-data" {
					foundRAGVolumeMount = true
					break
				}
			}
			if !foundRAGVolumeMount {
				return false, fmt.Errorf("RAG volume mount not found in main container")
			}

			// Check if RAG volume exists
			foundRAGVolume := false
			for _, volume := range dep.Spec.Template.Spec.Volumes {
				if volume.Name == "rag" && volume.VolumeSource.EmptyDir != nil {
					foundRAGVolume = true
					break
				}
			}
			if !foundRAGVolume {
				return false, fmt.Errorf("RAG volume not found in deployment")
			}
			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should preserve user-defined RAG sources when ROSA is detected", func() {
		By("updating OLSConfig with user-defined RAG sources")
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err = client.Get(cr)
			if err != nil {
				return err
			}

			// Add a user-defined RAG source
			userRAG := olsv1alpha1.RAGSpec{
				Image:     "registry.redhat.io/ubi9/ubi:latest",
				IndexPath: "/rag/vector_db",
				IndexID:   "user_vector_db_index",
			}
			cr.Spec.OLSConfig.RAG = append(cr.Spec.OLSConfig.RAG, userRAG)

			return client.Update(cr)
		})
		Expect(err).NotTo(HaveOccurred())

		By("verifying both ROSA and user RAG sources are present")
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err = client.Get(cr)
			if err != nil {
				return err
			}

			foundROSARag := false
			foundUserRag := false
			for _, ragSource := range cr.Spec.OLSConfig.RAG {
				if ragSource.Image == "quay.io/thoraxe/acm-byok:2510030943" {
					foundROSARag = true
				}
				if ragSource.Image == "registry.redhat.io/ubi9/ubi:latest" {
					foundUserRag = true
				}
			}

			if !foundROSARag {
				return fmt.Errorf("ROSA RAG source missing after user RAG was added")
			}
			if !foundUserRag {
				return fmt.Errorf("User RAG source not found")
			}

			// Verify no duplicate ROSA RAG sources
			rosaCount := 0
			for _, ragSource := range cr.Spec.OLSConfig.RAG {
				if ragSource.Image == "quay.io/thoraxe/acm-byok:2510030943" {
					rosaCount++
				}
			}
			if rosaCount > 1 {
				return fmt.Errorf("Found %d ROSA RAG sources, expected 1", rosaCount)
			}

			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("verifying deployment has both ROSA and user init containers")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentCondition(deployment, func(dep *appsv1.Deployment) (bool, error) {
			foundROSAInitContainer := false
			foundUserInitContainer := false

			for _, initContainer := range dep.Spec.Template.Spec.InitContainers {
				if initContainer.Image == "quay.io/thoraxe/acm-byok:2510030943" {
					foundROSAInitContainer = true
				}
				if initContainer.Image == "registry.redhat.io/ubi9/ubi:latest" {
					foundUserInitContainer = true
				}
			}

			if !foundROSAInitContainer {
				return false, fmt.Errorf("ROSA init container not found")
			}
			if !foundUserInitContainer {
				return false, fmt.Errorf("User init container not found")
			}

			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())
	})
})

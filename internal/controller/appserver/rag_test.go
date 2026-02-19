package appserver

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imagev1 "github.com/openshift/api/image/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("App server assets", func() {
	var cr *olsv1alpha1.OLSConfig

	Context("complete custom resource", func() {
		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image:     "rag-image-1",
					IndexPath: "/path/to/index-1",
					IndexID:   "index-id-1",
				},
				{
					Image:     "rag-image-2",
					IndexPath: "/path/to/index-2",
					IndexID:   "index-id-2",
				},
			}
		})

		AfterEach(func() {
		})

		It("should generate initContainer for each RAG", func() {
			initContainers := GenerateRAGInitContainers(cr)
			Expect(initContainers).To(HaveLen(2))
			Expect(initContainers[0]).To(MatchFields(IgnoreExtras, Fields{
				"Name":            Equal("rag-0"),
				"Image":           Equal("rag-image-1"),
				"ImagePullPolicy": Equal(corev1.PullAlways),
				"Command":         Equal([]string{"sh", "-c", "mkdir -p /rag-data/rag-0 && cp -a /path/to/index-1/. /rag-data/rag-0"}),
				"VolumeMounts": ConsistOf(corev1.VolumeMount{
					Name:      utils.RAGVolumeName,
					MountPath: "/rag-data",
				}),
			}))
			Expect(initContainers[1]).To(MatchFields(IgnoreExtras, Fields{
				"Name":            Equal("rag-1"),
				"Image":           Equal("rag-image-2"),
				"ImagePullPolicy": Equal(corev1.PullAlways),
				"Command":         Equal([]string{"sh", "-c", "mkdir -p /rag-data/rag-1 && cp -a /path/to/index-2/. /rag-data/rag-1"}),
				"VolumeMounts": ConsistOf(corev1.VolumeMount{
					Name:      utils.RAGVolumeName,
					MountPath: "/rag-data",
				}),
			}))
		})

		It("should create an ImageStream for each RAG image", func() {
			err := reconcileImageStreams(testReconcilerInstance, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			for _, rag := range cr.Spec.OLSConfig.RAG {
				var is imagev1.ImageStream
				err = testReconcilerInstance.Get(ctx, types.NamespacedName{Name: utils.ImageStreamNameFor(rag.Image), Namespace: testReconcilerInstance.GetNamespace()}, &is)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should generate ImageStream trigger annotation for each RAG init container", func() {
			triggersJSON, err := generateImageStreamTriggers(cr)
			Expect(err).NotTo(HaveOccurred())
			var triggers []ImageTrigger
			Expect(json.Unmarshal([]byte(triggersJSON), &triggers)).To(Succeed())
			Expect(triggers).To(HaveLen(2))
			for idx, rag := range cr.Spec.OLSConfig.RAG {
				isName := utils.ImageStreamNameFor(rag.Image)
				initContainerName := fmt.Sprintf("rag-%d", idx)
				Expect(triggers[idx].From.Kind).To(Equal("ImageStreamTag"))
				Expect(triggers[idx].From.Name).To(Equal(isName + ":latest"))
				Expect(triggers[idx].FieldPath).To(Equal(fmt.Sprintf("spec.template.spec.initContainers[?(@.name==\"%s\")].image", initContainerName)))
			}
		})
	})

	Context("two RAGSpec entries with the same image", func() {
		const sharedImage = "quay.io/myorg/rag-index:latest"

		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{Image: sharedImage, IndexPath: "/path/to/index-1", IndexID: "index-id-1"},
				{Image: sharedImage, IndexPath: "/path/to/index-2", IndexID: "index-id-2"},
			}
		})

		It("creates only one ImageStream for the shared image", func() {
			err := reconcileImageStreams(testReconcilerInstance, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			sel := labels.SelectorFromSet(utils.GenerateAppServerSelectorLabels())
			var list imagev1.ImageStreamList
			err = testReconcilerInstance.List(ctx, &list, client.InNamespace(testReconcilerInstance.GetNamespace()), &client.ListOptions{LabelSelector: sel})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].Name).To(Equal(utils.ImageStreamNameFor(sharedImage)))
		})
	})

	Context("no RAG entries", func() {
		BeforeEach(func() {
			cr = utils.GetDefaultOLSConfigCR()
			cr.Spec.OLSConfig.RAG = nil
		})

		It("creates no ImageStreams", func() {
			err := reconcileImageStreams(testReconcilerInstance, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			sel := labels.SelectorFromSet(utils.GenerateAppServerSelectorLabels())
			var list imagev1.ImageStreamList
			err = testReconcilerInstance.List(ctx, &list, client.InNamespace(testReconcilerInstance.GetNamespace()), &client.ListOptions{LabelSelector: sel})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(BeEmpty())
		})
	})

	Context("RAGSpec removed after ImageStream created", func() {
		const ragImage = "quay.io/myorg/rag:v1"

		It("creates ImageStream when RAG is set, then deletes it when RAG is removed", func() {
			cr = utils.GetDefaultOLSConfigCR()
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{Image: ragImage, IndexPath: "/data/index", IndexID: "idx-1"},
			}
			err := reconcileImageStreams(testReconcilerInstance, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			isName := utils.ImageStreamNameFor(ragImage)
			var is imagev1.ImageStream
			err = testReconcilerInstance.Get(ctx, types.NamespacedName{Name: isName, Namespace: testReconcilerInstance.GetNamespace()}, &is)
			Expect(err).NotTo(HaveOccurred())

			cr.Spec.OLSConfig.RAG = nil
			err = reconcileImageStreams(testReconcilerInstance, context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
			sel := labels.SelectorFromSet(utils.GenerateAppServerSelectorLabels())
			var list imagev1.ImageStreamList
			err = testReconcilerInstance.List(ctx, &list, client.InNamespace(testReconcilerInstance.GetNamespace()), &client.ListOptions{LabelSelector: sel})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(BeEmpty())
		})
	})
})

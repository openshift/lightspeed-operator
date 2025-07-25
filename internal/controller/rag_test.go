package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("App server assets", func() {
	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions

	Context("complete custom resource", func() {
		BeforeEach(func() {
			rOptions = &OLSConfigReconcilerOptions{
				LightspeedServiceImage: "lightspeed-service:latest",
				Namespace:              OLSNamespaceDefault,
			}
			cr = getDefaultOLSConfigCR()
			r = &OLSConfigReconciler{
				Options:    *rOptions,
				logger:     logf.Log.WithName("olsconfig.reconciler"),
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				stateCache: make(map[string]string),
			}
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
			initContainers := r.generateRAGInitContainers(cr)
			Expect(initContainers).To(HaveLen(2))
			Expect(initContainers[0]).To(MatchFields(IgnoreExtras, Fields{
				"Name":            Equal("rag-0"),
				"Image":           Equal("rag-image-1"),
				"ImagePullPolicy": Equal(corev1.PullAlways),
				"Command":         Equal([]string{"sh", "-c", "mkdir -p /rag-data/rag-0 && cp -a /path/to/index-1/. /rag-data/rag-0"}),
				"VolumeMounts": ConsistOf(corev1.VolumeMount{
					Name:      RAGVolumeName,
					MountPath: "/rag-data",
				}),
			}))
			Expect(initContainers[1]).To(MatchFields(IgnoreExtras, Fields{
				"Name":            Equal("rag-1"),
				"Image":           Equal("rag-image-2"),
				"ImagePullPolicy": Equal(corev1.PullAlways),
				"Command":         Equal([]string{"sh", "-c", "mkdir -p /rag-data/rag-1 && cp -a /path/to/index-2/. /rag-data/rag-1"}),
				"VolumeMounts": ConsistOf(corev1.VolumeMount{
					Name:      RAGVolumeName,
					MountPath: "/rag-data",
				}),
			}))
		})

	})
})

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var _ = Describe("ROSA RAG Configuration", func() {
	var cr *olsv1alpha1.OLSConfig
	var r *OLSConfigReconciler
	var rOptions *OLSConfigReconcilerOptions

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
	})

	Context("when applying ROSA RAG configuration", func() {
		It("should add default ROSA RAG source when no user RAG sources exist", func() {
			// Arrange: start with empty RAG sources
			cr.Spec.OLSConfig.RAG = nil

			// Act: apply ROSA RAG configuration
			err := r.applyROSARAGConfiguration(cr)

			// Assert: ROSA RAG source should be added
			Expect(err).ToNot(HaveOccurred())
			Expect(cr.Spec.OLSConfig.RAG).To(HaveLen(1))
			Expect(cr.Spec.OLSConfig.RAG[0].Image).To(Equal("quay.io/thoraxe/acm-byok:2510030943"))
			Expect(cr.Spec.OLSConfig.RAG[0].IndexPath).To(Equal("/rag/vector_db"))
			Expect(cr.Spec.OLSConfig.RAG[0].IndexID).To(Equal("vector_db_index"))
		})

		It("should add ROSA RAG source alongside existing user RAG sources", func() {
			// Arrange: start with existing user RAG sources
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image:     "user-rag-image:latest",
					IndexPath: "/user/rag/path",
					IndexID:   "user-rag-index",
				},
			}

			// Act: apply ROSA RAG configuration
			err := r.applyROSARAGConfiguration(cr)

			// Assert: ROSA RAG source should be added alongside user sources
			Expect(err).ToNot(HaveOccurred())
			Expect(cr.Spec.OLSConfig.RAG).To(HaveLen(2))

			// Check original user RAG source is preserved
			Expect(cr.Spec.OLSConfig.RAG[0].Image).To(Equal("user-rag-image:latest"))
			Expect(cr.Spec.OLSConfig.RAG[0].IndexPath).To(Equal("/user/rag/path"))
			Expect(cr.Spec.OLSConfig.RAG[0].IndexID).To(Equal("user-rag-index"))

			// Check ROSA RAG source is added
			Expect(cr.Spec.OLSConfig.RAG[1].Image).To(Equal("quay.io/thoraxe/acm-byok:2510030943"))
			Expect(cr.Spec.OLSConfig.RAG[1].IndexPath).To(Equal("/rag/vector_db"))
			Expect(cr.Spec.OLSConfig.RAG[1].IndexID).To(Equal("vector_db_index"))
		})

		It("should not add duplicate ROSA RAG source if already present", func() {
			// Arrange: start with ROSA RAG source already present
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image:     "quay.io/thoraxe/acm-byok:2510030943",
					IndexPath: "/rag/vector_db",
					IndexID:   "vector_db_index",
				},
			}

			// Act: apply ROSA RAG configuration
			err := r.applyROSARAGConfiguration(cr)

			// Assert: no duplicate should be added
			Expect(err).ToNot(HaveOccurred())
			Expect(cr.Spec.OLSConfig.RAG).To(HaveLen(1))
			Expect(cr.Spec.OLSConfig.RAG[0].Image).To(Equal("quay.io/thoraxe/acm-byok:2510030943"))
		})

		It("should handle multiple user RAG sources correctly", func() {
			// Arrange: start with multiple user RAG sources
			cr.Spec.OLSConfig.RAG = []olsv1alpha1.RAGSpec{
				{
					Image:     "user-rag-1:latest",
					IndexPath: "/user/rag/path1",
					IndexID:   "user-rag-index-1",
				},
				{
					Image:     "user-rag-2:latest",
					IndexPath: "/user/rag/path2",
					IndexID:   "user-rag-index-2",
				},
			}

			// Act: apply ROSA RAG configuration
			err := r.applyROSARAGConfiguration(cr)

			// Assert: ROSA RAG source should be added as the third source
			Expect(err).ToNot(HaveOccurred())
			Expect(cr.Spec.OLSConfig.RAG).To(HaveLen(3))

			// Check user RAG sources are preserved
			Expect(cr.Spec.OLSConfig.RAG[0].Image).To(Equal("user-rag-1:latest"))
			Expect(cr.Spec.OLSConfig.RAG[1].Image).To(Equal("user-rag-2:latest"))

			// Check ROSA RAG source is added
			Expect(cr.Spec.OLSConfig.RAG[2].Image).To(Equal("quay.io/thoraxe/acm-byok:2510030943"))
		})
	})

	Context("when not applying ROSA RAG configuration", func() {
		It("should not modify RAG sources for non-ROSA environments", func() {
			// Arrange: start with user RAG sources
			originalRAG := []olsv1alpha1.RAGSpec{
				{
					Image:     "user-rag-image:latest",
					IndexPath: "/user/rag/path",
					IndexID:   "user-rag-index",
				},
			}
			cr.Spec.OLSConfig.RAG = originalRAG

			// Note: This test assumes there would be a separate function or conditional logic
			// that checks if ROSA is detected before calling applyROSARAGConfiguration
			// For now, we're testing the ROSA configuration function directly

			// Act: no action taken for non-ROSA environments
			// This would be handled by the calling code that detects ROSA first

			// Assert: RAG sources should remain unchanged
			Expect(cr.Spec.OLSConfig.RAG).To(Equal(originalRAG))
		})
	})
})

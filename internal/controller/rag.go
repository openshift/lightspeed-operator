package controller

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func (r *OLSConfigReconciler) generateRAGVolume() corev1.Volume {
	return corev1.Volume{
		Name: RAGVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func (r *OLSConfigReconciler) generateRAGInitContainers(cr *olsv1alpha1.OLSConfig) []corev1.Container {
	var initContainers []corev1.Container
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		ragName := fmt.Sprintf("rag-%d", idx)
		initContainers = append(initContainers, corev1.Container{
			Name:            ragName,
			Image:           rag.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"sh", "-c", fmt.Sprintf("mkdir -p %s && cp -a %s/. %s", path.Join(RAGVolumeMountPath, ragName), rag.IndexPath, path.Join(RAGVolumeMountPath, ragName))},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      RAGVolumeName,
					MountPath: RAGVolumeMountPath,
				},
			},
		})
	}
	return initContainers
}

func (r *OLSConfigReconciler) generateRAGVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      RAGVolumeName,
		MountPath: RAGVolumeMountPath,
	}
}

// applyROSARAGConfiguration adds the ROSA-specific RAG source to the OLSConfig
// if it doesn't already exist. This is called when ROSA environment is detected.
func (r *OLSConfigReconciler) applyROSARAGConfiguration(cr *olsv1alpha1.OLSConfig) error {
	rosaRAGSpec := olsv1alpha1.RAGSpec{
		Image:     "quay.io/thoraxe/acm-byok:2510030943", // Using placeholder image as specified
		IndexPath: "/rag/vector_db",
		IndexID:   "vector_db_index",
	}

	// Check if ROSA RAG source already exists
	for _, existingRAG := range cr.Spec.OLSConfig.RAG {
		if existingRAG.Image == rosaRAGSpec.Image {
			r.logger.Info("ROSA RAG source already present, skipping addition",
				"image", rosaRAGSpec.Image)
			return nil
		}
	}

	// Add ROSA RAG source to the list
	cr.Spec.OLSConfig.RAG = append(cr.Spec.OLSConfig.RAG, rosaRAGSpec)
	r.logger.Info("Added ROSA RAG source to configuration",
		"image", rosaRAGSpec.Image,
		"totalRAGSources", len(cr.Spec.OLSConfig.RAG))

	return nil
}

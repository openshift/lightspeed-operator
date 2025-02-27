package controller

import (
	"fmt"

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
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sh", "-c", fmt.Sprintf("mkdir -p /rag-data/%s && cp -a %s /rag-data/%s", ragName, rag.IndexPath, ragName)},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      RAGVolumeName,
					MountPath: "/rag-data",
				},
			},
		})
	}
	return initContainers
}

func (r *OLSConfigReconciler) generateRAGVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      RAGVolumeName,
		MountPath: "/rag-data",
	}
}

package utils

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func GenerateRAGVolume() corev1.Volume {
	return corev1.Volume{
		Name: RAGVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func GenerateRAGInitContainer(name, image, indexPath string, cr *olsv1alpha1.OLSConfig) corev1.Container {
	return corev1.Container{
		Name:            name,
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		Command:         []string{"sh", "-c", fmt.Sprintf("mkdir -p %s && cp -a %s/. %s", path.Join(RAGVolumeMountPath, name), indexPath, path.Join(RAGVolumeMountPath, name))},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      RAGVolumeName,
				MountPath: RAGVolumeMountPath,
			},
		},
	}
}

func GenerateRAGInitContainers(cr *olsv1alpha1.OLSConfig) []corev1.Container {
	var initContainers []corev1.Container
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		ragName := fmt.Sprintf("rag-%d", idx)
		initContainers = append(initContainers, GenerateRAGInitContainer(ragName, rag.Image, rag.IndexPath, cr))
	}
	return initContainers
}

func GenerateRAGVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      RAGVolumeName,
		MountPath: RAGVolumeMountPath,
	}
}

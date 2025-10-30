package appserver

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func generateRAGVolume() corev1.Volume {
	return corev1.Volume{
		Name: utils.RAGVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func GenerateRAGInitContainers(cr *olsv1alpha1.OLSConfig) []corev1.Container {
	var initContainers []corev1.Container
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		ragName := fmt.Sprintf("rag-%d", idx)
		initContainers = append(initContainers, corev1.Container{
			Name:            ragName,
			Image:           rag.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"sh", "-c", fmt.Sprintf("mkdir -p %s && cp -a %s/. %s", path.Join(utils.RAGVolumeMountPath, ragName), rag.IndexPath, path.Join(utils.RAGVolumeMountPath, ragName))},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      utils.RAGVolumeName,
					MountPath: utils.RAGVolumeMountPath,
				},
			},
		})
	}
	return initContainers
}

func generateRAGVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      utils.RAGVolumeName,
		MountPath: utils.RAGVolumeMountPath,
	}
}

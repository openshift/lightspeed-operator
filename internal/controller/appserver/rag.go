package appserver

import (
	"encoding/json"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ImageTrigger represents a Build trigger entry for ImageStreamTag (used in deployment annotations).
type ImageTrigger struct {
	From      ImageTriggerFrom `json:"from"`
	FieldPath string           `json:"fieldPath"`
}

// ImageTriggerFrom identifies the image stream tag to trigger on.
type ImageTriggerFrom struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

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

func generateImageStreamTriggers(cr *olsv1alpha1.OLSConfig) (string, error) {
	var triggers []ImageTrigger
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		isName := utils.ImageStreamNameFor(rag.Image)
		initContainerName := fmt.Sprintf("rag-%d", idx)
		triggers = append(triggers, ImageTrigger{
			From: ImageTriggerFrom{
				Kind: "ImageStreamTag",
				Name: fmt.Sprintf("%s:latest", isName),
			},
			FieldPath: fmt.Sprintf("spec.template.spec.initContainers[?(@.name==\"%s\")].image", initContainerName),
		})
	}
	data, err := json.Marshal(triggers)
	if err != nil {
		return "", fmt.Errorf("marshal image triggers: %w", err)
	}
	return string(data), nil
}

package appserver

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ImageTrigger represents a Build trigger entry for ImageStreamTag (used in deployment annotations).
type ImageTrigger struct {
	From      ImageTriggerFrom `json:"from"`
	FieldPath string           `json:"fieldPath"`
}

// ImageTriggerFrom identifies the image stream tag for a Build trigger.
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

// generateRAGUtilVolume returns the volume that carries the copy binary
// staged by the rag-util init container. It is separate from the RAG data
// volume so the staged binary never mixes with the index files.
func generateRAGUtilVolume() corev1.Volume {
	return corev1.Volume{
		Name: utils.RAGUtilVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// generateRAGUtilInitContainer returns the init container that stages the
// copy binary and target directories for the RAG data init containers.
//
// Customer RAG images are data-only (a single layer with /rag/vector_db, no
// base layer, no shell), so the per-index init containers must not execute
// anything from the image's own filesystem — the OLM-bundle pattern: OLM
// stages its `cpb` binary from a separate multi-arch image and runs it as
// the bundle image's command. Here the service image is the source of
// `sh`/`cp`: the main app-server container runs it anyway, so it always
// matches the node architecture and adds no image dependency.
func generateRAGUtilInitContainer(cr *olsv1alpha1.OLSConfig, appServerImage string) corev1.Container {
	var dirs strings.Builder
	for idx := range cr.Spec.OLSConfig.RAG {
		fmt.Fprintf(&dirs, " %s", path.Join(utils.RAGVolumeMountPath, fmt.Sprintf("rag-%d", idx)))
	}
	script := fmt.Sprintf("mkdir -p%s && cp /usr/bin/cp %s/cp", dirs.String(), utils.RAGUtilVolumeMountPath)
	return corev1.Container{
		Name:            utils.RAGUtilInitContainerName,
		Image:           appServerImage,
		ImagePullPolicy: corev1.PullAlways,
		Command:         []string{"/bin/sh", "-c", script},
		SecurityContext: utils.RestrictedContainerSecurityContext(),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      utils.RAGVolumeName,
				MountPath: utils.RAGVolumeMountPath,
			},
			{
				Name:      utils.RAGUtilVolumeName,
				MountPath: utils.RAGUtilVolumeMountPath,
			},
		},
	}
}

// GenerateRAGInitContainers returns the init containers that copy each RAG
// index from the customer image into the shared volume. The first container
// stages the copy binary from the service image; each per-index container
// runs that binary as its command, overriding the image entrypoint so
// nothing from the (data-only, possibly foreign-architecture) customer
// image is ever executed.
func GenerateRAGInitContainers(cr *olsv1alpha1.OLSConfig, appServerImage string) []corev1.Container {
	if len(cr.Spec.OLSConfig.RAG) == 0 {
		return nil
	}
	initContainers := []corev1.Container{generateRAGUtilInitContainer(cr, appServerImage)}
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		ragName := fmt.Sprintf("rag-%d", idx)
		initContainers = append(initContainers, corev1.Container{
			Name:            ragName,
			Image:           rag.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{path.Join(utils.RAGUtilVolumeMountPath, "cp"), "-a", rag.IndexPath + "/.", path.Join(utils.RAGVolumeMountPath, ragName)},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      utils.RAGVolumeName,
					MountPath: utils.RAGVolumeMountPath,
				},
				{
					Name:      utils.RAGUtilVolumeName,
					MountPath: utils.RAGUtilVolumeMountPath,
					ReadOnly:  true,
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

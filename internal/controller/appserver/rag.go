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
// copy tool for the RAG data init containers.
//
// Customer RAG images are data-only (a single layer with /rag/vector_db, no
// base layer, no shell, no glibc), so the per-index init containers must not
// execute anything from the image's own filesystem — the OLM-bundle pattern.
// OLM stages a statically linked Go binary (`cpb`); RHEL's coreutils is not
// static (and on coreutils-single, /usr/bin/cp is not even a binary but a
// script invoking the multicall /usr/bin/coreutils ELF), so staging the
// binary alone is not enough: a dynamically linked binary's PT_INTERP
// loader path is resolved against the *customer image's* rootfs, where
// /lib64 does not exist.
//
// The util container therefore stages three things into the rag-util
// emptyDir: the real coreutils binary, the dynamic loader (renamed to the
// arch-neutral /rag-util/ld.so — the loader works under any name), and the
// binary's shared libraries. The per-index containers then invoke cp
// through the staged loader with --library-path, which sidesteps both the
// shebang and the missing /lib64.
//
// Boundary note: the image's entrypoint and its own binaries are never
// executed, but the staged loader does run in the customer image's root
// filesystem and honors its /etc/ld.so.preload — an image supplying that
// file plus a compatible object could run its constructors. This does not
// open a new trust boundary (the cluster administrator selects the image
// in OLSConfig), but the distinction matters for security review.
//
// The service image is the source of all staged files: the main app-server
// container runs it anyway, so it always matches the node architecture and
// adds no image dependency.
func generateRAGUtilInitContainer(cr *olsv1alpha1.OLSConfig, appServerImage string) corev1.Container {
	var dirs strings.Builder
	for idx := range cr.Spec.OLSConfig.RAG {
		fmt.Fprintf(&dirs, " %s", path.Join(utils.RAGVolumeMountPath, fmt.Sprintf("rag-%d", idx)))
	}
	script := fmt.Sprintf(`set -eu
mkdir -p%[1]s
mkdir -p %[2]s/lib
if [ -x /usr/bin/coreutils ]; then CPBIN=/usr/bin/coreutils; else CPBIN=$(readlink -f /usr/bin/cp); fi
cp -L "$CPBIN" %[2]s/cp
# Loader names differ per architecture: ld-linux-x86-64.so.2 (x86_64),
# ld-linux-aarch64.so.1 (aarch64), ld64.so.2 (ppc64le), ld64.so.1 (s390x).
# /lib64/ld*.so* covers all four; a failed glob would abort staging here.
set -- /lib64/ld*.so*
[ -e "$1" ] || { echo "no dynamic loader found in /lib64" >&2; exit 1; }
cp -L "$1" %[2]s/ld.so
ldd "$CPBIN" | awk '/=> \//{print $3}' | xargs -r -I{} cp -L "{}" %[2]s/lib/`, dirs.String(), utils.RAGUtilVolumeMountPath)
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
// stages the copy tool (binary + loader + libraries) from the service image;
// each per-index container runs that tool via the staged loader, overriding
// the image entrypoint. The image's entrypoint and its own binaries are not
// run, but the staged loader executes in the customer image's root
// filesystem and honors its /etc/ld.so.preload (see the boundary note on
// generateRAGUtilInitContainer).
func GenerateRAGInitContainers(cr *olsv1alpha1.OLSConfig, appServerImage string) []corev1.Container {
	if len(cr.Spec.OLSConfig.RAG) == 0 {
		return nil
	}
	initContainers := []corev1.Container{generateRAGUtilInitContainer(cr, appServerImage)}
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		ragName := fmt.Sprintf("rag-%d", idx)
		// Invoke the staged binary through the staged dynamic loader: the
		// customer image has no /lib64, so the ELF's PT_INTERP path would not
		// resolve if cp were executed directly.
		command := []string{
			path.Join(utils.RAGUtilVolumeMountPath, "ld.so"),
			"--library-path", path.Join(utils.RAGUtilVolumeMountPath, "lib"),
			path.Join(utils.RAGUtilVolumeMountPath, "cp"),
			"-a", rag.IndexPath + "/.", path.Join(utils.RAGVolumeMountPath, ragName),
		}
		initContainers = append(initContainers, corev1.Container{
			Name:            ragName,
			Image:           rag.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         command,
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

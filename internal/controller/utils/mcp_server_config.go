package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

// GetOpenShiftMCPServerConfigVolumeAndMount returns the Volume and VolumeMount for the
// openshift-mcp-server TOML configuration. The config file is mounted as a subPath
// so the MCP container can reference it via --config.
func GetOpenShiftMCPServerConfigVolumeAndMount() (corev1.Volume, corev1.VolumeMount) {
	volumeDefaultMode := VolumeDefaultMode
	volume := corev1.Volume{
		Name: OpenShiftMCPServerConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OpenShiftMCPServerConfigCmName,
				},
				DefaultMode: &volumeDefaultMode,
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      OpenShiftMCPServerConfigVolumeName,
		MountPath: path.Join(OpenShiftMCPServerConfigMountPath, OpenShiftMCPServerConfigFilename),
		SubPath:   OpenShiftMCPServerConfigFilename,
		ReadOnly:  true,
	}

	return volume, volumeMount
}

// GetOpenShiftMCPServerConfigPath returns the full path to the MCP server config file inside the container.
func GetOpenShiftMCPServerConfigPath() string {
	return path.Join(OpenShiftMCPServerConfigMountPath, OpenShiftMCPServerConfigFilename)
}

// GetOpenShiftMCPServerCACertHash returns a SHA256 hash of the MCP CA ConfigMap's
// service-ca.crt content when introspection is enabled. Empty string when disabled.
// Returns ErrOpenShiftMCPServerCANotReady when the ConfigMap is missing or the
// cert has not been injected yet so callers requeue instead of stamping an empty
// hash (which would skip a later roll when the CA appears).
func GetOpenShiftMCPServerCACertHash(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (string, error) {
	if !BoolDeref(cr.Spec.OLSConfig.IntrospectionEnabled, true) {
		return "", nil
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: OpenShiftMCPServerCAConfigMapName, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%s: %w", ErrOpenShiftMCPServerCANotReady, err)
		}
		return "", err
	}

	certData, ok := cm.Data[OpenShiftMCPServerCACertKey]
	if !ok || certData == "" {
		return "", fmt.Errorf("%s: waiting for service-ca inject into %s", ErrOpenShiftMCPServerCANotReady, OpenShiftMCPServerCAConfigMapName)
	}

	hash := sha256.Sum256([]byte(certData))
	return hex.EncodeToString(hash[:]), nil
}

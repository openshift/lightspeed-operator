package utils

import (
	"context"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

// OpenShiftMCPServerConfigTOML is the TOML configuration for the shipped openshift-mcp-server sidecar.
// It denies access to Secret resources at the server level so secret data never reaches the LLM.
// Other sensitive resource types (RBAC) are also denied as defense in depth.
// Toolsets are listed explicitly so upstream default changes do not affect OLS; the metrics toolset
// uses in-cluster Thanos Querier and Alertmanager endpoints.
const OpenShiftMCPServerConfigTOML = `# Denied resources prevent the MCP server from accessing these Kubernetes resource types.
# This ensures secret data never reaches the LLM through the shipped MCP server.
# User-brought MCP servers (spec.mcpServers) are the user's responsibility to secure.
# Toolsets are pinned explicitly so upstream default changes do not affect OLS.

toolsets = ["core", "config", "helm", "metrics"]

[[denied_resources]]
group = ""
version = "v1"
kind = "Secret"

[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"

[toolset_configs.metrics]
prometheus_url = "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091"
alertmanager_url = "https://alertmanager-main.openshift-monitoring.svc.cluster.local:9094"
guardrails = "none"
`

// GenerateOpenShiftMCPServerConfigMap generates the ConfigMap containing the TOML configuration
// for the openshift-mcp-server sidecar. This ConfigMap is mounted into the sidecar container
// and referenced via the --config flag.
func GenerateOpenShiftMCPServerConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig, labels map[string]string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpenShiftMCPServerConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    labels,
		},
		Data: map[string]string{
			OpenShiftMCPServerConfigFilename: OpenShiftMCPServerConfigTOML,
		},
	}

	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return cm, nil
}

// ReconcileOpenShiftMCPServerConfigMap reconciles the ConfigMap for the openshift-mcp-server.
// When introspection is enabled, the ConfigMap is created/updated.
// When introspection is disabled, the ConfigMap is deleted if it exists.
func ReconcileOpenShiftMCPServerConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig, labels map[string]string) error {
	foundCm := &corev1.ConfigMap{}
	getErr := r.Get(ctx, client.ObjectKey{Name: OpenShiftMCPServerConfigCmName, Namespace: r.GetNamespace()}, foundCm)

	if !cr.Spec.OLSConfig.IntrospectionEnabled {
		if getErr == nil {
			r.GetLogger().Info("deleting MCP server config configmap", "configmap", OpenShiftMCPServerConfigCmName)
			if err := r.Delete(ctx, foundCm); err != nil {
				return fmt.Errorf("%s: %w", ErrDeleteMCPServerConfigMap, err)
			}
		}
		return nil
	}

	cm, err := GenerateOpenShiftMCPServerConfigMap(r, cr, labels)
	if err != nil {
		return fmt.Errorf("failed to generate MCP server config configmap: %w", err)
	}

	if getErr != nil && errors.IsNotFound(getErr) {
		r.GetLogger().Info("creating MCP server config configmap", "configmap", cm.Name)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateMCPServerConfigMap, err)
		}
		return nil
	} else if getErr != nil {
		return fmt.Errorf("%s: %w", ErrGetMCPServerConfigMap, getErr)
	}

	if ConfigMapEqual(foundCm, cm) {
		r.GetLogger().Info("MCP server config configmap reconciliation skipped", "configmap", foundCm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	if err := r.Update(ctx, foundCm); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateMCPServerConfigMap, err)
	}

	r.GetLogger().Info("MCP server config configmap reconciled", "configmap", cm.Name)
	return nil
}

// GetOpenShiftMCPServerConfigVolumeAndMount returns the Volume and VolumeMount for the
// openshift-mcp-server TOML configuration. The config file is mounted as a subPath
// so the sidecar can reference it via --config.
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

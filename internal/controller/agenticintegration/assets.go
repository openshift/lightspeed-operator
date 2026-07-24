// Package agenticintegration publishes the classic→agentic handoff ConfigMap.
// Client CA Secrets are owned by appserver; this package only references their names.
package agenticintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// SandboxModeFromCR returns the configured sandbox mode, defaulting to bare-pod.
func SandboxModeFromCR(cr *olsv1alpha1.OLSConfig) olsv1alpha1.SandboxMode {
	if cr.Spec.AgenticOLS == nil || cr.Spec.AgenticOLS.SandboxMode == "" {
		return olsv1alpha1.SandboxModeBarePod
	}
	return cr.Spec.AgenticOLS.SandboxMode
}

func agenticSandboxConfigFromCR(cr *olsv1alpha1.OLSConfig) olsv1alpha1.Config {
	if cr.Spec.AgenticOLS == nil {
		return olsv1alpha1.Config{}
	}
	return cr.Spec.AgenticOLS.AgenticSandboxConfig
}

func defaultSandboxResources() *corev1.ResourceRequirements {
	// Requests only (OpenShift / OLS-3397). Sandbox pods are mostly LLM invocation.
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Claims: []corev1.ResourceClaim{},
	}
}

const (
	sandboxHomeVolumeName          = "home"
	sandboxHomeMountPath           = "/home/agent"
	sandboxSkillsWorkdirVolumeName = "skills-workdir"
	sandboxSkillsWorkdirMountPath  = "/app/skills/.agents"
)

// GenerateSandboxPodSpec builds a base PodSpec for agentic sandbox pods:
// image, resources / tolerations / nodeSelector from AgenticSandboxConfig,
// and writable emptyDir mounts matching agentic-operator (home + skills-workdir).
// Replicas are ignored. No OTEL/MCP env or CA mounts — those come from the handoff ConfigMap/Secrets.
func GenerateSandboxPodSpec(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) *corev1.PodSpec {
	cfg := agenticSandboxConfigFromCR(cr)
	resources := utils.GetResourcesOrDefault(cfg.Resources, defaultSandboxResources())

	spec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:      utils.AgenticSandboxContainerName,
				Image:     r.GetAgenticSandboxImage(),
				Resources: *resources,
				VolumeMounts: []corev1.VolumeMount{
					{Name: sandboxHomeVolumeName, MountPath: sandboxHomeMountPath},
					{Name: sandboxSkillsWorkdirVolumeName, MountPath: sandboxSkillsWorkdirMountPath},
				},
			},
		},
		Volumes: []corev1.Volume{
			{Name: sandboxHomeVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: sandboxSkillsWorkdirVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}
	if len(cfg.Tolerations) > 0 {
		spec.Tolerations = cfg.Tolerations
	}
	if len(cfg.NodeSelector) > 0 {
		spec.NodeSelector = cfg.NodeSelector
	}
	return spec
}

// GenerateAgenticConfigurationConfigMap builds lightspeed-agentic-configuration.
// MCP endpoint / CA Secret name keys are set when introspection is enabled.
// Client CA Secrets themselves are reconciled by appserver.
func GenerateAgenticConfigurationConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	podSpec := GenerateSandboxPodSpec(r, cr)
	podSpecJSON, err := json.Marshal(podSpec)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrMarshalAgenticSandboxPodSpec, err)
	}

	ns := r.GetNamespace()
	otelHost := fmt.Sprintf("%s.%s.svc", utils.OtelCollectorServiceName, ns)

	data := map[string]string{
		utils.AgenticConfigurationSandboxModeKey:           string(SandboxModeFromCR(cr)),
		utils.AgenticConfigurationSandboxPodSpecKey:        string(podSpecJSON),
		utils.AgenticConfigurationOtelCollectorEndpointKey: fmt.Sprintf("%s:%d", otelHost, utils.OtelCollectorGRPCPort),
		utils.AgenticConfigurationOtelAdminEndpointKey:     fmt.Sprintf("https://%s:%d", otelHost, utils.OtelCollectorAdminPort),
		utils.AgenticConfigurationOtelCASecretKey:          utils.AgenticOtelCASecretName,
	}
	if utils.BoolDeref(cr.Spec.OLSConfig.IntrospectionEnabled, true) {
		data[utils.AgenticConfigurationMCPEndpointKey] = utils.OpenShiftMCPServerServiceURL(ns)
		data[utils.AgenticConfigurationMCPCASecretKey] = utils.AgenticMCPCASecretName
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AgenticConfigurationConfigMapName,
			Namespace: ns,
			Labels:    utils.GenerateAgenticIntegrationSelectorLabels(),
		},
		Data: data,
	}
	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetAgenticConfigurationConfigMapOwnerRef, err)
	}
	return cm, nil
}

// TouchAgenticConfiguration bumps an annotation on the handoff ConfigMap so its
// resourceVersion changes and agentic-operator can reload client CA material.
func TouchAgenticConfiguration(r reconciler.Reconciler, ctx context.Context) error {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AgenticConfigurationConfigMapName, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("agentic configuration configmap not found, skip touch",
				"configmap", utils.AgenticConfigurationConfigMapName)
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAgenticConfigurationConfigMap, err)
	}
	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
	}
	cm.Annotations[utils.AgenticConfigurationCertReloadAnnotation] = time.Now().Format(time.RFC3339Nano)
	if err := r.Update(ctx, cm); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrTouchAgenticConfigurationConfigMap, err)
	}
	r.GetLogger().Info("touched agentic configuration configmap", "configmap", cm.Name)
	return nil
}

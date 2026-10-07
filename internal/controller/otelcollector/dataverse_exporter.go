package otelcollector

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

const telemetryAuthRegistry = "cloud.openshift.com"

// GenerateOtelDataverseExporterConfigMap creates the OTEL-mode exporter config.
// OpenShift auth is supplied by the sidecar's projected in-cluster credentials, never ConfigMap data.
func GenerateOtelDataverseExporterConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	serviceID := utils.ServiceIDOLS
	if cr.Labels != nil {
		if _, hasRHOSOLightspeedLabel := cr.Labels[utils.RHOSOLightspeedOwnerIDLabel]; hasRHOSOLightspeedLabel {
			serviceID = utils.ServiceIDRHOSO
		}
	}

	configYAML, err := yaml.Marshal(map[string]interface{}{
		"data_mode":           "otel",
		"data_dir":            utils.OtelDataverseExporterDataMountPath,
		"otel_active_file":    path.Base(utils.OtelCollectorDataCollectionTraceFilePath),
		"ledger_file":         utils.OtelDataverseExporterLedgerFilePath,
		"archive_path_prefix": utils.OtelDataverseExporterArchivePathPrefix,
		"collection_interval": utils.OtelDataverseExporterCollectionIntervalSeconds,
		"cleanup_after_send":  false,
		"service_id":          serviceID,
		"ingress_server_url":  utils.OtelDataverseExporterIngressServerURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OTEL Dataverse exporter config: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelDataverseExporterConfigMapName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
		},
		Data: map[string]string{
			utils.OtelDataverseExporterConfigMapDataKey: string(configYAML),
		},
	}
	if err := controllerutil.SetControllerReference(cr, configMap, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to set OTEL Dataverse exporter ConfigMap owner reference: %w", err)
	}
	return configMap, nil
}

// GenerateOtelDataverseExporterClusterRole grants only the completed ClusterVersion read required by the exporter.
func GenerateOtelDataverseExporterClusterRole(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.OtelDataverseExporterClusterRoleName,
			Labels: utils.GenerateOtelCollectorSelectorLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"config.openshift.io"},
				Resources:     []string{"clusterversions"},
				ResourceNames: []string{"version"},
				Verbs:         []string{"get"},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, role, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to set OTEL Dataverse exporter ClusterRole owner reference: %w", err)
	}
	return role, nil
}

// GenerateOtelDataverseExporterPullSecretClusterRole grants a named Secret read scoped by a RoleBinding in openshift-config.
func GenerateOtelDataverseExporterPullSecretClusterRole(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.OtelDataverseExporterPullSecretClusterRoleName,
			Labels: utils.GenerateOtelCollectorSelectorLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{utils.TelemetryPullSecretName},
				Verbs:         []string{"get"},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, role, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to set OTEL Dataverse exporter pull-secret ClusterRole owner reference: %w", err)
	}
	return role, nil
}

// GenerateOtelDataverseExporterClusterRoleBinding grants the Collector ServiceAccount only the ClusterVersion read.
func GenerateOtelDataverseExporterClusterRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRoleBinding, error) {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.OtelDataverseExporterClusterRoleBindingName,
			Labels: utils.GenerateOtelCollectorSelectorLabels(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.OtelCollectorServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OtelDataverseExporterClusterRoleName,
		},
	}
	if err := controllerutil.SetControllerReference(cr, binding, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to set OTEL Dataverse exporter ClusterRoleBinding owner reference: %w", err)
	}
	return binding, nil
}

// GenerateOtelDataverseExporterPullSecretRoleBinding scopes pull-secret access to openshift-config.
func GenerateOtelDataverseExporterPullSecretRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.RoleBinding, error) {
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
			Namespace: utils.TelemetryPullSecretNamespace,
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.OtelCollectorServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OtelDataverseExporterPullSecretClusterRoleName,
		},
	}
	if err := controllerutil.SetControllerReference(cr, binding, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to set OTEL Dataverse exporter pull-secret RoleBinding owner reference: %w", err)
	}
	return binding, nil
}

func dataverseExporterEnabled(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (bool, error) {
	if cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled {
		return false, nil
	}

	pullSecret := &corev1.Secret{}
	key := client.ObjectKey{Name: utils.TelemetryPullSecretName, Namespace: utils.TelemetryPullSecretNamespace}
	if err := r.Get(ctx, key, pullSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read telemetry pull secret %s/%s: %w", key.Namespace, key.Name, err)
	}

	dockerConfigJSON, ok := pullSecret.Data[corev1.DockerConfigJsonKey]
	if !ok {
		return false, fmt.Errorf("telemetry pull secret %s/%s does not contain %s", key.Namespace, key.Name, corev1.DockerConfigJsonKey)
	}

	var dockerConfig struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(dockerConfigJSON, &dockerConfig); err != nil {
		return false, fmt.Errorf("failed to decode telemetry pull secret %s/%s %s: %w", key.Namespace, key.Name, corev1.DockerConfigJsonKey, err)
	}

	registryAuth, ok := dockerConfig.Auths[telemetryAuthRegistry]
	return ok && strings.TrimSpace(registryAuth.Auth) != "", nil
}

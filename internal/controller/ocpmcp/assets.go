// Package ocpmcp reconciles the standalone OpenShift MCP server (ocp-mcp) operand.
package ocpmcp

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// configTOML is the openshift-mcp-server runtime config.
// Denied resources keep Secret (and RBAC) data out of the LLM path; toolsets are pinned
// so upstream default changes do not affect OLS. Metrics uses in-cluster Thanos/Alertmanager.
const configTOML = `# Denied resources prevent the MCP server from accessing these Kubernetes resource types.
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
# Query-safety PromQL checks (not RBAC). "!tsdb" disables TSDB-dependent guardrails that
# OpenShift Thanos Querier often lacks (/api/v1/status/tsdb); other guardrails stay on.
# Auth still uses the caller's bearer token forwarded to Thanos/Alertmanager.
guardrails = "!tsdb"
`

func selectorLabels() map[string]string {
	return map[string]string{
		"app":                          utils.OpenShiftMCPServerDeploymentName,
		"app.kubernetes.io/component":  utils.OpenShiftMCPServerComponentLabel,
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       utils.OpenShiftMCPServerDeploymentName,
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

// GenerateServiceAccount generates the standalone MCP server ServiceAccount.
// The SA has no RBAC bindings; callers pass through their own token.
func GenerateServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa, err := utils.GenerateServiceAccount(r, cr, utils.OpenShiftMCPServerServiceAccountName)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrGenerateOpenShiftMCPServerServiceAccount, err)
	}
	return sa, nil
}

// GenerateConfigMap generates the TOML ConfigMap for openshift-mcp-server.
func GenerateConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OpenShiftMCPServerConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
		},
		Data: map[string]string{
			utils.OpenShiftMCPServerConfigFilename: configTOML,
		},
	}
	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOpenShiftMCPServerConfigMapOwnerReference, err)
	}
	return cm, nil
}

// GenerateCAConfigMap generates an empty ConfigMap annotated for service-ca
// inject-cabundle. Clients (app-server) mount service-ca.crt for TLS verify.
func GenerateCAConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OpenShiftMCPServerCAConfigMapName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
			Annotations: map[string]string{
				utils.InjectCABundleAnnotationKey: "true",
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOpenShiftMCPServerCAConfigMapOwnerReference, err)
	}
	return cm, nil
}

// GenerateService generates the ClusterIP Service on HTTPS port 8443 with a
// service-ca serving-cert annotation.
func GenerateService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OpenShiftMCPServerServiceName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.OpenShiftMCPServerCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       utils.OpenShiftMCPServerHTTPSPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("https"),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOpenShiftMCPServerServiceOwnerReference, err)
	}
	return &service, nil
}

// GenerateNetworkPolicy allows ingress to the MCP pods from any pod in the
// operator namespace on HTTPS :8443 (app-server and future sandbox consumers).
func GenerateNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	tcp := corev1.ProtocolTCP
	httpsPort := intstr.FromInt32(utils.OpenShiftMCPServerHTTPSPort)
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OpenShiftMCPServerNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: selectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &tcp,
							Port:     &httpsPort,
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOpenShiftMCPServerNetworkPolicyOwnerReference, err)
	}
	return &np, nil
}

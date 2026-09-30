package rhokp

import (
	"fmt"
	"strings"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func selectorLabels() map[string]string {
	return map[string]string{
		"app":                          utils.RHOKPDeploymentName,
		"app.kubernetes.io/component":  utils.RHOKPComponentLabel,
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       utils.RHOKPDeploymentName,
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

// GenerateService generates the ClusterIP Service on HTTPS port 8443 with a
// service-ca serving-cert annotation.
func GenerateService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RHOKPServiceName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.RHOKPCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       utils.RHOOKPImageHTTPSPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("https"),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetRHOKPServiceOwnerReference, err)
	}
	return &service, nil
}

// GenerateNetworkPolicy allows HTTPS ingress to RHOKP pods from clients in the
// operator namespace and cluster Prometheus pods in openshift-monitoring.
func GenerateNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	tcp := corev1.ProtocolTCP
	httpsPort := intstr.FromInt32(utils.RHOOKPImageHTTPSPort)
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RHOKPNetworkPolicyName,
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
						{
							PodSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{Key: "app.kubernetes.io/name", Operator: metav1.LabelSelectorOpIn, Values: []string{"prometheus"}},
									{Key: "prometheus", Operator: metav1.LabelSelectorOpIn, Values: []string{"k8s"}},
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": utils.ClientCACmNamespace,
								},
							},
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
		return nil, fmt.Errorf("%s: %w", utils.ErrSetRHOKPNetworkPolicyOwnerReference, err)
	}
	return &np, nil
}

// generateServiceMonitor generates a ServiceMonitor for HTTPS scraping of
// RHOKP Solr metrics on :8443, path /solr/admin/metrics. Server TLS only (service-ca).
func generateServiceMonitor(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
	metaLabels := selectorLabels()
	metaLabels["monitoring.openshift.io/collection-profile"] = "full"
	metaLabels["app.kubernetes.io/component"] = "metrics"
	metaLabels["openshift.io/user-monitoring"] = "false"

	valFalse := false
	serverName := strings.Join([]string{utils.RHOKPServiceName, r.GetNamespace(), "svc"}, ".")
	var schemeHTTPS monv1.Scheme = "https"

	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RHOKPServiceMonitorName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "https",
					Path:     utils.RHOKPMetricsPath,
					Interval: "30s",
					Scheme:   &schemeHTTPS,
					HTTPConfigWithProxyAndTLSFiles: monv1.HTTPConfigWithProxyAndTLSFiles{
						HTTPConfigWithTLSFiles: monv1.HTTPConfigWithTLSFiles{
							TLSConfig: &monv1.TLSConfig{
								TLSFilesConfig: monv1.TLSFilesConfig{
									CAFile: "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
								},
								SafeTLSConfig: monv1.SafeTLSConfig{
									InsecureSkipVerify: &valFalse,
									ServerName:         &serverName,
								},
							},
						},
					},
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: selectorLabels(),
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &serviceMonitor, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetRHOKPServiceMonitorOwnerReference, err)
	}
	return &serviceMonitor, nil
}

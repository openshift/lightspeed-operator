package lcore

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// Service account for running LCore server
func GenerateServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerServiceAccountName,
			Namespace: r.GetNamespace(),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &sa, r.GetScheme()); err != nil {
		return nil, err
	}

	return &sa, nil
}

// SAR = SubjectAccessReview
// SARClusterRole provides permissions for the OLS Application Server to perform
// authentication and authorization checks for users accessing the OLS API.
func GenerateSARClusterRole(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSAppServerSARRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"config.openshift.io"},
				Resources: []string{"clusterversions"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{"pull-secret"},
				Verbs:         []string{"get"},
			},
			{
				NonResourceURLs: []string{"/ls-access"},
				Verbs:           []string{"get"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &role, r.GetScheme()); err != nil {
		return nil, err
	}

	return &role, nil
}

// Binding SARClusterRole to server account
func generateSARClusterRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRoleBinding, error) {
	rb := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.OLSAppServerSARRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.OLSAppServerServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.OLSAppServerSARRoleName,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rb, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rb, nil
}

func GenerateService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	annotations := map[string]string{}

	// Let service-ca operator generate a TLS certificate if the user does not provide their own
	if cr.Spec.OLSConfig.TLSConfig == nil || cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name == "" {
		annotations[utils.ServingCertSecretAnnotationKey] = utils.OLSCertsSecretName
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        utils.OLSAppServerServiceName,
			Namespace:   r.GetNamespace(),
			Labels:      utils.GenerateAppServerSelectorLabels(),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					Port:       utils.OLSAppServerServicePort,
					TargetPort: intstr.Parse("https"),
				},
			},
			Selector: utils.GenerateAppServerSelectorLabels(),
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GenerateServiceMonitor(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
	metaLabels := utils.GenerateAppServerSelectorLabels()
	metaLabels["monitoring.openshift.io/collection-profile"] = "full"
	metaLabels["app.kubernetes.io/component"] = "metrics"
	metaLabels["openshift.io/user-monitoring"] = "true"

	valFalse := false
	serverName := strings.Join([]string{utils.OLSAppServerServiceName, r.GetNamespace(), "svc"}, ".")
	var schemeHTTPS monv1.Scheme = "https"

	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AppServerServiceMonitorName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "https",
					Path:     utils.AppServerMetricsPath,
					Interval: "30s",
					Scheme:   &schemeHTTPS,
					HTTPConfigWithProxyAndTLSFiles: monv1.HTTPConfigWithProxyAndTLSFiles{
						HTTPConfigWithTLSFiles: monv1.HTTPConfigWithTLSFiles{
							TLSConfig: &monv1.TLSConfig{
								TLSFilesConfig: monv1.TLSFilesConfig{
									CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
									CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
									KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
								},
								SafeTLSConfig: monv1.SafeTLSConfig{
									InsecureSkipVerify: &valFalse,
									ServerName:         &serverName,
								},
							},
							HTTPConfigWithoutTLS: monv1.HTTPConfigWithoutTLS{
								Authorization: &monv1.SafeAuthorization{
									Type: "Bearer",
									Credentials: &corev1.SecretKeySelector{
										Key: "token",
										LocalObjectReference: corev1.LocalObjectReference{
											Name: utils.MetricsReaderServiceAccountTokenSecretName,
										},
									},
								},
							},
						},
					},
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &serviceMonitor, r.GetScheme()); err != nil {
		return nil, err
	}

	return &serviceMonitor, nil
}

func GeneratePrometheusRule(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.PrometheusRule, error) {
	metaLabels := utils.GenerateAppServerSelectorLabels()
	metaLabels["app.kubernetes.io/component"] = "metrics"

	rule := monv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AppServerPrometheusRuleName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.PrometheusRuleSpec{
			Groups: []monv1.RuleGroup{
				{
					Name: "ols.operations.rules",
					Rules: []monv1.Rule{
						{
							Record: "ols:rest_api_query_calls_total:2xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"2..\"})"),
							Labels: map[string]string{"status_code": "2xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:4xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"4..\"})"),
							Labels: map[string]string{"status_code": "4xx"},
						},
						{
							Record: "ols:rest_api_query_calls_total:5xx",
							Expr:   intstr.FromString("sum by(status_code) (ols_rest_api_calls_total{path=\"/v1/streaming_query\",status_code=~\"5..\"})"),
							Labels: map[string]string{"status_code": "5xx"},
						},
						{
							Record: "ols:provider_model_configuration",
							Expr:   intstr.FromString("max by (provider,model) (ols_provider_model_configuration)"),
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rule, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rule, nil
}

func GenerateAppServerNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					// allow prometheus to scrape metrics (both cluster and user-workload monitoring)
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app.kubernetes.io/name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"prometheus"},
									},
									{
										Key:      "prometheus",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"k8s", "user-workload"},
									},
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "kubernetes.io/metadata.name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"openshift-monitoring", "openshift-user-workload-monitoring"},
									},
								},
							},
						},
					},

					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
						},
					},
				},
				{
					// allow the console to access the API
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "console",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-console",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
						},
					},
				},
				{
					// allow ingress controller to access the API
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"network.openshift.io/policy-group": "ingress",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OLSAppServerContainerPort)}[0],
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, err
	}

	return &np, nil
}

func GenerateMetricsReaderSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.MetricsReaderServiceAccountTokenSecretName,
			Namespace: r.GetNamespace(),
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": utils.MetricsReaderServiceAccountName,
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":      "service-account-token",
				"app.kubernetes.io/component": "metrics",
				"app.kubernetes.io/part-of":   "lightspeed-operator",
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	if err := controllerutil.SetControllerReference(cr, secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return secret, nil
}

// GenerateLlamaStackConfigMap generates the Llama Stack configuration ConfigMap
func GenerateLlamaStackConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	llamaStackYAML, err := buildLlamaStackYAML(r, ctx, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to build Llama Stack YAML: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.LlamaStackConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			"run.yaml": llamaStackYAML,
		},
	}

	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return cm, nil
}

// GenerateLcoreConfigMap generates the LCore configuration ConfigMap
func GenerateLcoreConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	// Build OLS config YAML from components
	lcoreConfigYAML, err := buildLCoreConfigYAML(r, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to build OLS config YAML: %w", err)
	}

	// Create ConfigMap
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.LCoreConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			"lightspeed-stack.yaml": lcoreConfigYAML,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return &cm, nil
}

// generateExporterConfigMap generates the ConfigMap for the data exporter
func generateExporterConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	serviceID := utils.ServiceIDOLS
	if cr.Labels != nil {
		if _, hasRHOSLightspeedLabel := cr.Labels[utils.RHOSOLightspeedOwnerIDLabel]; hasRHOSLightspeedLabel {
			serviceID = utils.ServiceIDRHOSO
		}
	}

	// Collection interval is set to 300 seconds in production (5 minutes)
	exporterConfigContent := fmt.Sprintf(`service_id: "%s"
ingress_server_url: "https://console.redhat.com/api/ingress/v1/upload"
allowed_subdirs:
 - feedback
 - transcripts
 - config_status
# Collection settings
collection_interval: 300
cleanup_after_send: true
ingress_connection_timeout: 30`, serviceID)

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ExporterConfigCmName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			utils.ExporterConfigFilename: exporterConfigContent,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return &cm, nil
}

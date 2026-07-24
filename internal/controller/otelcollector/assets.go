// Package otelcollector reconciles the in-cluster OTEL Collector operand.
package otelcollector

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

const postgresConnectionStringRef = "${env:" + utils.OtelCollectorPostgresConnectionStringEnvVar + "}"

// GenerateOtelCollectorConfigMap generates the collector runtime ConfigMap.
func GenerateOtelCollectorConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	configYAML, err := buildCollectorConfigYAML(cr)
	if err != nil {
		return nil, err
	}

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorConfigMapName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
		},
		Data: map[string]string{
			utils.OtelCollectorConfigMapDataKey: string(configYAML),
		},
	}
	if err := controllerutil.SetControllerReference(cr, &configMap, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorConfigMapOwnerReference, err)
	}
	return &configMap, nil
}

// GenerateOtelCollectorClientConfigMap generates the client connectivity ConfigMap
// consumed by agentic-operator (OTLP endpoint, admin API URL, and CA).
// CA PEM is the OpenShift service-ca bundle (same CA that signs the serving cert),
// not the leaf tls.crt — mounting the leaf as a CA breaks gRPC/OpenSSL verify.
func GenerateOtelCollectorClientConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	caCM := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: utils.OLSCAConfigMap, Namespace: r.GetNamespace()}, caCM); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorClientCAConfigMap, err)
	}
	caPEM, ok := caCM.Data[utils.AppOtelCollectorCACertFile]
	if !ok || caPEM == "" {
		return nil, fmt.Errorf("%s: key %q missing or empty in ConfigMap %s",
			utils.ErrGetOtelCollectorClientCAConfigMap, utils.AppOtelCollectorCACertFile, utils.OLSCAConfigMap)
	}

	ns := r.GetNamespace()
	host := fmt.Sprintf("%s.%s.svc", utils.OtelCollectorServiceName, ns)

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorClientConfigMapName,
			Namespace: ns,
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
		},
		Data: map[string]string{
			utils.OtelCollectorClientCollectorEndpointKey: fmt.Sprintf("%s:%d", host, utils.OtelCollectorGRPCPort),
			utils.OtelCollectorClientAdminEndpointKey:     fmt.Sprintf("https://%s:%d", host, utils.OtelCollectorAdminPort),
			utils.OtelCollectorClientCACertKey:            caPEM,
		},
	}
	if err := controllerutil.SetControllerReference(cr, &configMap, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorClientConfigMapOwnerReference, err)
	}
	return &configMap, nil
}

// GenerateOtelCollectorService generates the collector Service with a service-ca serving cert.
func GenerateOtelCollectorService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorServiceName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.OtelCollectorCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: utils.GenerateOtelCollectorSelectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "otlp-grpc",
					Port:       utils.OtelCollectorGRPCPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("otlp-grpc"),
				},
				{
					Name:       "otlp-http",
					Port:       utils.OtelCollectorHTTPPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("otlp-http"),
				},
				{
					Name:       "admin",
					Port:       utils.OtelCollectorAdminPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("admin"),
				},
				{
					Name:       "metrics",
					Port:       utils.OtelCollectorMetricsPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("metrics"),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorServiceOwnerReference, err)
	}
	return &service, nil
}

// PostgresConnectionString builds a Postgres DSN from lightspeed-postgres-secret for collector env injection.
func PostgresConnectionString(r reconciler.Reconciler, ctx context.Context) (string, error) {
	postgresSecret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.PostgresSecretName, Namespace: r.GetNamespace()}, postgresSecret)
	if err != nil {
		return "", fmt.Errorf("%s: %w", utils.ErrGetPostgresSecret, err)
	}

	password, ok := postgresSecret.Data[utils.PostgresSecretKeyName]
	if !ok || len(password) == 0 {
		return "", fmt.Errorf("%s: postgres secret %s missing key %q", utils.ErrGetPostgresSecret, utils.PostgresSecretName, utils.PostgresSecretKeyName)
	}

	return buildPostgresConnectionString(r.GetNamespace(), string(password))
}

// GenerateOtelCollectorPostgresSecret generates the Secret used by the collector
// to source POSTGRES_CONNECTION_STRING via SecretKeyRef.
func GenerateOtelCollectorPostgresSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	connectionString, err := PostgresConnectionString(r, ctx)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorPostgresDSNSecretName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
		},
		Data: map[string][]byte{
			utils.OtelCollectorPostgresConnectionStringSecretKey: []byte(connectionString),
		},
		Type: corev1.SecretTypeOpaque,
	}
	if err := controllerutil.SetControllerReference(cr, secret, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorPostgresSecretOwnerReference, err)
	}
	return secret, nil
}

// GenerateOtelCollectorNetworkPolicy restricts collector ingress to:
//   - pods in the operator namespace on OTLP gRPC and admin HTTPS
//   - Prometheus in openshift-monitoring on HTTPS metrics :8888
func GenerateOtelCollectorNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	tcp := corev1.ProtocolTCP
	grpcPort := intstr.FromInt32(utils.OtelCollectorGRPCPort)
	adminPort := intstr.FromInt32(utils.OtelCollectorAdminPort)
	metricsPort := intstr.FromInt32(utils.OtelCollectorMetricsPort)
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GenerateOtelCollectorSelectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							// Empty podSelector selects all pods in the NetworkPolicy namespace.
							PodSelector: &metav1.LabelSelector{},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &tcp,
							Port:     &grpcPort,
						},
						{
							Protocol: &tcp,
							Port:     &adminPort,
						},
					},
				},
				{
					// Allow cluster Prometheus to scrape HTTPS metrics.
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
										Values:   []string{"k8s"},
									},
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
							Port:     &metricsPort,
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
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorNetworkPolicyOwnerReference, err)
	}
	return &np, nil
}

// GenerateOtelCollectorServiceMonitor generates a ServiceMonitor for HTTPS scraping of
// collector metrics on :8888. Server TLS only (service-ca); no client mTLS or Bearer.
func GenerateOtelCollectorServiceMonitor(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*monv1.ServiceMonitor, error) {
	metaLabels := utils.GenerateOtelCollectorSelectorLabels()
	metaLabels["monitoring.openshift.io/collection-profile"] = "full"
	metaLabels["app.kubernetes.io/component"] = "metrics"
	metaLabels["openshift.io/user-monitoring"] = "false"

	valFalse := false
	serverName := strings.Join([]string{utils.OtelCollectorServiceName, r.GetNamespace(), "svc"}, ".")
	var schemeHTTPS monv1.Scheme = "https"

	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorServiceMonitorName,
			Namespace: r.GetNamespace(),
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "metrics",
					Path:     utils.OtelCollectorMetricsPath,
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
				MatchLabels: utils.GenerateOtelCollectorSelectorLabels(),
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &serviceMonitor, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorServiceMonitorOwnerReference, err)
	}
	return &serviceMonitor, nil
}

// GenerateOtelCollectorServiceAccount generates the collector ServiceAccount.
func GenerateOtelCollectorServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa, err := utils.GenerateServiceAccount(r, cr, utils.OtelCollectorServiceAccountName)
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func buildPostgresConnectionString(namespace, password string) (string, error) {
	host := strings.Join([]string{utils.PostgresServiceName, namespace, "svc"}, ".")
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(utils.PostgresDefaultUser, password),
		Host:   fmt.Sprintf("%s:%d", host, utils.PostgresServicePort),
		Path:   "/" + utils.PostgresDefaultDbName,
	}
	q := u.Query()
	q.Set("sslmode", utils.PostgresDefaultSSLMode)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func buildCollectorConfigYAML(cr *olsv1alpha1.OLSConfig) ([]byte, error) {
	loggingEnabled := utils.BoolDeref(cr.Spec.Audit.Logging, true)
	tracingEndpoint := cr.Spec.Audit.TracingEndpoint

	config := map[string]interface{}{
		"receivers": map[string]interface{}{
			"otlp": map[string]interface{}{
				"protocols": map[string]interface{}{
					"grpc": otlpTLSEndpoint("0.0.0.0:4317"),
					"http": otlpTLSEndpoint("0.0.0.0:4318"),
				},
			},
		},
		"processors": map[string]interface{}{
			"batch": map[string]interface{}{
				"timeout":         "1s",
				"send_batch_size": 100,
			},
		},
		"extensions": collectorExtensions(),
	}

	exporters := map[string]interface{}{}
	connectors := map[string]interface{}{}
	pipelines := map[string]interface{}{}

	if loggingEnabled {
		exporters["postgres"] = postgresExporterConfig()
		connectors["routing/logs"] = map[string]interface{}{
			"default_pipelines": []interface{}{},
			"table": []interface{}{
				map[string]interface{}{
					"condition": fmt.Sprintf(`attributes["service.name"] == %q`, utils.OtelSandboxServiceName),
					"pipelines": []interface{}{"logs/postgres"},
				},
			},
		}
		pipelines["logs"] = map[string]interface{}{
			"receivers":  []interface{}{"otlp"},
			"processors": []interface{}{"batch"},
			"exporters":  []interface{}{"routing/logs"},
		}
		pipelines["logs/postgres"] = map[string]interface{}{
			"receivers": []interface{}{"routing/logs"},
			"exporters": []interface{}{"postgres"},
		}
	}

	if tracingEndpoint != "" {
		exporters["otlp/tracing"] = map[string]interface{}{
			"endpoint": "${env:" + utils.OtelCollectorTracesBackendEndpointEnvVar + "}",
			"tls": map[string]interface{}{
				"insecure": false,
			},
		}
		connectors["routing/traces"] = map[string]interface{}{
			"default_pipelines": []interface{}{},
			"table": []interface{}{
				map[string]interface{}{
					"condition": fmt.Sprintf(`resource.attributes["service.name"] == %q`, utils.OtelAppServerServiceName),
					"pipelines": []interface{}{"traces/lightspeed"},
				},
			},
		}
		pipelines["traces"] = map[string]interface{}{
			"receivers":  []interface{}{"otlp"},
			"processors": []interface{}{"batch"},
			"exporters":  []interface{}{"routing/traces"},
		}
		pipelines["traces/lightspeed"] = map[string]interface{}{
			"receivers": []interface{}{"routing/traces"},
			"exporters": []interface{}{"otlp/tracing"},
		}
	}

	if len(exporters) > 0 {
		config["exporters"] = exporters
	}
	if len(connectors) > 0 {
		config["connectors"] = connectors
	}

	config["service"] = map[string]interface{}{
		"extensions": collectorServiceExtensions(),
		"pipelines":  pipelines,
		"telemetry": map[string]interface{}{
			"logs": map[string]interface{}{
				"level": "info",
			},
			"metrics": map[string]interface{}{
				"readers": []interface{}{
					map[string]interface{}{
						"pull": map[string]interface{}{
							"exporter": map[string]interface{}{
								"prometheus": map[string]interface{}{
									"host":                "127.0.0.1",
									"port":                utils.OtelCollectorMetricsInternalPort,
									"without_type_suffix": true,
									"without_units":       true,
								},
							},
						},
					},
				},
			},
		},
	}

	return yaml.Marshal(config)
}

func otlpTLSEndpoint(endpoint string) map[string]interface{} {
	return map[string]interface{}{
		"endpoint": endpoint,
		"tls": map[string]interface{}{
			"cert_file": utils.OtelCollectorServingCertTLSFile,
			"key_file":  utils.OtelCollectorServingCertTLSKeyFile,
		},
	}
}

func postgresExporterConfig() map[string]interface{} {
	return map[string]interface{}{
		"connection_string": postgresConnectionStringRef,
		"schema":            "templogs",
		"logs_table":        "logs",
		"retry_on_failure": map[string]interface{}{
			"enabled": true,
		},
		"sending_queue": map[string]interface{}{
			"enabled":       true,
			"num_consumers": 4,
			"queue_size":    1000,
			"storage":       "file_storage",
		},
	}
}

func collectorExtensions() map[string]interface{} {
	return map[string]interface{}{
		"health_check": map[string]interface{}{
			"endpoint": fmt.Sprintf("0.0.0.0:%d", utils.OtelCollectorHealthCheckPort),
		},
		"file_storage": map[string]interface{}{
			"directory":        utils.OtelCollectorFileStorageMountPath,
			"create_directory": true,
			"compaction": map[string]interface{}{
				"on_start":  true,
				"directory": utils.OtelCollectorFileStorageMountPath + "/compaction",
			},
		},
		"postgres_admin": map[string]interface{}{
			"endpoint":          fmt.Sprintf("0.0.0.0:%d", utils.OtelCollectorAdminPort),
			"connection_string": postgresConnectionStringRef,
			"schema":            "templogs",
			"logs_table":        "logs",
			"tls_cert_file":     utils.OtelCollectorServingCertTLSFile,
			"tls_key_file":      utils.OtelCollectorServingCertTLSKeyFile,
		},
		utils.OtelCollectorHTTPSMetricsExtension: map[string]interface{}{
			"endpoint":      fmt.Sprintf("0.0.0.0:%d", utils.OtelCollectorMetricsPort),
			"upstream":      utils.OtelCollectorMetricsUpstreamURL,
			"tls_cert_file": utils.OtelCollectorServingCertTLSFile,
			"tls_key_file":  utils.OtelCollectorServingCertTLSKeyFile,
		},
	}
}

func collectorServiceExtensions() []interface{} {
	return []interface{}{"health_check", "file_storage", "postgres_admin", utils.OtelCollectorHTTPSMetricsExtension}
}

// Package otelcollector reconciles the in-cluster OTEL Collector operand.
package otelcollector

import (
	"context"
	"fmt"
	"net/url"
	"strings"

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

// GenerateOtelCollectorNetworkPolicy restricts collector ingress to pods in the operator
// namespace (OLS app-server, agentic-operator, sandbox pods, etc.) on OTLP gRPC.
func GenerateOtelCollectorNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	tcp := corev1.ProtocolTCP
	port := intstr.FromInt32(utils.OtelCollectorGRPCPort)
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
							Port:     &port,
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
		"extensions": collectorExtensions(loggingEnabled),
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
			"default_pipelines": []interface{}{"traces/lightspeed"},
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
		"extensions": collectorServiceExtensions(loggingEnabled),
		"pipelines":  pipelines,
		"telemetry": map[string]interface{}{
			"logs": map[string]interface{}{
				"level": "info",
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

func collectorExtensions(loggingEnabled bool) map[string]interface{} {
	extensions := map[string]interface{}{
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
	}
	if loggingEnabled {
		extensions["postgres_admin"] = map[string]interface{}{
			"endpoint":          fmt.Sprintf("0.0.0.0:%d", utils.OtelCollectorAdminPort),
			"connection_string": postgresConnectionStringRef,
			"schema":            "templogs",
			"logs_table":        "logs",
			"tls_cert_file":     utils.OtelCollectorServingCertTLSFile,
			"tls_key_file":      utils.OtelCollectorServingCertTLSKeyFile,
		}
	}
	return extensions
}

func collectorServiceExtensions(loggingEnabled bool) []interface{} {
	extensions := []interface{}{"health_check", "file_storage"}
	if loggingEnabled {
		extensions = append(extensions, "postgres_admin")
	}
	return extensions
}

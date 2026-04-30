# Observability

The operator configures monitoring, health probes, and status reporting for all components.

## Behavioral Rules

### Prometheus Metrics
1. The operator creates ServiceMonitor resources for both the operator itself (`controller-manager-metrics-monitor`) and the backend (`lightspeed-app-server-monitor`) if Prometheus Operator CRDs are available. Availability is checked at startup via `IsPrometheusOperatorAvailable()`.
2. ServiceMonitors are configured for HTTPS scraping with mTLS: CA from `/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt`, client cert from `/etc/prometheus/secrets/metrics-client-certs/tls.crt`, client key from the matching key file. `insecureSkipVerify` is set to `false`. The backend ServiceMonitor also includes Bearer token authorization from the `metrics-reader-token` Secret.
3. The operator creates a PrometheusRule (`lightspeed-app-server-prometheus-rule`) with recording rules that aggregate query call counts by HTTP status code class (`ols:rest_api_query_calls_total:2xx`, `ols:rest_api_query_calls_total:4xx`, `ols:rest_api_query_calls_total:5xx`) and track provider/model configuration (`ols:provider_model_configuration`).
4. Metrics are scraped at a fixed 30-second interval.
5. If Prometheus Operator CRDs are not installed, ServiceMonitor and PrometheusRule creation is silently skipped. The `PrometheusAvailable` flag is set at operator startup and not re-checked.

### Health Probes
6. The AppServer backend uses HTTPS health probes: readiness at `/readiness` and liveness at `/liveness`, both on the `https` port (8443) with `URISchemeHTTPS`. Initial delay: 30s, period: 30s, timeout: 30s, failure threshold: 15.
7. The LCore lightspeed-stack container uses exec-based health probes that curl the HTTPS endpoints with a bearer token from the service account: `curl -k --fail -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" https://localhost:8443/liveness`. Initial delay: 20s, period: 10s, timeout: 5s, failure threshold: 3.
8. The Llama Stack container (LCore server mode) uses HTTP health probes at `/v1/health` on port 8321. Initial delay: 60s (to account for model download and warmup), period: 10s, timeout: 5s, failure threshold: 3.
9. PostgreSQL uses the standard PostgreSQL health check mechanism via the postgres container image.
10. All probe parameters are set as internal constants in the deployment generation code. They are not configurable via the CR.

### Status Reporting
11. The operator reports status via four condition types defined in `utils/types.go`: `ApiReady`, `CacheReady`, `ConsolePluginReady`, and `ResourceReconciliation` (used for Phase 1 failures).
12. `status.overallStatus` aggregates all conditions: `Ready` when all deployment conditions are `True`, `NotReady` otherwise. The field is required (no `omitempty`).
13. When deployments fail health checks, `status.diagnosticInfo` is populated with per-pod diagnostic entries. Diagnostics are collected by listing pods matching the deployment's selector labels and inspecting container and pod statuses.
14. Each `PodDiagnostic` entry includes: `failedComponent` (matching the condition type, e.g., `ApiReady`), `podName`, `containerName` (empty string for pod-level issues), `reason`, `message`, `exitCode` (pointer, set for terminated containers), `type` (diagnostic category), and `lastUpdated` timestamp.
15. Diagnostic types categorize the failure: `ContainerWaiting` (image pull issues, CrashLoopBackOff, pending states), `ContainerTerminated` (crashes, OOM, non-zero exit codes), `PodScheduling` (unschedulable pods), `PodCondition` (readiness failures for running pods without container-level diagnostics).
16. Terminal/recurring failures (`CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `OOMKilled`, `PreviousCrash:*`) cause the deployment status to be marked as `Failed`. Other diagnostic entries result in `Progressing` status. Both trigger exponential backoff retries via returned errors.

### Operator Metrics
17. The operator exposes its own metrics endpoint, optionally secured with mTLS via the `--secure-metrics-server` flag.
18. When mTLS is enabled, the operator reads the client CA from the `openshift-monitoring/metrics-client-ca` ConfigMap (key `client-ca.crt`).
19. The operator's TLS profile for metrics follows the OLSConfig CR's `spec.ols.tlsSecurityProfile` or falls back to the cluster API server's profile.

### Data Collection
20. The data collector sidecar (`lightspeed-to-dataverse-exporter`) exports feedback and transcript data to the Red Hat data pipeline at `https://console.redhat.com/api/ingress/v1/upload`. It runs in `openshift` mode to use the cluster ID as identity.
21. Data collection is enabled only when both conditions are met: (a) user data collection is not fully disabled (at least one of `spec.ols.userDataCollection.feedbackDisabled` or `spec.ols.userDataCollection.transcriptsDisabled` is false), AND (b) the telemetry pull secret (`openshift-config/pull-secret`) contains valid `cloud.openshift.com` credentials in its `.dockerconfigjson` data.
22. The service ID for data collection is `ols` by default, or `rhos-lightspeed` if the OLSConfig CR has the `openstack.org/lightspeed-owner-id` label.
23. The exporter config is generated as a ConfigMap (`lightspeed-exporter-config`) with a fixed 300-second collection interval.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.ols.logLevel` | Log level for backend service (app, lib, uvicorn levels all set to this value) |
| `spec.olsDataCollector.logLevel` | Log level for data collector sidecar (defaults to `info`) |
| `spec.ols.userDataCollection.feedbackDisabled` | Disable feedback collection |
| `spec.ols.userDataCollection.transcriptsDisabled` | Disable transcript collection |

## Constraints

1. ServiceMonitor and PrometheusRule are only created when Prometheus Operator CRDs are detected at operator startup. There is no runtime re-check.
2. Data collection requires the telemetry pull secret with `cloud.openshift.com` auth; removing the secret or the auth entry disables collection.
3. Diagnostics are cleared from status when the corresponding deployment becomes healthy (the entire `diagnosticInfo` array is rebuilt from scratch on each status update).
4. Health probe parameters are internal constants and cannot be customized via the CR.

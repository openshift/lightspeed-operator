# Agentic Data Collection — Classic Operator

[PLANNED: OLS-3569] This specification defines only the lightspeed-operator behavior for deploying and reconciling Agentic product data collection. All product-wide semantics and cross-repository ownership are defined by `../../../../.ai/spec/what/agentic-data-collection.md`; the accepted architecture is recorded in `../../../../.ai/spec/decisions/0042-agentic-data-collection-via-otel.md`.

## Behavioral Rules

### Gate and Configuration Surface

1. [PLANNED: OLS-3569] The Agentic product-collection pipeline, `emptyDir`, mounts, and exporter sidecar are absent when the Agentic v2 bundle is unavailable. The parent spec owns platform and bundle eligibility. Where that bundle is available, the operator enables Agentic collection only when `spec.ols.userDataCollection.transcriptsDisabled` is `false` or absent and `openshift-config/pull-secret` contains usable `cloud.openshift.com` telemetry credentials in `.dockerconfigjson`.
2. [PLANNED: OLS-3569] `spec.ols.userDataCollection.feedbackDisabled`, `spec.audit`, and Agentic compliance-audit settings do not participate in this gate.
3. [PLANNED: OLS-3569] No OLSConfig or AgenticOLSConfig field is added. Agentic collection reuses:
   - `spec.ols.userDataCollection.transcriptsDisabled` for the user opt-out;
   - `spec.olsDataCollector.logLevel` for the exporter log level;
   - `spec.ols.deployment.dataCollector.resources` for exporter resources;
   - `spec.ols.deployment.otelCollector` for Collector pod scheduling; and
   - the existing `--dataverse-exporter-image` selection, telemetry credentials, and `lightspeed-exporter-config` ConfigMap, including its fixed 300-second collection schedule.

### Collector Runtime Configuration

4. [PLANNED: OLS-3569] When the gate passes, the operator includes the Agentic product-collection pipeline in `lightspeed-otel-collector-config`. This pipeline consumes traces only; it does not consume OTLP logs.
5. [PLANNED: OLS-3569] When the gate fails, the operator omits only the Agentic product-collection pipeline. The Collector Deployment, OTLP receiver, templog/PostgreSQL logs pipeline, admin endpoint, metrics, and optional trace-forwarding pipeline retain their existing configuration.

### Collector Deployment

6. [PLANNED: OLS-3569] When the gate passes, the Collector Deployment includes one Agentic collection `emptyDir` shared only by the Collector container and a separate `lightspeed-to-dataverse-exporter` sidecar. The Collector mounts the volume root at `/var/lib/lightspeed-data-collection`; the exporter mounts it at `/app-root/ols-user-data`.
7. [PLANNED: OLS-3569] The separate exporter sidecar uses `GetDataverseExporterImage()`, the existing telemetry credentials, `lightspeed-exporter-config`, `spec.olsDataCollector.logLevel`, and `spec.ols.deployment.dataCollector.resources`. It shares the Collector pod scheduling configured by `spec.ols.deployment.otelCollector`.
8. [PLANNED: OLS-3569] When the gate fails, the Collector Deployment omits the Agentic exporter sidecar, shared `emptyDir`, and both Agentic volume mounts.
9. [PLANNED: OLS-3569] The Collector runtime ConfigMap and conditional Deployment resources are reconciled from the same gate state. A gate transition updates the desired pod template and uses the normal Collector rollout path; replacement ends the old pod-local `emptyDir` lifecycle.
10. [PLANNED: OLS-3569] The existing app-server `lightspeed-to-dataverse-exporter` instance, its volumes, gate, and configuration remain unchanged.

### Agentic Handoff

11. [PLANNED: OLS-3569] Agentic collection does not change `lightspeed-agentic-configuration`, its existing `otel-collector-endpoint` key, or the Collector Service. No collection state, credential state, or spool path is handed to agentic-operator or sandbox pods.
12. [PLANNED: OLS-3569] The operator keeps the handoff and OTLP receiver resources present regardless of the Agentic collection gate. Missing collection credentials must not remove or change those resources or any existing Collector pipeline.

## Cross-References

- Parent `../../../../.ai/spec/what/agentic-data-collection.md` — canonical product semantics and cross-repository ownership
- Parent `../../../../.ai/spec/decisions/0042-agentic-data-collection-via-otel.md` — accepted architecture
- `what/crd-api.md` — reused OLSConfig fields; no new API
- `what/templog.md` — Collector pipelines and trace-only separation
- `what/observability.md` — separate exporter instances
- `what/agentic-sandbox-profile.md` — unchanged Agentic handoff
- `how/deployment-generation.md` — conditional Collector resource generation

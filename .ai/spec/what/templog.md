# Temporary Audit Log Storage — Operator

Implementation details for the lightspeed-operator's role in the templog / OTEL Collector epic ([OLS-3505](https://redhat.atlassian.net/browse/OLS-3505)). See parent spec `what/templog.md` (lightspeed-service repo) for product requirements.

## Architecture

The OTEL Collector is the in-cluster telemetry hub. It is **always deployed** when Lightspeed is installed. Configuration is split between **service behavior** (stdout audit events, trace export to collector) and **collector behavior** (Postgres storage, trace forwarding).

```
lightspeed-service
  ├─ stdout JSON audit events     ← spec.ols.auditEventsEnabled
  └─ OTLP traces (gRPC :4317)   ← always → lightspeed-otel-collector Service

OTEL Collector (always deployed)
  ├─ logs pipeline → Postgres     ← spec.audit.logging (*bool, default true)
  └─ traces pipeline → backend    ← spec.audit.tracingEndpoint (optional)
```

## CRD — OLSConfig ([OLS-3509](https://redhat.atlassian.net/browse/OLS-3509))

### `spec.audit` (collector-only)

Replaces the previous service-oriented `audit.logging` enum and `audit.otel` block.

| Field | Type | Default | Purpose |
|-------|------|---------|---------|
| `logging` | `*bool` | `true` when absent | Enable Collector logs → Postgres pipeline |
| `tracingEndpoint` | `string` | empty | OTLP trace export backend (e.g. `jaeger:4317`); TLS always used |

- Value type (`Audit AuditConfig`), not pointer.
- No helper methods on `AuditConfig`.
- Does **not** configure lightspeed-service `olsconfig.yaml`.

### `spec.ols.auditEventsEnabled` (service stdout audit)

| Field | Type | Default | Purpose |
|-------|------|---------|---------|
| `auditEventsEnabled` | `*bool` | `true` when absent | Structured compliance audit JSON on stdout |

Maps to `audit.logging: Enabled|Disabled` in generated `olsconfig.yaml`.

### `spec.ols.deployment.otelCollector`

Standard `Config` (replicas, resources, tolerations, nodeSelector) for Collector pod overrides.

### Removed from CRD

- `AuditLoggingMode`, `AuditOTELConfig`, `AuditOTELTLSMode`
- `AuditConfig.LoggingEnabled()`, `OTELEndpoint()`, `OTELInsecure()`
- `spec.templog` (collector always deployed; Postgres pipeline toggled via `spec.audit.logging`)

## App server — olsconfig.yaml ([OLS-3509](https://redhat.atlassian.net/browse/OLS-3509))

The operator generates service audit config independently of `spec.audit`:

| olsconfig.yaml | Source |
|----------------|--------|
| `audit.logging` | `spec.ols.auditEventsEnabled` (default Enabled) |
| `audit.otel.endpoint` | Always `lightspeed-otel-collector.<ns>.svc:4317` |
| `audit.otel.tls_mode` | Always `Secure` (OTLP/gRPC with TLS) |

Service continues to use the existing gRPC OTLP trace exporter (`opentelemetry.exporter.otlp.proto.grpc`).

## Operator image flag ([OLS-3509](https://redhat.atlassian.net/browse/OLS-3509))

- CLI: `--otel-collector-image`
- Default: `lightspeed-otel-collector` from `related_images.json` (Konflux fallback when absent)
- Reconciler: `GetOtelCollectorImage()` — consumed by collector operand ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510))
- Bundle PR adds `related_images.json` entry with `operator_arg: otel-collector-image`

## PostgreSQL Bootstrap ([OLS-3511](https://redhat.atlassian.net/browse/OLS-3511))

1. Postgres bootstrap **always** creates the `templogs` schema alongside `quota` and `conversation_cache`.
2. Creates `templogs.logs` table and `idx_logs_trace_id` index (see parent spec for DDL).
3. Schema is never dropped by the operator.

## Collector Operand ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510), [OLS-3513](https://redhat.atlassian.net/browse/OLS-3513))

1. **Always** deploy a single-replica Collector Deployment (`lightspeed-otel-collector` Service, port 4317 OTLP gRPC).
2. Image from `GetOtelCollectorImage()`; pod scheduling from `spec.ols.deployment.otelCollector`.
3. ConfigMap pipelines driven by `spec.audit`:
   - `logging` true/absent → logs pipeline with `postgresexporter`
   - `logging` false → no Postgres export pipeline
   - `tracingEndpoint` set → traces pipeline to backend (TLS)
4. Postgres DSN uses existing operator-managed Postgres credentials; TLS via service-ca.
5. NetworkPolicy: ingress on 4317 from app-server, agentic-operator, and sandbox pods (exact rules in OLS-3510).

## Agentic Pod Wiring ([OLS-3512](https://redhat.atlassian.net/browse/OLS-3512))

Separate story — not part of OLS-3509. Sets OTLP log endpoint env on agentic-operator and sandbox pods when enabled.

## Configuration Surface

| Field path | Consumer | Description |
|------------|----------|-------------|
| `spec.audit.logging` | Collector | Postgres audit log storage pipeline. Default: true. |
| `spec.audit.tracingEndpoint` | Collector | External trace export (TLS). Optional. |
| `spec.ols.auditEventsEnabled` | lightspeed-service | Stdout audit JSON events. Default: true. |
| `spec.ols.deployment.otelCollector` | Collector | Pod resources / scheduling overrides. |

## Constraints

1. Collector is always a single replica.
2. Collector container image is operator-managed via `--otel-collector-image` (not user-supplied in CR).
3. `templogs` schema is never dropped by the operator.
4. Bundle PR (after OLS-3509) adds collector image to `related_images.json`; collector reconciler PR does not add new operator flags.

## Cross-References

- Epic: [OLS-3505](https://redhat.atlassian.net/browse/OLS-3505)
- `what/postgres.md` — PostgreSQL deployment, bootstrap script
- Parent spec: `what/templog.md` (lightspeed-service / ols repo)

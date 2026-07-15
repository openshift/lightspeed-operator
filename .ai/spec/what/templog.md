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
  │     (only service.name=lightspeed-agentic-sandbox)
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

The operator mounts the collector serving cert into the app-server at `/etc/certs/otel-collector-ca/tls.crt` and adds it to `extra_ca` in `olsconfig.yaml`. See `tls.md`.

Service continues to use the existing gRPC OTLP trace exporter (`opentelemetry.exporter.otlp.proto.grpc`).

## Operator image flag ([OLS-3509](https://redhat.atlassian.net/browse/OLS-3509))

- CLI: `--otel-collector-image`
- Default: `lightspeed-otel-collector` from `related_images.json` (Konflux fallback when absent)
- Reconciler: `GetOtelCollectorImage()` — consumed by collector operand ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510))
- `related_images.json` entry uses `operator_arg: otel-collector-image`

## PostgreSQL — templogs schema

The Postgres bootstrap script creates only `quota` and `conversation_cache` schemas. It does **not** create the `templogs` schema or tables.

When `spec.audit.logging` is true (default), the OTEL Collector creates and manages the `templogs` schema, `logs` table, and indexes via the `postgres_admin` extension at collector startup. The operator never drops this schema.

See `postgres.md` for Postgres bootstrap scope and `templog.md` (lightspeed-service repo) for table DDL semantics.

## Collector Operand ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510), [OLS-3513](https://redhat.atlassian.net/browse/OLS-3513))

1. **Always** deploy a single-replica Collector Deployment. Service exposes OTLP gRPC `:4317`, OTLP HTTP `:4318`, health check `:13133`, and (when logging enabled) `postgres_admin` HTTPS `:8080`.
2. Image from `GetOtelCollectorImage()`; pod scheduling from `spec.ols.deployment.otelCollector`.
3. ConfigMap pipelines driven by `spec.audit`:
   - `logging` true/absent → logs pipeline with `routing/logs` connector and `postgresexporter`; only OTLP logs where `service.name == "lightspeed-agentic-sandbox"` are stored in Postgres
   - `logging` false → no Postgres export pipeline, no `postgres_admin` extension
   - `tracingEndpoint` set → traces pipeline to backend (TLS); when unset, traces are received on `:4317` but not exported
4. Postgres DSN uses operator-managed Postgres credentials (`sslmode=require`, service-ca TLS).
5. NetworkPolicy: ingress on `:4317` from all pods in the operator namespace (empty `PodSelector`).
6. Serving cert via service-ca (`lightspeed-otel-collector-cert`); cert rotation restarts collector and app-server deployments.
7. Phase 1: ConfigMap, ServiceAccount, NetworkPolicy. Phase 2: Service, TLS secret wait, Deployment. Status condition: `OtelCollectorReady`.

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
3. `templogs` schema is created by the OTEL Collector, not Postgres bootstrap; the operator never drops it.
4. Only sandbox audit logs (`service.name=lightspeed-agentic-sandbox`) are routed to Postgres.

## Cross-References

- Epic: [OLS-3505](https://redhat.atlassian.net/browse/OLS-3505)
- `what/postgres.md` — PostgreSQL deployment, bootstrap script
- `what/reconciliation.md` — Phase 1/2 collector wiring, `OtelCollectorReady`
- `what/tls.md` — collector serving cert, app-server `extra_ca`
- Parent spec: `what/templog.md` (lightspeed-service / ols repo)

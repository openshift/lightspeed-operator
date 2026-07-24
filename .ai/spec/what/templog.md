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
  ├─ postgres_admin HTTPS :8080   ← always (templog cleanup / GET for agentic-operator)
  ├─ HTTPS metrics :8888          ← always (https_metrics; Prometheus scrape)
  └─ traces pipeline → backend    ← spec.audit.tracingEndpoint (optional)

Agentic handoff — see agentic-sandbox-profile.md
  appserver: Secret lightspeed-agentic-otel-ca
  agenticintegration (Phase 2 last): ConfigMap lightspeed-agentic-configuration
  → agentic-operator: OTLP/admin endpoints, OTEL CA Secret name + PEM
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

The operator mounts the OpenShift service-ca bundle into the app-server at `/etc/certs/otel-collector-ca/service-ca.crt`, adds it to `extra_ca` in `olsconfig.yaml`, and sets `OTEL_EXPORTER_OTLP_CERTIFICATE` to that path (required for OTLP/gRPC; `extra_ca` alone is not used by the exporter). See `tls.md`.

Service continues to use the existing gRPC OTLP trace exporter (`opentelemetry.exporter.otlp.proto.grpc`).

## Operator image flag ([OLS-3509](https://redhat.atlassian.net/browse/OLS-3509))

- CLI: `--otel-collector-image`
- Default: `lightspeed-otel-collector` from `related_images.json` (Konflux fallback when absent)
- Reconciler: `GetOtelCollectorImage()` — consumed by collector operand ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510))
- `related_images.json` entry uses `operator_arg: otel-collector-image`

## PostgreSQL — templogs schema

The Postgres bootstrap script creates only `quota` and `conversation_cache` schemas. It does **not** create the `templogs` schema or tables.

The OTEL Collector always creates and manages the `templogs` schema, `logs` table, and indexes via the `postgres_admin` extension at collector startup (`postgres_admin` is always enabled for clients). `spec.audit.logging` only controls whether new OTLP logs are exported into that schema. The operator never drops this schema. The `logs` table uses `agentic_run_id` (AgenticRun UID, normalized to 32-char hex) and `phase` (audit phase name) as the primary query dimensions, with a composite index on `(agentic_run_id, phase)`.

See `postgres.md` for Postgres bootstrap scope and `templog.md` (lightspeed-service repo) for table DDL semantics.

## Collector Operand ([OLS-3510](https://redhat.atlassian.net/browse/OLS-3510), [OLS-3513](https://redhat.atlassian.net/browse/OLS-3513), [OLS-3656](https://redhat.atlassian.net/browse/OLS-3656))

1. **Always** deploy a single-replica Collector Deployment. Service exposes OTLP gRPC `:4317`, OTLP HTTP `:4318`, `postgres_admin` HTTPS `:8080`, and HTTPS Prometheus metrics `:8888`. Health check listens on `:13133` (pod-local; not on the Service).
2. Image from `GetOtelCollectorImage()`; pod scheduling from `spec.ols.deployment.otelCollector`.
3. ConfigMap pipelines driven by `spec.audit`:
   - `logging` true/absent → logs pipeline with `routing/logs` connector and `postgresexporter`; only OTLP logs where `service.name == "lightspeed-agentic-sandbox"` are stored in Postgres; unmatched logs go to `logs/unmatched` → `nop`
   - `logging` false → logs pipeline exports to `nop` (no Postgres); `postgres_admin` extension remains enabled (agentic templog cleanup)
   - `tracingEndpoint` set → traces pipeline to backend (TLS); when unset, traces pipeline exports to `nop` so the OTLP receiver does not return `UNIMPLEMENTED`
   - `nop` exporter is always present for pipelines that must exist but have no backend
4. Postgres DSN uses operator-managed Postgres credentials (`sslmode=require`, service-ca TLS), always injected into the Deployment (DSN Secret env, admin container port, Postgres wait init) because `postgres_admin` is always enabled for clients. `spec.audit.logging` only toggles the logs export pipeline in the runtime ConfigMap.
5. NetworkPolicy: (a) ingress from all pods in the operator namespace (empty `PodSelector`) on `:4317` **and** `:8080`; (b) ingress from Prometheus pods in `openshift-monitoring` on HTTPS metrics `:8888` only.
6. Serving cert via service-ca (`lightspeed-otel-collector-cert`); used for OTLP, `postgres_admin`, and `https_metrics`. Cert rotation restarts collector and app-server deployments; `RestartAppServer` refreshes `lightspeed-agentic-otel-ca` and touches the handoff ConfigMap.
7. Phase 1: runtime ConfigMap (`lightspeed-otel-collector-config` / `config.yaml`), ServiceAccount, Postgres DSN Secret, NetworkPolicy. Phase 2: Service, TLS secret wait, Deployment, ServiceMonitor (`lightspeed-otel-collector-monitor`, when Prometheus Operator CRDs are available). Status condition: `OtelCollectorReady`. Collector connectivity for agentic-operator is **not** published here — see `agentic-sandbox-profile.md` (`lightspeed-agentic-configuration` + appserver-owned `lightspeed-agentic-otel-ca`).
8. [OLS-3656] Runtime ConfigMap always enables collector internal metrics: stock Prometheus pull on `127.0.0.1:18888` (not cluster-reachable) and the `https_metrics` extension reverse-proxying that endpoint on `0.0.0.0:8888` with the serving cert. Cluster Prometheus scrapes HTTPS `:8888` `/metrics` via ServiceMonitor (server TLS verify with service-ca; no client mTLS or Bearer).

## Agentic OTEL Connectivity ([OLS-3684](https://redhat.atlassian.net/browse/OLS-3684))

Agentic-operator reads OTLP/admin endpoints from `lightspeed-agentic-configuration` and the OTEL CA from appserver-owned Secret `lightspeed-agentic-otel-ca`, not from a collector-owned client ConfigMap. See `agentic-sandbox-profile.md`. Agentic consumption/wiring: OLS-3685+.

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
- `what/agentic-sandbox-profile.md` — agentic handoff ConfigMap + appserver-owned OTEL CA Secret
- Parent spec: `what/templog.md` (lightspeed-service / ols repo)

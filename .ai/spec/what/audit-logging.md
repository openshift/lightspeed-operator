# Audit Logging

Implementation spec for compliance audit logging in the lightspeed-operator. Parent spec: `ols/.ai/spec/what/audit-logging.md` (authoritative for cross-repo requirements, event semantics, and correlation contract).

Collector / Postgres storage and OTEL hub behavior: see `what/templog.md`.

## Architecture

Audit configuration is split between **service** (stdout JSON events, trace export to in-cluster collector) and **collector** (Postgres storage, optional external trace forwarding). The operator generates `olsconfig.yaml` for lightspeed-service from service fields only; `spec.audit` is collector-only.

```
spec.ols.auditEventsEnabled  →  olsconfig.yaml audit.logging (Enabled|Disabled)
                               →  stdout compliance JSON

always                         →  olsconfig.yaml audit.otel.endpoint
                               →  lightspeed-otel-collector.<ns>.svc:4317 (TLS)

spec.audit.logging             →  collector Postgres pipeline (OLS-3510+)
spec.audit.tracingEndpoint     →  collector external trace export (OLS-3510+)
```

## Behavioral Rules

### CRD — service stdout audit

1. `OLSConfig` exposes **`spec.ols.auditEventsEnabled`** (`*bool`, optional). Default: **`true`** when absent.
2. When `true` (or absent), structured compliance audit JSON is emitted on stdout by lightspeed-service.
3. When `false`, stdout audit is disabled.
4. This field does **not** control collector Postgres storage — see `spec.audit.logging` in `templog.md`.

Example:

```yaml
spec:
  ols:
    auditEventsEnabled: false   # disable stdout audit JSON
```

### CRD — collector audit (not service config)

5. **`spec.audit`** configures the OTEL Collector operand only (`logging`, `tracingEndpoint`). It is **not** propagated into `olsconfig.yaml`. See `templog.md` and `crd-api.md`.

### Config generation (olsconfig.yaml)

6. The operator MUST generate service audit config in `olsconfig.yaml` under `ols_config.audit`:

| olsconfig.yaml key | Source | Default |
|---|---|---|
| `audit.logging` | `spec.ols.auditEventsEnabled` | `Enabled` |
| `audit.otel.endpoint` | operator constant | `lightspeed-otel-collector.<namespace>.svc:4317` |
| `audit.otel.tls_mode` | operator constant | `Secure` |

7. Trace export to the in-cluster collector is **always** configured (gRPC OTLP). External backends are reached via the collector (`spec.audit.tracingEndpoint`), not by setting a service endpoint on the CR.
8. `spec.audit` MUST NOT affect generated `olsconfig.yaml` audit settings.
9. Changes to `spec.ols.auditEventsEnabled` MUST trigger reconciliation that regenerates `olsconfig.yaml` and rolls the app-server deployment.
10. The operator mounts the collector serving cert (`lightspeed-otel-collector-cert`) at `/etc/certs/otel-collector-ca/tls.crt` in the app-server and adds it to `extra_ca`. See `tls.md`.

### Reconciliation

11. The operator does not emit its own audit events. Its responsibilities are CRD schema, `olsconfig.yaml` generation for stdout audit and in-cluster trace export, and OTEL Collector operand reconciliation (`OtelCollectorReady`). See `reconciliation.md` and `templog.md`.

## Migration (breaking change)

The previous `spec.audit.logging` (`Enabled`/`Disabled`) and `spec.audit.otel` block configured **service** behavior. That shape was removed in OLS-3509.

| Previous | New |
|---|---|
| `spec.audit.logging: Enabled/Disabled` | `spec.ols.auditEventsEnabled: true/false` |
| `spec.audit.otel.endpoint` | removed — service always exports to in-cluster collector |
| `spec.audit.otel.tlsMode: Insecure` | removed — service always uses `Secure` to collector |
| (none) | `spec.audit.tracingEndpoint` — external trace export via collector |

Existing CRs with the old `spec.audit` shape must be rewritten manually before upgrade. There is no conversion webhook.

## Cross-References

- `templog.md` — OTEL Collector, Postgres templogs, `spec.audit` collector fields
- `crd-api.md` — OLSConfig CRD field reference
- `reconciliation.md` — reconciliation loop where config generation happens

# Audit Logging

Implementation spec for compliance audit logging in the lightspeed-operator. Parent spec: `ols/.ai/spec/what/audit-logging.md` (authoritative for cross-repo requirements, event semantics, and correlation contract).

Collector / Postgres storage and OTEL hub behavior: see `what/templog.md`.

## Architecture

Audit configuration is split between **service** (stdout JSON events) and **collector** (Postgres storage, optional external trace forwarding). The operator generates `olsconfig.yaml` for lightspeed-service from service fields only; `spec.audit` is collector-only. Service trace export to the in-cluster collector is currently **disabled** (OLS-3737).

```yaml
spec.ols.auditEventsEnabled  →  olsconfig.yaml audit.logging (Enabled|Disabled)
                               →  stdout compliance JSON

(disabled — OLS-3737)          →  olsconfig.yaml audit.otel.endpoint
                               →  NOT injected until e2e coverage exists

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

7. **OLS-3737**: OTEL endpoint injection (`audit.otel.endpoint`, `audit.otel.tls_mode`) is **disabled** until e2e tests prove the collector pipeline works end-to-end. The service falls back to a no-op tracer when the otel section is absent. Re-enablement is tracked in OLS-3737 Phase 3.
8. `spec.audit` MUST NOT affect generated `olsconfig.yaml` audit settings.
9. Changes to `spec.ols.auditEventsEnabled` MUST trigger reconciliation that regenerates `olsconfig.yaml` and rolls the app-server deployment.
10. The operator mounts the OpenShift service-ca bundle (`openshift-service-ca.crt`) at `/etc/certs/otel-collector-ca/service-ca.crt` in the app-server, adds it to `extra_ca`, and sets `OTEL_EXPORTER_OTLP_CERTIFICATE` to that path. These mounts are **retained** even while OTEL export is disabled (OLS-3737) to simplify Phase 3 re-enablement. See `tls.md`.

### Reconciliation

11. The operator does not emit its own audit events. Its responsibilities are CRD schema and `olsconfig.yaml` generation for stdout audit config. OTEL Collector operand reconciliation (`OtelCollectorReady`) and in-cluster trace export are currently **disabled** (OLS-3737); see `reconciliation.md` and `templog.md`.

## Migration (breaking change)

The previous `spec.audit.logging` (`Enabled`/`Disabled`) and `spec.audit.otel` block configured **service** behavior. That shape was removed in OLS-3509.

| Previous | New |
|---|---|
| `spec.audit.logging: Enabled/Disabled` | `spec.ols.auditEventsEnabled: true/false` |
| `spec.audit.otel.endpoint` | removed — operator-injected endpoint (currently disabled, OLS-3737) |
| `spec.audit.otel.tlsMode: Insecure` | removed — TLS mode `Secure` when re-enabled (OLS-3737 Phase 3) |
| (none) | `spec.audit.tracingEndpoint` — external trace export via collector |

Existing CRs with the old `spec.audit` shape must be rewritten manually before upgrade. There is no conversion webhook.

## Cross-References

- `templog.md` — OTEL Collector, Postgres templogs, `spec.audit` collector fields
- `crd-api.md` — OLSConfig CRD field reference
- `reconciliation.md` — reconciliation loop where config generation happens

# Audit Logging

Implementation spec for compliance audit logging support in the lightspeed-operator. Parent spec: `ols/.ai/spec/what/audit-logging.md` (authoritative for cross-repo requirements, event semantics, and correlation contract).

## Behavioral Rules

### CRD Changes

1. The `OLSConfig` CRD MUST add `spec.audit` with the following structure:

```yaml
spec:
  audit:                    # optional block; omitting = logging enabled, no OTEL export
    logging: Enabled        # Enabled (default) | Disabled; structured JSON audit events to stdout
    otel:
      endpoint: ""          # optional OTLP endpoint; no-op exporter when empty/absent
      tlsMode: Secure       # Secure (default) | Insecure
```

2. `spec.audit.logging` controls structured JSON audit events to stdout. Defaults to `Enabled` — when the CR is absent or the field is not set, audit logging is enabled. Set to `Disabled` to disable structured audit log output.

3. `spec.audit.otel.endpoint` controls OTEL trace export. When set, the operator configures the service with an OTLP endpoint. When empty or absent, no OTEL traces are exported. Independent of the `logging` field — tracing works regardless of whether logging is on or off.

### Config Generation

4. The operator MUST propagate `spec.audit` from the `OLSConfig` CR into the generated `olsconfig.yaml` ConfigMap so lightspeed-service can read the audit configuration.

5. The generated `olsconfig.yaml` MUST include the audit section under `ols_config` with `logging` and `otel` values, preserving the defaults (logging=Enabled when absent).

6. Changes to `spec.audit` MUST trigger a reconciliation that regenerates `olsconfig.yaml` and restarts the service deployment to pick up the new config.

### Reconciliation

7. The operator does not emit its own audit events (it manages classic OLS, not the agentic workflow). Its sole audit responsibility is CRD schema and config propagation.

## Cross-References

- `crd-api.md` — OLSConfig CRD definition (needs `spec.audit` addition)
- `reconciliation.md` — reconciliation loop where config generation happens

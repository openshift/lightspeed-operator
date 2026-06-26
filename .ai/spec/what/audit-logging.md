# Audit Logging

Implementation spec for compliance audit logging support in the lightspeed-operator. Parent spec: `ols/.ai/spec/what/audit-logging.md` (authoritative for cross-repo requirements, event semantics, and correlation contract).

## Behavioral Rules

### CRD Changes

1. The `OLSConfig` CRD MUST add `spec.audit` with the following structure:

```yaml
spec:
  audit:                    # optional block; omitting = audit enabled, no OTEL export
    enabled: true           # default: true (audit on even if spec.audit is absent)
    otel:
      endpoint: ""          # optional OTLP endpoint; no-op exporter when empty/absent
```

2. `spec.audit.enabled` defaults to `true`. When `spec.audit` is absent entirely, behavior is `enabled: true`.

3. `spec.audit.otel.endpoint` is optional. When empty or absent, OTEL export is disabled (no-op exporter). Structured JSON to stdout always emits when audit is enabled.

### Config Generation

4. The operator MUST propagate `spec.audit` from the `OLSConfig` CR into the generated `olsconfig.yaml` ConfigMap so lightspeed-service can read the audit configuration.

5. The generated `olsconfig.yaml` MUST include the audit section with `enabled` and `otel.endpoint` values, preserving the defaults (enabled=true when absent).

6. Changes to `spec.audit` MUST trigger a reconciliation that regenerates `olsconfig.yaml` and restarts the service deployment to pick up the new config.

### Reconciliation

7. The operator does not emit its own audit events (it manages classic OLS, not the agentic workflow). Its sole audit responsibility is CRD schema and config propagation.

## Cross-References

- `crd-api.md` — OLSConfig CRD definition (needs `spec.audit` addition)
- `reconciliation.md` — reconciliation loop where config generation happens

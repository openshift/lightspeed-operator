# Temporary Audit Log Storage — Operator

Implementation details for the lightspeed-operator's role in the templog feature. See parent spec `what/templog.md` for requirements and architecture.

## Behavioral Rules

### CRD

1. `AgenticOLSConfig.spec.templog` is a boolean field. Default: `true`.
2. The operator reads `spec.templog` during reconciliation to decide whether to deploy the Collector.

### PostgreSQL Bootstrap

3. When `spec.templog` is `true` (or absent), the Postgres bootstrap script creates the `templogs` schema alongside the existing `quota` and `conversation_cache` schemas.
4. The bootstrap script creates the `templogs.logs` table and `idx_logs_trace_id` index (see parent spec for DDL).
5. When `spec.templog` is `false`, the bootstrap script does not create the `templogs` schema. If it already exists, it is left in place — no destructive cleanup.

### Collector Deployment

6. When `spec.templog` is `true` (or absent), the operator deploys a single-replica Deployment for the custom OTel Collector using the `lightspeed-otel-postgres-collector` container image.
7. The Collector Deployment follows the same management patterns as PostgreSQL: operator-managed image reference, resource requirements, tolerations, node selectors.
8. The operator creates a Service exposing port 4317 (OTLP gRPC) for the Collector.
9. The operator creates a ConfigMap containing the Collector configuration (YAML). The configuration specifies the OTLP receiver, the `postgresexporter` with the Postgres DSN, and the logs pipeline wiring them together.
10. The Postgres DSN in the Collector configuration uses the same credentials secret the operator already manages for PostgreSQL.
11. The operator creates a NetworkPolicy allowing ingress to the Collector on port 4317 from agentic-operator and sandbox pods only.
12. TLS between the Collector and PostgreSQL uses the existing service-ca certificates.

### Collector Teardown

13. When `spec.templog` is `false`, the operator removes the Collector Deployment, Service, ConfigMap, and NetworkPolicy if they exist.
14. The `templogs` schema and its data are not deleted on teardown.

### Agentic Pod Wiring

15. When `spec.templog` is `true` (or absent), the operator sets an environment variable on agentic-operator and sandbox pods with the Collector's OTLP log endpoint: `<collector-service>.<namespace>.svc:4317`.
16. When `spec.templog` is `false`, the operator removes the OTLP log endpoint environment variable from agentic-operator and sandbox pods.
17. The OTLP log endpoint environment variable is independent of `spec.audit.otel.endpoint` (which is for tracing). Both can be set simultaneously.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.templog` | Boolean. Deploy the OTel Collector for temporary audit log storage in PostgreSQL. Default: `true`. |

## Constraints

1. The Collector is always a single replica.
2. The Collector image reference is managed by the operator (not user-configurable).
3. The `templogs` schema is never dropped by the operator, even when `spec.templog` is set to `false`.

## Cross-References

- Parent spec: `what/templog.md`
- `what/postgres.md` — PostgreSQL deployment, bootstrap script
- `what/crd-api.md` — `AgenticOLSConfig` CRD fields

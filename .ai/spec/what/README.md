# Behavioral Specifications

Defines what the operator must do. Each spec contains numbered behavioral rules, configuration surface tables, constraints, and planned changes.

## Spec Index

| Spec | Description |
|---|---|
| `system-overview.md` | Operator role, component inventory, lifecycle, deployment model, integration points, and top-level constraints. Start here. |
| `crd-api.md` | OLSConfig CRD field-by-field specification. Field paths, types, defaults, validation rules, and status conditions. |
| `reconciliation.md` | Reconciliation behavioral rules: ordering, idempotency, error handling, status updates, finalizer semantics. |
| `app-server.md` | AppServer (legacy) backend behavioral rules: deployment shape, configuration generation, health checks, resource requirements. |
| `lcore.md` | LCore (new) backend behavioral rules: dual-container deployment, Llama Stack configuration, MCP integration, RAG support. |
| `postgres.md` | PostgreSQL component behavioral rules: deployment, secret management, TLS, connection parameters, PVC lifecycle. |
| `console-ui.md` | Console UI plugin behavioral rules: ConsolePlugin CR, service proxy, OCP version-based image selection, enable/disable lifecycle. |
| `tls.md` | TLS behavioral rules: service-ca integration, custom certificate support, TLS profile inheritance, CA bundle management. |
| `security.md` | Security behavioral rules: RBAC, network policies, secret handling, security contexts, pod security standards. |
| `external-resources.md` | External resource watching behavioral rules: annotation-based discovery, data comparison, deployment restart mapping. |
| `observability.md` | Observability behavioral rules: ServiceMonitor, PrometheusRule, metrics endpoints, status conditions, diagnostic info. |

## How to Use

1. Start with `system-overview.md` for orientation.
2. Read the spec for the component you are working on.
3. Cross-reference with `reconciliation.md` for ordering and error handling rules.
4. Check `tls.md` and `security.md` for cross-cutting constraints that apply to all components.

## Relationship to how/

Each `what/` spec has a corresponding implementation guide in `how/` where applicable:

| what/ | how/ |
|---|---|
| `reconciliation.md` | `how/reconciliation.md` -- implementation patterns, code locations, task registration |
| `app-server.md`, `lcore.md`, `postgres.md`, `console-ui.md` | `how/deployment-generation.md` -- how deployments/services/configmaps are generated |
| `crd-api.md` | `how/config-generation.md` -- how CRD fields map to generated configuration |
| `system-overview.md` | `how/project-structure.md` -- codebase layout, package responsibilities |

The `what/` specs are authoritative for behavior. The `how/` specs are authoritative for implementation. When they conflict, the `what/` spec wins and the `how/` spec should be updated.

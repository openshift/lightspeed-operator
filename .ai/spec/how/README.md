# Architecture Specifications

Defines how the operator is implemented. Each spec maps behavioral rules from `what/` to code locations, patterns, and structural decisions.

## Spec Index

| Spec | Description |
|---|---|
| `project-structure.md` | Codebase layout: package responsibilities, file naming conventions, import graph, key entry points. Map from concept to file path. |
| `reconciliation.md` | Reconciliation implementation: task registration pattern, error propagation, status update mechanics, watcher configuration, finalizer implementation. |
| `deployment-generation.md` | How Kubernetes resources (Deployments, Services, ConfigMaps, Secrets, PVCs) are generated: builder functions, volume/mount assembly, container spec construction, owner references. |
| `config-generation.md` | How CRD fields are transformed into operand configuration: OLS config YAML generation, Llama Stack run.yaml generation, PostgreSQL configuration, environment variable mapping. |

## When to Read

| Situation | Read |
|---|---|
| Need to find where something is implemented | `project-structure.md` |
| Debugging reconciliation ordering or error handling | `reconciliation.md` |
| Modifying a deployment, service, or volume | `deployment-generation.md` |
| Changing how CRD fields map to operand config | `config-generation.md` |
| Adding a new reconciliation task | `reconciliation.md` + `deployment-generation.md` |
| Understanding watcher behavior | `reconciliation.md` |

## Relationship to what/

The `how/` specs implement the behavioral rules defined in `what/`. Each `how/` spec references the `what/` rules it implements.

- `how/` specs describe code structure, function signatures, and file locations.
- `what/` specs describe invariants, ordering constraints, and expected behavior.
- When implementing a change, read the `what/` spec first to understand the required behavior, then read the `how/` spec to find the implementation location.
- If a `how/` spec contradicts a `what/` spec, the `what/` spec is authoritative and the implementation should be updated to match.

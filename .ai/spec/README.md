# OpenShift Lightspeed Operator -- Specifications

Machine-readable behavioral and architectural specifications for the OpenShift Lightspeed Operator.

## Structure

This specification uses a two-layer structure:

| Layer | Path | Purpose |
|---|---|---|
| **what/** | `.ai/spec/what/` | Behavioral rules. Defines what the operator must do, its invariants, and its configuration surface. Implementation-agnostic. |
| **how/** | `.ai/spec/how/` | Architecture. Defines how the codebase is organized, how reconciliation is implemented, and how resources are generated. Implementation-specific. |

The separation exists so that behavioral rules remain stable across refactors. An agent fixing a reconciliation bug reads both layers; an agent answering "what happens when X" reads only `what/`.

## Scope

These specs cover the **operator** only. The following are separate projects with their own repositories and specifications:

- **lightspeed-service** -- the Python/FastAPI backend application
- **lightspeed-console** -- the OpenShift Console plugin UI code
- **RAG content pipeline** -- the retrieval-augmented generation data pipeline
- **Jira project data** -- issue tracking lives in the service repo's Jira project (OLS)

## Audience

AI agents (Claude). Content is optimized for precision and machine consumption over human readability.

## Quick Start

| Task | Start here |
|---|---|
| Understand what the operator does | `what/system-overview.md` |
| Fix a reconciliation bug | `what/reconciliation.md` + `how/reconciliation.md` |
| Add a new managed component | `what/system-overview.md` + `how/project-structure.md` |
| Understand the CRD | `what/crd-api.md` |
| Navigate the codebase | `how/project-structure.md` |
| Understand TLS configuration | `what/tls.md` |
| Understand security constraints | `what/security.md` |
| Debug external resource watching | `what/external-resources.md` + `how/reconciliation.md` |
| Add metrics or alerts | `what/observability.md` |

## Conventions

### Planned changes

Unimplemented behavior is marked with `[PLANNED: OLS-XXXX]` where `OLS-XXXX` is the Jira ticket. These markers appear inline next to the behavioral rule they affect. A summary table of all planned changes appears at the end of each `what/` spec that contains them.

### Configuration field references

User-configurable values are referenced by their CRD field path (e.g., `spec.ols.defaultModel`). Operator startup flags are referenced by their flag name (e.g., `--use-lcore`).

### Internal constants

Behavioral rules state the rule without embedding the numeric value. For example: "the finalizer cleanup waits for owned resources to be deleted before removing the finalizer" rather than "waits for 3 minutes". The actual value lives in code and may change.

### Rule numbering

Behavioral rules are numbered sequentially within each section. Numbers are stable within a spec version but may be renumbered across major revisions.

## Project History

| Phase | Period | Operator milestones |
|---|---|---|
| Prototype | Q4 2023 | Initial operator scaffold with kubebuilder. Basic OLSConfig CRD. AppServer deployment reconciliation. |
| Early Access | Q1-Q2 2024 | PostgreSQL conversation cache. Console UI plugin integration. LLM secret management. Redis replaced by PostgreSQL. |
| Tech Preview | Q3 2024 | TLS hardening (service-ca integration, custom certs). Prometheus monitoring. Status conditions. Air-gap support (image overrides). |
| GA | Q4 2024 - Q1 2025 | Finalizer-based cleanup. ResourceVersion-based change detection. External resource watcher system. OCP version detection for console plugin image selection. |
| Post-GA | 2025-2026 | LCore/Llama Stack backend (dual-container deployment). MCP server integration. RAG support with vector database. Event-driven reconciliation (removed timer-based). Dataverse exporter. PatternFly 5/6 console image selection. |

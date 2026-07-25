# Spec Verification Report: lightspeed-operator

**Date**: 2026-07-24
**Repo**: lightspeed-operator
**Spec path**: `.ai/spec/what/`
**Files reviewed**: system-overview.md, crd-api.md, reconciliation.md, resource-lifecycle.md, security.md, tls.md, console-ui.md, app-server.md, observability.md, audit-logging.md, bundle-composition.md, postgres.md, templog.md, agentic-console-ui.md, ocpmcp.md

---

## Pass 1: Acceptance Criteria

No explicit `- [ ]` acceptance criteria checkboxes were found in any of the what/ files. The specs use numbered behavioral rules, constraints, and planned changes sections rather than checkbox-style acceptance criteria. All behavioral rules are declarative statements of expected behavior.

**Result**: N/A -- no `- [ ]` criteria to evaluate.

---

## Pass 2: Constraint Compliance

**Shared constraints from `/Users/xavi/street/github.com/AI/ols/.ai/spec/constraints.md`:**

1. **All repos use a fork-based workflow. Push to your fork, PR against `origin/main`.** -- PASS. Not a spec content concern; this is a process constraint. The specs do not contradict this.

2. **All commit messages and PR titles start with `OLS-XXXX` (Jira ticket reference).** -- PASS. Process constraint. Specs reference Jira tickets correctly with `OLS-XXXX` format throughout (e.g., OLS-3505, OLS-3509, OLS-3510, OLS-3236, OLS-3442, OLS-3572, OLS-3594, OLS-3348, OLS-3656, OLS-3221, OLS-3397, OLS-3526, OLS-3512, OLS-3513, OLS-3514, OLS-3516).

3. **Squash commits before pushing -- one logical commit per PR unless the PR explicitly tracks multiple independent changes.** -- PASS. Process constraint, not relevant to spec content.

4. **Project key is OLS on `redhat.atlassian.net`.** -- PASS. All Jira references use the OLS project key.

5. **Classic OLS CRDs use API group `ols.openshift.io/v1alpha1`.** -- PASS. `crd-api.md` rule 1: "API group: `ols.openshift.io`, version: `v1alpha1`".

6. **Agentic OLS CRDs use API group `agentic.openshift.io/v1alpha1`.** -- PASS. `system-overview.md` rule 5 and `bundle-composition.md` rule 11 both reference `agentic.openshift.io` for the agentic controller. `crd-api.md` does not cover agentic CRDs (correctly scoped to `ols.openshift.io` only).

7. **All components deploy into the `openshift-lightspeed` namespace.** -- PASS. `system-overview.md` rule 1: "Reconciled workloads are created in the openshift-lightspeed namespace." Rule 19: "The operator runs as a single-instance deployment in the openshift-lightspeed namespace (configurable)."

8. **The embedding model used to build RAG indexes must be identical to the model used to query them at runtime.** -- PASS. Not violated. RAG specs in `app-server.md` and `crd-api.md` describe BYOK RAG via image references without specifying embedding model selection, and the OKP/Solr hybrid path is fully operator-managed. No contradictory embedding model claims.

**Constraint violations: 0**

---

## Pass 3: Term Consistency

Skipped per instructions (no glossary file exists).

---

## Pass 4: Internal Reference Accuracy

### References in system-overview.md

| Reference | Target | Status |
|---|---|---|
| `bundle-composition.md` (rule 9) | `what/bundle-composition.md` | PASS -- exists, covers bundle structure, CRD ownership, image references |

### References in crd-api.md

| Reference | Target | Status |
|---|---|---|
| `templog.md` (rule 55, Audit section) | `what/templog.md` | PASS -- exists, covers collector Postgres pipeline behavior |
| `audit-logging.md` (rule 57) | `what/audit-logging.md` | PASS -- exists, covers audit configuration split |
| OLS-3505, OLS-3510 Jira links (rules 54-56) | External Jira | PASS -- valid format |
| OLS-3442 (PLANNED, rule 108) | External Jira | PASS -- valid format, marked PLANNED |
| `lightspeed-service what/llm-providers.md rule 13` (rule 108) | Cross-repo reference | PASS -- `/Users/xavi/street/github.com/AI/ols/lightspeed-service/.ai/spec/what/llm-providers.md` exists |
| OLS-3572 design spec `docs/superpowers/specs/2026-07-21-inter-operator-handoff.md` (Planned Changes) | Design doc | PASS -- exists at `/Users/xavi/street/github.com/AI/ols/docs/superpowers/specs/2026-07-21-inter-operator-handoff.md` |

### References in reconciliation.md

| Reference | Target | Status |
|---|---|---|
| `what/resource-lifecycle.md` (rule 30) | `what/resource-lifecycle.md` | PASS -- exists, covers both ownership models |
| `what/crd-api.md` (Configuration Surface) | `what/crd-api.md` | PASS -- exists |
| `what/system-overview.md` (Configuration Surface) | `what/system-overview.md` | PASS -- exists |

### References in resource-lifecycle.md

No explicit cross-references to other spec files (only general references to "CRD fields" documented inline).

### References in security.md

| Reference | Target | Status |
|---|---|---|
| `what/tls.md` (Configuration Surface) | `what/tls.md` | PASS -- exists, covers TLS-related security config |

### References in tls.md

No explicit cross-references to other what/ files. References `service-ca operator` and `OpenShift API server` as external systems.

### References in console-ui.md

| Reference | Target | Status |
|---|---|---|
| `agentic-console-ui.md` (intro) | `what/agentic-console-ui.md` | PASS -- exists |

### References in app-server.md

| Reference | Target | Status |
|---|---|---|
| `ocpmcp.md` (rule 4) | `what/ocpmcp.md` | PASS -- exists, covers standalone MCP server |
| `config-generation.md` (implicit, rule 8) | `how/config-generation.md` | PASS -- exists |
| OLS-3572, OLS-3594 (Planned Changes) | External Jira | PASS -- valid format |
| OLS-3221 (Planned Changes, rules 26-27) | External Jira | PASS -- valid format, marked PLANNED |
| OLS-3397 (rules 30-32) | External Jira | PASS -- valid format |

### References in observability.md

No explicit cross-references to other what/ files.

### References in audit-logging.md

| Reference | Target | Status |
|---|---|---|
| `ols/.ai/spec/what/audit-logging.md` (header, "Parent spec") | Workspace-level spec | PASS -- `/Users/xavi/street/github.com/AI/ols/.ai/spec/what/audit-logging.md` exists |
| `what/templog.md` (header, rules 4-5, Cross-References) | `what/templog.md` | PASS -- exists |
| `crd-api.md` (Cross-References) | `what/crd-api.md` | PASS -- exists |
| `reconciliation.md` (Cross-References) | `what/reconciliation.md` | PASS -- exists |

### References in bundle-composition.md

No cross-references to other what/ files. References to external systems (OLM, CSV) and Jira tickets.

### References in postgres.md

| Reference | Target | Status |
|---|---|---|
| `templog.md` (Cross-References) | `what/templog.md` | PASS -- exists, covers templogs schema ownership |

### References in templog.md

| Reference | Target | Status |
|---|---|---|
| `what/templog.md` (lightspeed-service repo) -- "Parent spec" header | Cross-repo reference | **ISSUE** -- The header says "See parent spec `what/templog.md` (lightspeed-service repo)" but `/Users/xavi/street/github.com/AI/ols/lightspeed-service/.ai/spec/what/templog.md` does NOT exist. However, `/Users/xavi/street/github.com/AI/ols/.ai/spec/what/templog.md` (workspace-level) DOES exist. The reference is ambiguous -- it says "lightspeed-service repo" but later says "lightspeed-service / ols repo". The workspace-level file exists, but the lightspeed-service repo-level file does not. |
| `what/postgres.md` (Cross-References) | `what/postgres.md` | PASS -- exists |
| `what/reconciliation.md` (Cross-References) | `what/reconciliation.md` | PASS -- exists |
| `what/tls.md` (Cross-References) | `what/tls.md` | PASS -- exists |
| `what/templog.md` (lightspeed-service / ols repo) (Cross-References) | Workspace-level spec | PASS -- exists at `/Users/xavi/street/github.com/AI/ols/.ai/spec/what/templog.md` |

### References in agentic-console-ui.md

| Reference | Target | Status |
|---|---|---|
| `console-ui.md` (intro) | `what/console-ui.md` | PASS -- exists |

### References in ocpmcp.md

| Reference | Target | Status |
|---|---|---|
| `app-server.md` (rule 10) | `what/app-server.md` | PASS -- exists |
| `config-generation.md` (rule 10) | `how/config-generation.md` | PASS -- exists |
| `tls.md` (rule 11) | `what/tls.md` | PASS -- exists |

### how/ file existence check (referenced from README.md)

| File | Status |
|---|---|
| `how/config-generation.md` | PASS -- exists |
| `how/deployment-generation.md` | PASS -- exists |
| `how/project-structure.md` | PASS -- exists |
| `how/reconciliation.md` | PASS -- exists |

**Reference issues: 1**

---

## Summary

| Category | Result |
|---|---|
| Acceptance criteria (Pass 1) | N/A -- no `- [ ]` criteria found in any spec |
| Constraint violations (Pass 2) | 0 |
| Reference issues (Pass 4) | 1 |

### Reference Issues

1. **templog.md** -- Header says "See parent spec `what/templog.md` (lightspeed-service repo)" but the file does not exist at `lightspeed-service/.ai/spec/what/templog.md`. The workspace-level file at `ols/.ai/spec/what/templog.md` exists. Later in the same file (Cross-References section) it says "lightspeed-service / ols repo", which is a more accurate pointer. The header should be updated to clarify the actual location.

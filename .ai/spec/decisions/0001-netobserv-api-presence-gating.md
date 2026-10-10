# 0001: Gate NetObserv on FlowCollector API Presence

- **Status:** Accepted design; implementation planned under [OLS-4391](https://redhat.atlassian.net/browse/OLS-4391).
- **Date:** 2026-10-08
- **Behavioral contract:** [OpenShift MCP server](../what/ocpmcp.md), rules 21–28 and acceptance coverage.
- **Lifecycle contract:** [Reconciliation](../what/reconciliation.md).

## Context

OLS explicitly pins the toolsets of its managed OpenShift MCP server. The currently pinned Red Hat image contains `netobserv_export_flows`, `netobserv_get_flow_metrics`, and `netobserv_list_flows`, but OLS does not enable `netobserv` in its generated TOML. Enabling the toolset does not require a running NetObserv backend at server startup; unavailable backends produce tool-call errors instead. On 2026-10-08, a network-isolated MCP initialization and `tools/list` check against `registry.redhat.io/openshift-mcp/openshift-mcp-server-rhel9@sha256:a551fd58f7b2a7505a76ba2109ae5a8f031606605490c9c68d60f91f502f6fe5` confirmed startup and all three tools with `toolsets = ["netobserv"]`, a dummy kubeconfig, and no real cluster credentials. This check establishes toolset availability, not successful backend calls.

OLS-4391 adds NetObserv to the defaults only on clusters advertising the FlowCollector API. The agreed scope is minimum API-presence detection, not installation health verification or automatic configuration. Changing the OCP MCP server is out of scope. Upstream [PR #1448](https://github.com/containers/kubernetes-mcp-server/pull/1448) concerns FlowCollector-based configuration and RBAC metadata; it is not a prerequisite for this operator-side gate and its proposed absence fallback keeps tools visible.

## Decision

The OLS operator conditionally adds `netobserv` to its existing pinned toolsets using Kubernetes API discovery: any served version in `flows.netobserv.io` advertising resource `flowcollectors` with kind `FlowCollector` establishes presence. The operator does not read FlowCollector instances or derive NetObserv endpoint settings. Existing MCP defaults are retained; no new OLSConfig fields are added.

Confirmed absence omits NetObserv. Discovery uncertainty preserves the last successful decision, including across operator restarts through the existing generated configuration when available. Without a prior decision, the operator deploys the base toolsets and retries. Discovery errors are logged and use existing error backoff without preventing base MCP resource reconciliation.

Detection follows API installation/removal while the operator is running. Prefer a targeted FlowCollector CRD watch, with relevant events refreshing discovery and establishment races retried. Implementing this watch requires verification/provisioning of operator watch permissions; it does not justify granting the MCP ServiceAccount privileges or reading FlowCollector instances. Steady-state reconciliation remains event-driven.

## Alternatives

1. **Unconditional enablement:** smallest configuration change, but exposes unusable NetObserv tools on clusters lacking the API. Rejected in favor of the requested gate.
2. **MCP-side compatibility filtering:** places detection with the tool owner, but NetObserv does not currently supply the required filter. Rejected because MCP server changes are out of scope.
3. **Read FlowCollector or verify backend readiness:** more accurate than API presence but introduces additional permissions, backend-specific checks, and configuration/lifecycle complexity. Rejected as beyond minimum scope.

## Consequences

- Changes are confined to the OLS operator; MCP-side automatic configuration and bounded-RBAC enhancements are not required for gating.
- Existing base toolsets, introspection disablement, caller-token authorization, Secret/RBAC denials, and agentic handoff behavior remain unchanged.
- Retained CRDs can produce false positives. API presence does not establish that a FlowCollector instance or healthy plugin exists.
- Endpoint defaults match a standard NetObserv installation, but network access, service-CA trust, caller permissions, and required backends remain deployment prerequisites. This feature does not configure those prerequisites.
- Configuration changes roll the MCP Deployment through existing change tracking; unchanged detection must not cause rollouts.
- OLS-2715 remains separate work for administrator-configurable toolsets. Optional agentic MCP auto-injection remains deferred under OLS-3594.

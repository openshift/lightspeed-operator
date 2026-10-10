# 0001: Delegate NetObserv API-presence Filtering to the OCP MCP server

- **Status:** Accepted design; implementation planned under [OLS-4391](https://redhat.atlassian.net/browse/OLS-4391).
- **Date:** 2026-10-09
- **Behavioral contract and acceptance coverage:** [OCP MCP server](../what/ocpmcp.md#netobserv-compatibility-filtering-in-the-ocp-mcp-server-planned-ols-4391), rules 21–28.

## Context

OLS pins the managed OCP MCP server's toolsets and already enables target compatibility filtering. NetObserv tools should be available on compatible clusters without making the operator responsible for tool-specific discovery.

## Decision

Include NetObserv in the default toolsets and delegate tool visibility to the OCP MCP server's compatibility filtering. Keep operand lifecycle management in the operator and tool-specific detection in the server that owns the tools. The behavioral contract above defines configuration, discovery policy, runtime refresh, scope, and image prerequisites.

## Alternatives

1. **Enable NetObserv without compatibility filtering:** always advertises tools on clusters lacking the API. Rejected in favor of API-aware tool visibility.
2. **Read FlowCollector or verify backend readiness:** requires backend-specific permissions and lifecycle logic. Rejected as beyond minimum API-presence filtering.

## Consequences

- Discovery and filtering evolve with the OCP MCP server rather than adding tool-specific controller state and permissions to OLS.
- Fail-open discovery favors availability over strict suppression of potentially unusable tools; filtering is not an authorization boundary.
- API presence can produce false positives and does not establish backend health. Usability still depends on deployment prerequisites.
- Release delivery depends on a compatible OCP MCP server image; OLS consumes that image rather than building or releasing the OCP MCP server.

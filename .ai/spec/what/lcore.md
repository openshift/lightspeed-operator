# LCore

LCore (Lightspeed Core) is the new agent-based backend for OpenShift Lightspeed. It uses a dual-container deployment: Llama Stack for LLM provider communication and tool runtime, plus Lightspeed Stack for the OLS API layer.

## Behavioral Rules

### Deployment Modes
1. LCore has two deployment modes selected by the `--lcore-server` flag: server mode (default, two containers) and library mode (one container with embedded Llama Stack).
2. In server mode, the llama-stack container runs the Llama Stack server on its configured port, and lightspeed-stack connects to it via localhost.
3. In library mode, a single lightspeed-stack container loads Llama Stack as a library using the config file directly.

### Server Mode Containers
4. The llama-stack container starts Llama Stack in background, polls its health endpoint until ready, warms up the embedding model and LLM with test requests, then waits.
5. The lightspeed-stack container provides the OLS API on HTTPS and connects to Llama Stack on localhost.
6. Health probes for llama-stack use HTTP GET on the health endpoint. Health probes for lightspeed-stack use exec curl to the HTTPS liveness/readiness endpoints.

### Configuration Generation
7. The operator generates two configuration files: a Llama Stack config (run.yaml) and a Lightspeed Stack config (lightspeed-stack.yaml).
8. The Llama Stack config maps CR providers to Llama Stack provider format using a type mapping (openai->remote::openai, azure_openai->remote::azure, rhoai_vllm->remote::vllm, rhelai_vllm->remote::vllm, fake_provider->remote::fake).
9. Llama Stack Generic providers (those with `spec.llm.providers[].providerType` set) are passed through directly with their config, using the providerType as-is.
10. Provider credentials are injected as environment variable references (${env.PROVIDER_API_KEY}) in the Llama Stack config, with the actual values mounted from secrets.
11. Azure OpenAI providers additionally support client_id, tenant_id, and client_secret from the same credential secret.
12. The Llama Stack config always includes an inline sentence-transformers embedding provider for tool filtering and RAG.
13. Vector databases in the Llama Stack config are derived from `spec.ols.rag` entries. Each RAG entry maps to a vector DB with the faiss provider and the sentence-transformers embedding model.

### LCore-Specific Behavior
14. The Lightspeed Stack config uses `auth_enabled: false` because authentication is handled by the service itself using Kubernetes TokenReview.
15. The Lightspeed Stack connects to PostgreSQL using the "ols_production" database with schema "lcore" for its own state and schema "conversation_cache" for conversation history.
16. Llama Stack connects to PostgreSQL using a separate database (hardcoded name, determined by Llama Stack itself, not configurable).
17. The system prompt is embedded directly in the Lightspeed Stack config, not referenced by file path.
18. MCP servers in LCore config use authorization_headers with placeholder tokens ({{KUBERNETES_TOKEN}}, {{CLIENT_TOKEN}}) that the service resolves at runtime.

### Shared Behavior with AppServer
19. The same optional sidecars are supported: data collector (when enabled) and OpenShift MCP server (when introspection enabled).
20. The same PostgreSQL wait init container runs before main containers.
21. The same network policy rules apply (Prometheus, Console, ingress controller access).
22. The same RBAC (SubjectAccessReview, TokenReview) is configured.
23. The same ServiceMonitor and PrometheusRule are created for observability.

## Configuration Surface

| Field path | Description |
|---|---|
| `spec.ols.deployment.api.replicas` | Number of LCore deployment replicas |
| `spec.ols.deployment.api.resources` | Lightspeed Stack container resources |
| `spec.ols.deployment.llamaStack.resources` | Llama Stack container resources |
| `spec.ols.deployment.dataCollector.resources` | Data collector container resources |
| `spec.ols.deployment.mcpServer.resources` | MCP server container resources |
| `spec.llm.providers[].providerType` | Llama Stack Generic provider type (e.g., "remote::anthropic") |
| `spec.llm.providers[].config` | Arbitrary Llama Stack provider config (RawExtension) |
| `spec.llm.providers[].credentialKey` | Custom secret key name for credentials |

(All AppServer configuration surface fields also apply to LCore.)

## Constraints

1. The Llama Stack database name is hardcoded by the Llama Stack project and must not be changed.
2. Llama Stack Generic mode cannot be mixed with legacy provider-specific fields (deploymentName, projectID, url, apiVersion).
3. The Lightspeed Stack always connects to Llama Stack via localhost, even in server mode (they share a pod).
4. Vector database IDs are sanitized from RAG image names if indexID is not explicitly provided.

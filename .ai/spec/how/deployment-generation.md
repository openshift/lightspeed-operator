# Deployment Generation

## Module Map

| File | Key Functions | Responsibility |
|---|---|---|
| `internal/controller/appserver/deployment.go` | `GenerateOLSDeployment()`, `updateOLSDeployment()`, `RestartAppServer()`, `dataCollectorEnabled()` | AppServer deployment spec, change detection, restart |
| `internal/controller/lcore/deployment.go` | `GenerateLCoreDeployment()`, `generateLCoreServerDeployment()`, `generateLCoreLibraryDeployment()`, `updateLCoreDeployment()`, `RestartLCore()` | LCore deployment spec (2 modes), change detection, restart |
| `internal/controller/postgres/deployment.go` | `GeneratePostgresDeployment()`, `UpdatePostgresDeployment()` | PostgreSQL deployment spec |
| `internal/controller/console/deployment.go` | `GenerateConsoleUIDeployment()` | Console UI deployment spec |

## Data Flow

### AppServer Deployment Construction
```
GenerateOLSDeployment(r, cr)
  1. Check dataCollectorEnabled (requires both user config AND telemetry pull secret)
  2. Build LLM provider credential volumes + mounts (via ForEachExternalSecret, source "llm-provider-*")
  3. Build postgres secret volume + mount
  4. Build TLS volume + mount (user-provided KeyCertSecretRef OR service-ca generated OLSCertsSecretName)
  5. Build OLS config configmap volume + mount
  6. Conditionally add data collector volumes (user-data emptyDir, exporter config CM)
  7. Add kube-root-ca.crt configmap volume + cert-bundle emptyDir volume
  8. Add user-provided CA volumes (additional-ca CM, proxy-ca CM via ForEachExternalConfigMap)
  9. Add RAG emptyDir volume (if spec.ols.rag configured)
  10. Add postgres-ca configmap volume + tmp emptyDir volume
  11. Add MCP header secret volumes (via ForEachExternalSecret, source "mcp-*")
  12. Build init containers:
      a. PostgreSQL wait init container (polls pg service)
      b. RAG init containers (one per RAG entry, copies data to shared emptyDir)
  13. Get ConfigMap ResourceVersions for tracking annotations
  14. Get proxy CA cert hash for tracking annotation
  15. Assemble Deployment:
      - Container: "lightspeed-service-api", image: r.GetAppServerImage(), port: 8443
      - Env: OLS_CONFIG_FILE path + proxy vars (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
      - Probes: HTTPS GET on /readiness, /liveness (initial: 30s, period: 30s, timeout: 30s, failure: 15)
      - Default resources: 500m CPU request, 1Gi-4Gi memory
  16. Apply pod-level config (replicas, nodeSelector, tolerations, affinity, topologySpreadConstraints)
  17. Set ImageStream triggers annotation (if RAG configured)
  18. Set owner reference to OLSConfig CR
  19. Conditionally add data collector sidecar container ("lightspeed-to-dataverse-exporter")
  20. Conditionally add OpenShift MCP server sidecar container ("openshift-mcp-server")
```

### LCore Server Mode Deployment Construction
```
generateLCoreServerDeployment(r, ctx, cr)
  1. Check dataCollectorEnabled
  2. Get ConfigMap ResourceVersions (lcore, llama-stack, mcp-server configs)
  3. Build config volumes: llama-stack-config CM, lightspeed-stack-config CM
  4. Build shared volumes: postgres-ca, llama-cache (emptyDir), TLS (custom or service-ca), OpenShift CA bundles (service-ca + root CA)
  5. Build llama-stack container volume mounts (llama config, postgres-ca, llama-cache, CA bundles, user CA, proxy CA)
  6. Build lightspeed-stack container volume mounts (lcore config, TLS, postgres-ca, MCP header secrets, data collector)
  7. Build llama-stack env vars (provider credentials as env refs + POSTGRES_PASSWORD)
  8. Build lightspeed-stack env vars (LOG_LEVEL + POSTGRES_PASSWORD)
  9. Assemble Deployment with 2 containers:
     Container 1: "llama-stack"
       - Image: r.GetLCoreImage(), PullAlways, port: 8321
       - Complex bash startup command: background start, health poll, embedding warmup, LLM warmup
       - Probes: HTTP GET on /v1/health (initial: 60s, period: 10s, timeout: 5s, failure: 3)
       - Default resources: 500m-1000m CPU, 512Mi-2Gi memory
     Container 2: "lightspeed-stack"
       - Image: r.GetLCoreImage(), PullAlways, port: 8443
       - Probes: Exec-based curl with Bearer token from serviceaccount (initial: 20s, period: 10s, timeout: 5s, failure: 3)
       - Default resources: 500m-1000m CPU, 512Mi-1Gi memory
  10. Apply pod-level config, set owner reference
  11. Conditionally add OpenShift MCP server sidecar
  12. Conditionally add data collector sidecar
```

### LCore Library Mode Deployment Construction
```
generateLCoreLibraryDeployment(r, ctx, cr)
  1. Single container deployment ("lightspeed-service-api")
  2. Image: r.GetLCoreImage(), PullIfNotPresent
  3. Both llama-stack and lcore config mounts in same container
  4. Combined env vars from both llama-stack + lightspeed-stack builders
  5. Same probes as server mode lightspeed-stack container (exec curl)
  6. Same resource defaults as lightspeed-stack (500m-1000m CPU, 512Mi-1Gi memory)
  7. LCore config has use_as_library_client: true and library_client_config_path set
```

### Change Detection Pattern
All deployments use the same pattern in their update functions:
1. Compare desired vs existing deployment spec using `DeploymentSpecEqual()` (from `utils/`)
2. Compare ConfigMap ResourceVersions via deployment annotations (one per tracked CM)
3. Compare content hashes (proxy CA cert hash) via annotations
4. If any differ: update spec + annotations, call RestartX() function
   - RestartX() sets `ols.openshift.io/force-reload` annotation to `time.Now().Format(time.RFC3339Nano)`
   - This triggers a rolling restart by changing the pod template

**AppServer tracks:** OLS config CM version, MCP server config CM version, proxy CA cert hash
**LCore tracks:** LCore config CM version, Llama Stack config CM version, MCP server config CM version, proxy CA cert hash

## Key Abstractions

### Resource Requirement Defaults
Each component defines default CPU/memory requests and limits in local `get*Resources()` functions. User-provided values from the CR override defaults via `utils.GetResourcesOrDefault()` which returns user values if non-nil, otherwise defaults.

Default resources by container:
| Container | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---|---|---|---|---|
| AppServer `lightspeed-service-api` | 500m | - | 1Gi | 4Gi |
| LCore `llama-stack` | 500m | 1000m | 512Mi | 2Gi |
| LCore `lightspeed-stack` | 500m | 1000m | 512Mi | 1Gi |
| Data collector | 50m | - | 64Mi | 200Mi |
| MCP server | 50m | - | 64Mi | 200Mi |

### Volume/Mount Construction
Volumes and mounts are built as slices and conditionally appended. LCore uses helper functions (`addTLSVolumesAndMounts()`, `addPostgresCAVolumesAndMounts()`, etc.) that accept pointer-to-slice parameters for in-place modification. AppServer uses inline append patterns.

### Init Container Generation
- **PostgreSQL wait:** `utils.GeneratePostgresWaitInitContainer()` generates a container that polls the PostgreSQL service until it responds. Used by both AppServer and LCore.
- **RAG (AppServer only):** `GenerateRAGInitContainers()` creates one init container per RAG entry, each copying data from the RAG image to the shared emptyDir volume at `/app-root/rag/rag-<index>`.

### ImageStream Triggers (AppServer only)
RAG images use OpenShift ImageStreams for automatic updates. The deployment is annotated with `image.openshift.io/triggers` JSON that maps ImageStreamTag changes to init container image fields. This allows RAG content updates without operator intervention. LCore does not use ImageStream triggers for RAG.

### Data Collector Enablement
Computed from two inputs:
1. User data collection config: `!FeedbackDisabled || !TranscriptsDisabled`
2. Telemetry pull secret: `openshift-config/pull-secret` has `.auths."cloud.openshift.com"` entry in `.dockerconfigjson`

Both must be true. The service ID is `"ols"` unless the CR has `openstack.org/lightspeed-owner-id` label, in which case it's `"rhos-lightspeed"`.

### Pod Scheduling Configuration
`utils.ApplyPodDeploymentConfig()` applies scheduling from `cr.Spec.OLSConfig.DeploymentConfig.APIContainer`:
- Replicas (configurable for API container; forced to 1 for postgres and console)
- NodeSelector
- Tolerations
- Affinity
- TopologySpreadConstraints

## Integration Points

| Consumer | Provider | Data |
|---|---|---|
| Deployment spec | `utils/constants.go` | Resource names, ports, mount paths |
| Container resources | CR `spec.ols.deployment.api.resources` | User-overridable CPU/memory |
| Pod scheduling | CR `spec.ols.deployment.api` | Tolerations, nodeSelector, affinity, topology |
| Volume secrets | Kubernetes Secrets | LLM credentials, TLS certs, PostgreSQL password, MCP header values |
| Volume configmaps | Generated ConfigMaps | OLS config, Llama Stack config, LCore config, nginx config, MCP server config |
| Proxy env vars | `utils.GetProxyEnvVars()` | HTTP_PROXY, HTTPS_PROXY, NO_PROXY from cluster |
| RAG images | CR `spec.ols.rag[].image` | Container images for init containers |

## Implementation Notes

- `RevisionHistoryLimit` is set to 1 for all deployments to minimize stored ReplicaSets.
- All sidecar containers use `utils.RestrictedContainerSecurityContext()` which sets: `RunAsNonRoot: true`, `ReadOnlyRootFilesystem: true`, `AllowPrivilegeEscalation: false`, Drop ALL capabilities, RuntimeDefault seccomp profile.
- The force-reload annotation (`ols.openshift.io/force-reload`) is set to `time.Now().Format(time.RFC3339Nano)` to guarantee uniqueness and trigger pod replacement.
- LCore's `llama-stack` container uses `PullAlways`; the `lightspeed-stack` container also uses `PullAlways` in server mode but `PullIfNotPresent` in library mode. The OpenShift MCP server always uses `PullIfNotPresent`.
- LCore's llama-stack startup script includes warmup for both the embedding model (sentence-transformers) and the safety model (Llama Guard via LLM inference) to prevent cold-start latency.
- The `VolumeDefaultMode` is `int32(420)` (0644 octal), defined in `utils/constants.go`.
- AppServer and LCore share the same `ServiceAccountName` (`utils.OLSAppServerServiceAccountName`).
- LCore deployment name is `utils.LCoreDeploymentName` ("lightspeed-stack-deployment"); AppServer is `utils.OLSAppServerDeploymentName`.

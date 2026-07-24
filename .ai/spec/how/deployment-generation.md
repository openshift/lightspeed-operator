# Deployment Generation

## Module Map

| File | Key Functions | Responsibility |
|---|---|---|
| `internal/controller/appserver/deployment.go` | `GenerateOLSDeployment()`, `updateOLSDeployment()`, `RestartAppServer()`, `dataCollectorEnabled()` | AppServer deployment spec, change detection, restart |
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
      - Env: OCP_CLUSTER_VERSION (`<major>.<minor>`) when `!byokRAGOnly` (same cluster-version source as console UI)
      - Env: OLS_ROSA_PRODUCT when `!byokRAGOnly` and startup detection finds ROSA brand. `External` topology → `red_hat_openshift_service_on_aws` (HCP); any other topology on ROSA → `red_hat_openshift_service_on_aws_classic_architecture` (Classic). Omitted on non-ROSA or detection failure.
      - Probes: HTTPS GET on /readiness, /liveness (initial: 30s, period: 30s, timeout: 30s, failure: 15)
      - Default resources: 500m CPU request, 1Gi memory request (no limits)
  16. Apply pod-level config (replicas, nodeSelector, tolerations)
  17. Set ImageStream triggers annotation (if RAG configured)
  18. Set owner reference to OLSConfig CR
  19. Conditionally add data collector sidecar container ("lightspeed-to-dataverse-exporter")
  20. Conditionally add RHOKP sidecar container ("rhokp") when `!byokRAGOnly`.
  21. When introspection is enabled, mount MCP client CA Secret `lightspeed-agentic-mcp-ca` (no MCP sidecar; standalone operand)
      Container: image from r.GetRHOOKPImage(), Solr HTTP on port 9080 (remapped from image default 8080),
      resources from `spec.ols.deployment.rhokp.resources` or defaults (2 CPU / 2 GiB memory / 75 GiB ephemeral requests; CPU and memory requests only per OLS-3397),
      startup script remaps Apache Listen directives before `mel` start.
      Writable root filesystem (Solr data).
```

### Change Detection Pattern
All deployments use the same pattern in their update functions:
1. Compare desired vs existing deployment spec using `DeploymentSpecEqual()` (from `utils/`)
2. Compare ConfigMap ResourceVersions via deployment annotations (one per tracked CM)
3. Compare content hashes (proxy CA cert hash; OpenShift MCP CA hash when introspection is enabled) via annotations
4. If any differ: update spec + annotations, call RestartX() function
   - RestartX() sets `ols.openshift.io/force-reload` annotation to `time.Now().Format(time.RFC3339Nano)`
   - This triggers a rolling restart by changing the pod template

**AppServer tracks:** OLS config CM version, MCP server config CM version, proxy CA cert hash, OpenShift MCP CA ConfigMap content hash (when introspection is enabled)

## Key Abstractions

### Resource Requirement Defaults
Each component defines default CPU/memory requests in local `get*Resources()` functions. Per [OpenShift conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits), operator defaults set requests only and do not set limits. User-provided values from the CR override defaults via `utils.GetResourcesOrDefault()` which returns user values if non-nil, otherwise defaults. Users may still set limits via the CRD if needed for their environment.

Default resources by container:
| Container | CPU Request | Memory Request | Ephemeral Storage Request |
|---|---|---|---|
| AppServer `lightspeed-service-api` | 500m | 1Gi | — |
| Data collector | 50m | 64Mi | — |
| MCP server | 50m | 64Mi | — |
| RHOKP `rhokp` | 2000m | 2Gi | 75Gi |

### Volume/Mount Construction
Volumes and mounts are built as slices and conditionally appended using inline append patterns.

### Init Container Generation
- **PostgreSQL wait:** `utils.GeneratePostgresWaitInitContainer()` generates a container that polls the PostgreSQL service until it responds.
- **RAG (AppServer only):** `GenerateRAGInitContainers()` creates one init container per RAG entry, each copying data from the RAG image to the shared emptyDir volume at `/app-root/rag/rag-<index>`.

### ImageStream Triggers (AppServer only)
RAG images use OpenShift ImageStreams for automatic updates. The deployment is annotated with `image.openshift.io/triggers` JSON that maps ImageStreamTag changes to init container image fields. This allows RAG content updates without operator intervention.

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

Affinity and topology spread constraints are not exposed on `Config` (CRD size); use cluster-level defaults or patch deployments out of band if needed.

## Integration Points

| Consumer | Provider | Data |
|---|---|---|
| Deployment spec | `utils/constants.go` | Resource names, ports, mount paths |
| Container resources | CR `spec.ols.deployment.api.resources` | User-overridable CPU/memory |
| RHOKP resources | CR `spec.ols.deployment.rhokp.resources` | User-overridable CPU/memory/ephemeral storage |
| Pod scheduling | CR `spec.ols.deployment.api` | Tolerations, nodeSelector |
| Volume secrets | Kubernetes Secrets | LLM credentials, TLS certs, PostgreSQL password, MCP header values |
| Volume configmaps | Generated ConfigMaps | OLS config, nginx config, MCP server config |
| Proxy env vars | `utils.GetProxyEnvVars()` | HTTP_PROXY, HTTPS_PROXY, NO_PROXY from cluster |
| RAG images | CR `spec.ols.rag[].image` | Container images for init containers |
| RHOKP image | `--rhokp-image` flag | RHOKP sidecar container image; default from `related_images.json` (`rhokp`) |

## Agentic Controller Deployment (OLM-managed)

Unlike the AppServer, PostgreSQL, and Console UI deployments (which are reconciled by the lightspeed-operator controller at runtime), the agentic controller deployment is statically defined in the CSV and managed by OLM. The lightspeed-operator controller has no code to generate, update, or restart the agentic controller deployment. The agentic controller's operand images (agentic console plugin, etc.) are configured via startup flags on its deployment in the CSV, not via the lightspeed-operator's flags.

## Implementation Notes

- `RevisionHistoryLimit` is set to 1 for all deployments to minimize stored ReplicaSets.
- All sidecar containers use `utils.RestrictedContainerSecurityContext()` which sets: `RunAsNonRoot: true`, `ReadOnlyRootFilesystem: true`, `AllowPrivilegeEscalation: false`, Drop ALL capabilities, RuntimeDefault seccomp profile.
- The force-reload annotation (`ols.openshift.io/force-reload`) is set to `time.Now().Format(time.RFC3339Nano)` to guarantee uniqueness and trigger pod replacement.
- The OpenShift MCP server always uses `PullIfNotPresent`.
- The `VolumeDefaultMode` is `int32(420)` (0644 octal), defined in `utils/constants.go`.
- AppServer deployment name is `utils.OLSAppServerDeploymentName` (`"lightspeed-app-server"`).

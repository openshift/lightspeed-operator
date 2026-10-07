# Deployment Generation

## Module Map

| File | Key Functions | Responsibility |
|---|---|---|
| `internal/controller/appserver/deployment.go` | `GenerateOLSDeployment()`, `updateOLSDeployment()`, `RestartAppServer()`, `dataCollectorEnabled()` | AppServer deployment spec, change detection, restart |
| `internal/controller/postgres/deployment.go` | `GeneratePostgresDeployment()`, `UpdatePostgresDeployment()` | PostgreSQL deployment spec |
| `internal/controller/console/deployment.go` | `GenerateConsoleUIDeployment()` | Console UI deployment spec |
| `internal/controller/agenticconsole/deployment.go` | `GenerateAgenticConsoleUIDeployment()` | Agentic console plugin deployment spec |
| `internal/controller/otelcollector/deployment.go` | `GenerateOtelCollectorDeployment()`, `UpdateOtelCollectorDeployment()` | OTEL Collector Deployment; conditional FileExporter storage and transcript-plus-credential-gated Dataverse sidecar |
| `internal/controller/ocpmcp/deployment.go` | `GenerateDeployment()`, `UpdateDeployment()` | Standalone OpenShift MCP deployment spec |
| `internal/controller/rhokp/deployment.go` | `GenerateDeployment()`, `UpdateDeployment()` | Standalone RHOKP deployment spec |
| `internal/controller/alertsadapter/deployment.go` | `GenerateDeployment()` | Alerts adapter deployment spec |

## Data Flow

### AppServer Deployment Construction
```
GenerateOLSDeployment(r, cr)
  1. Check the unchanged app-server Dataverse exporter gate (`dataCollectorEnabled`): at least one of feedback/transcripts enabled, plus telemetry pull-secret auth-entry presence
  2. Build LLM provider credential volumes + mounts (via ForEachExternalSecret, source "llm-provider-*")
  3. Build postgres secret volume + mount
  4. Build TLS volume + mount (user-provided KeyCertSecretRef OR service-ca generated OLSCertsSecretName)
  5. Build OLS config configmap volume + mount
  6. Conditionally add app-server Dataverse exporter volumes (user-data emptyDir, exporter ConfigMap)
  7. Add kube-root-ca.crt configmap volume + cert-bundle emptyDir volume
  8. Add user-provided CA volumes (additional-ca CM, proxy-ca CM via ForEachExternalConfigMap)
  9. Add RAG emptyDir volume (if spec.ols.rag configured)
  10. Add postgres-ca configmap volume + tmp emptyDir volume
  11. Add MCP header secret volumes (via ForEachExternalSecret, source "mcp-*")
  12. Build init containers:
      a. PostgreSQL wait init container (polls pg service)
      b. RAG init containers (one per RAG entry, copies data to shared emptyDir)
      c. [PLANNED: OLS-3799] RHOKP wait init container (when `!byokRAGOnly`) — not yet implemented; today only the PostgreSQL wait + RAG init containers are generated.
  13. Get ConfigMap ResourceVersions for tracking annotations
  14. Get proxy CA cert hash for tracking annotation
  15. Assemble Deployment:
      - Container: "lightspeed-service-api", image: r.GetAppServerImage(), port: 8443
      - Env: OLS_CONFIG_FILE path + proxy vars (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
      - Env: OCP_CLUSTER_VERSION (`<major>.<minor>`) when `!byokRAGOnly` (same cluster-version source as console UI)
      - Env: OLS_ROSA_PRODUCT when `!byokRAGOnly` and startup detection finds ROSA brand. `External` topology → `red_hat_openshift_service_on_aws` (HCP); any other topology on ROSA → `red_hat_openshift_service_on_aws_classic_architecture` (Classic). Omitted on non-ROSA or detection failure.
      - Probes: HTTPS GET on /readiness (initial: 30s, period: 30s, timeout: 30s, failure: 15) and /liveness (initial: 30s, period: 30s, timeout: 30s, failure: 3 — OLS-3221)
      - Default resources: 500m CPU request, 1Gi memory request (no limits)
  16. Apply pod-level config (replicas, nodeSelector, tolerations)
  17. Set ImageStream triggers annotation (if RAG configured)
  18. Set owner reference to OLSConfig CR
  19. Conditionally add the app-server Dataverse exporter sidecar (`lightspeed-to-dataverse-exporter`)
  20. When `!byokRAGOnly`, mount the RHOKP client CA Secret `lightspeed-agentic-rhokp-ca` at `/etc/certs/rhokp-ca/` (added to `extra_ca`). RHOKP itself runs as a standalone Deployment (`internal/controller/rhokp/`, HTTPS `:8443`), not an app-server sidecar — see `rhokp.md`.
  21. When introspection is enabled, mount MCP client CA Secret `lightspeed-agentic-mcp-ca` (no MCP sidecar; standalone operand).
```


### Collector Trace Storage and Dataverse Sidecar

When `!spec.ols.userDataCollection.transcriptsDisabled`, the Collector runtime ConfigMap includes `file/data_collection`, `routing/data_collection`, and `traces/data_collection`. The exact service-name selector and FileExporter settings are defined in [`data-collection.md`](../what/data-collection.md); the file branch is unbatched, while configured backend forwarding is batched on its separate pipeline. This branch is independent of telemetry credentials.

The `data-collection` source `emptyDir` has `sizeLimit: 500Mi` and is mounted at `/var/lib/lightspeed-data/otel` in the Collector container. FileExporter writes `/var/lib/lightspeed-data/otel/traces.jsonl` with the specified 10MiB, 40-backup, one-day rotation/retention settings. The Collector's `file_storage` queue remains on a separate volume.

The OTel Dataverse sidecar is present only when transcripts are enabled **and** a successfully read, well-formed `openshift-config/pull-secret` has a nonempty (after trimming whitespace) `.dockerconfigjson.auths["cloud.openshift.com"].auth` token. Its separate ConfigMap sets `data_mode: otel`, `data_dir: /input`, `otel_active_file: traces.jsonl`, `ledger_file: /state/ledger.json`, `archive_path_prefix: v1/`, `collection_interval: 300` seconds, and `cleanup_after_send: false`; it uses the same service-ID rule and ingress URL as the app-server exporter. `--mode openshift` selects API-based authentication, while `data_mode: otel` selects ingestion. The ConfigMap has no credentials. When credential checking runs, a NotFound pull-secret object is treated as gate-off; valid JSON with absent/empty telemetry auth is also gate-off. A missing `.dockerconfigjson` key on an existing Secret, malformed JSON, or API read errors other than NotFound are reconciliation failures, not disabled credentials, and do not clean up exporter resources. Neither gate-off case disables transcript routing or FileExporter storage.

The source `emptyDir` is mounted read/write in the Collector at `/var/lib/lightspeed-data/otel` and read-only in the exporter at `/input`. A separate writable `emptyDir` is mounted at `/state` for the ledger. The Collector Pod keeps `automountServiceAccountToken: false`; the projected token, `kube-root-ca.crt` CA, and namespace are mounted at the default in-cluster auth path in the exporter container only. The exporter receives cluster proxy environment variables and the restricted container security context. No new listener, Service port, or ingress NetworkPolicy rule is added.

The selected Collector image must provide the stock contrib FileExporter v0.159.0; the existing `--otel-collector-image` override selects it. The Dataverse sidecar reuses `r.GetDataverseExporterImage()` and the existing `--dataverse-exporter-image` override. It requires an exporter image with PR #147's OTel ingestion support. The current released default pin lacks that support, so the existing override can select a compatible test image for this rollout. Before merging or releasing the follow-up, update the released default pin to a compatible released exporter; do not pin a temporary CI image.

### Change Detection Pattern
All deployments use the same pattern in their update functions:
1. Compare desired vs existing deployment spec using `DeploymentSpecEqual()` (from `utils/`), including semantic equality of `emptyDir` medium and size limit. A size-limit-only change triggers a rollout even when tracked ConfigMap ResourceVersions are unchanged.
2. Compare ConfigMap ResourceVersions via deployment annotations (one per tracked CM)
3. Compare content hashes (proxy CA cert hash; OpenShift MCP CA hash when introspection is enabled) via annotations
4. If any differ: update spec + annotations, call RestartX() function
   - RestartX() sets `ols.openshift.io/force-reload` annotation to `time.Now().Format(time.RFC3339Nano)`
   - This triggers a rolling restart by changing the pod template

**AppServer tracks:** OLS config CM version, MCP server config CM version, proxy CA cert hash, MCP client CA Secret content hash (when introspection is enabled)
**Otelcollector tracks:** Collector runtime ConfigMap and dedicated OTel exporter ConfigMap contents/resource versions. An exporter ConfigMap change rolls the Collector Deployment. Source and ledger `emptyDir` data are pod-local and are lost on Pod replacement.

## Key Abstractions

### Resource Requirement Defaults
Each component defines default CPU/memory requests in local `get*Resources()` functions. Per [OpenShift conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits), operator defaults set requests only and do not set limits. User-provided values from the CR override defaults via `utils.GetResourcesOrDefault()` which returns user values if non-nil, otherwise defaults. Users may still set limits via the CRD if needed for their environment.

Default resources by container:
| Container | CPU Request | Memory Request | Ephemeral Storage Request |
|---|---|---|---|
| AppServer `lightspeed-service-api` | 500m | 1Gi | — |
| App-server Dataverse exporter | 50m | 64Mi | — |
| OTel Dataverse exporter | 50m | 64Mi | — |
| MCP server (standalone) | 50m | 64Mi | — |
| RHOKP `rhokp` (standalone) | 2000m | 2Gi | — (75Gi EmptyDir `sizeLimit`, not an ephemeral-storage request) |

### Volume/Mount Construction
Volumes and mounts are built as slices and conditionally appended using inline append patterns.

### Init Container Generation
- **PostgreSQL wait:** `utils.GeneratePostgresWaitInitContainer()` generates a container that polls the PostgreSQL service until it responds.
- **RHOKP wait (when `!byokRAGOnly`):** [PLANNED: OLS-3799] — not yet implemented. When added, `utils.GenerateRHOKPWaitInitContainer()` would poll the RHOKP Solr ping endpoint until it responds (~360s budget), following the PostgreSQL wait pattern. No such function exists today.
- **RAG (AppServer only):** `GenerateRAGInitContainers()` creates one init container per RAG entry, each copying data from the RAG image to the shared emptyDir volume at `/app-root/rag/rag-<index>`.

### ImageStream Triggers (AppServer only)
RAG images use OpenShift ImageStreams for automatic updates. The deployment is annotated with `image.openshift.io/triggers` JSON that maps ImageStreamTag changes to init container image fields. This allows RAG content updates without operator intervention.

### Dataverse Exporter Gates

The existing app-server Dataverse exporter gate is:
1. At least one path is enabled: `!spec.ols.userDataCollection.feedbackDisabled || !spec.ols.userDataCollection.transcriptsDisabled`.
2. `openshift-config/pull-secret` contains a `cloud.openshift.com` auth entry in `.dockerconfigjson` (the existing app-server presence check).

Both conditions must hold. The OTel Dataverse sidecar has a separate gate:
1. Transcripts are enabled: `!spec.ols.userDataCollection.transcriptsDisabled`.
2. When transcripts are enabled, the telemetry pull secret is checked: NotFound or valid JSON with absent/empty `.auth` is gate-off; a present, successfully read pull secret with valid JSON and a nonempty (after trimming whitespace) `.dockerconfigjson.auths["cloud.openshift.com"].auth` token satisfies the credential condition. A missing `.dockerconfigjson` key on an existing Secret, malformed JSON, or API read errors other than NotFound fail reconciliation instead of being treated as disabled credentials.

The Collector trace-file branch depends only on the transcript condition and does not require telemetry credentials. Both exporters use service ID `"ols"` unless the CR has `openstack.org/lightspeed-owner-id`, in which case they use `"rhos-lightspeed"`. The app-server keeps `lightspeed-exporter-config`; the Collector uses its separate exporter ConfigMap.

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
| Projected in-cluster auth | OTel Collector ServiceAccount, `kube-root-ca.crt`, namespace | Token, CA, and namespace mounted at the default in-cluster auth path in the OTel exporter container only |
| Volume secrets | Kubernetes Secrets | LLM credentials, TLS certs, PostgreSQL password, MCP header values |
| Volume configmaps | Generated ConfigMaps | OLS config, nginx config, MCP server config, app-server Dataverse exporter ConfigMap (`lightspeed-exporter-config`), and separate OTel Dataverse exporter ConfigMap; Collector runtime config includes the FileExporter trace branch. |
| Proxy env vars | `utils.GetProxyEnvVars()` | HTTP_PROXY, HTTPS_PROXY, NO_PROXY; provided to Collector and OTel Dataverse exporter |
| RAG images | CR `spec.ols.rag[].image` | Container images for init containers |
| RHOKP image | `--rhokp-image` flag | Standalone RHOKP Deployment container image; default from `related_images.json` (`rhokp`) |
| Dataverse exporter image | `GetDataverseExporterImage()` / `--dataverse-exporter-image` | Existing image getter and override used by both exporters; OTel mode requires PR #147 support |

## Agentic Controller Deployment (OLM-managed)

Unlike the AppServer, PostgreSQL, and Console UI deployments (which are reconciled by the lightspeed-operator controller at runtime), the agentic controller deployment is statically defined in the CSV and managed by OLM. The lightspeed-operator controller has no code to generate, update, or restart the agentic controller deployment. The agentic controller's operand images (agentic console plugin, etc.) are configured via startup flags on its deployment in the CSV, not via the lightspeed-operator's flags.

## Implementation Notes

- `RevisionHistoryLimit` is set to 1 for all deployments to minimize stored ReplicaSets.
- All sidecar containers use `utils.RestrictedContainerSecurityContext()` which sets: `RunAsNonRoot: true`, `ReadOnlyRootFilesystem: true`, `AllowPrivilegeEscalation: false`, Drop ALL capabilities, RuntimeDefault seccomp profile.
- The force-reload annotation (`ols.openshift.io/force-reload`) is set to `time.Now().Format(time.RFC3339Nano)` to guarantee uniqueness and trigger pod replacement.
- The OpenShift MCP server always uses `PullIfNotPresent`.
- The `VolumeDefaultMode` is `int32(420)` (0644 octal), defined in `utils/constants.go`.
- AppServer deployment name is `utils.OLSAppServerDeploymentName` (`"lightspeed-app-server"`).

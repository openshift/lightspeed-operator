# Trace Data Collection — Operator

This specification defines Collector-side capture of selected native OTLP traces and best-effort Dataverse forwarding by the lightspeed-operator.

## Behavioral Rules

1. The existing `spec.ols.userDataCollection.transcriptsDisabled` field controls trace routing and FileExporter storage: `false` or unset enables them; `true` disables them. The operator adds no CRD field. This local trace-file branch does not require telemetry credentials.
2. The `routing/data_collection` connector selects OTLP traces whose resource `service.name` is exactly `lightspeed-agentic-operator` or `lightspeed-agentic-sandbox` and routes them to `traces/data_collection`.

   ```yaml
   routing/data_collection:
     table:
       - context: resource
         condition: attributes["service.name"] == "lightspeed-agentic-operator" or attributes["service.name"] == "lightspeed-agentic-sandbox"
         pipelines: [traces/data_collection]
   ```

3. `traces/data_collection` sends selected traces directly to `file/data_collection` without a batch processor. When trace forwarding is configured, its separate pipeline applies batching independently. The file branch stores traces and their native span events. OTLP logs remain in the existing `templog` pipeline: they are exported to PostgreSQL when `spec.audit.logging` is enabled and use the `nop` exporter otherwise. OTLP logs are never ingested by the Dataverse exporter.
4. The stock FileExporter writes each selected OTLP trace batch as JSONL, retaining raw, unredacted native trace content: every resource and scope, span, and attached event in the original nested structure `resourceSpans[] → scopeSpans[] → spans[] → events[]`. Its operator-generated settings are:

   ```yaml
   file/data_collection:
     path: /var/lib/lightspeed-data/otel/traces.jsonl
     format: json
     create_directory: true
     rotation:
       max_megabytes: 10
       max_backups: 40
       max_days: 1
   ```

5. The `data-collection` source `emptyDir` has `sizeLimit: 500Mi`. The Collector mounts its root at `/var/lib/lightspeed-data/otel`, preserving the FileExporter path above. The Collector's `file_storage` queue uses a separate volume.
6. One active file plus up to forty 10MiB backups implies about 410MiB of active-plus-backup data. This is a nominal sizing estimate, not a quota; the pod-local source `emptyDir` remains limited to `500Mi`.
7. The OTel Dataverse exporter sidecar is enabled only when transcripts are enabled (`!spec.ols.userDataCollection.transcriptsDisabled`) and `openshift-config/pull-secret` is successfully read with valid `.dockerconfigjson` JSON containing a nonempty (after trimming whitespace) `.dockerconfigjson.auths["cloud.openshift.com"].auth` token. Feedback enablement does not gate this sidecar. If transcripts are disabled, the pull-secret object is NotFound, or valid JSON has no telemetry auth entry or an absent, empty, or whitespace-only `.auth` value, the sidecar and exporter-only resources—the dedicated ConfigMap, ClusterRoles `lightspeed-otel-dataverse-exporter` and `lightspeed-otel-dataverse-exporter-pull-secret`, ClusterRoleBinding `lightspeed-otel-dataverse-exporter-binding`, and RoleBinding `lightspeed-otel-dataverse-exporter-pull-secret` in `openshift-config`—are omitted or removed. Transcript-gated trace routing and FileExporter storage remain credential-independent. When credential checking runs (with transcripts enabled), a failed API read other than NotFound, a missing `.dockerconfigjson` key on an existing Secret, or malformed JSON is a reconciliation error, not a disabled-auth result, and does not trigger exporter-resource cleanup. The existing app-server auth-entry-presence gate and its invalid-pull-secret Phase 1 error behavior remain unchanged.
8. The sidecar reads a dedicated OTel exporter ConfigMap, separate from the Collector runtime ConfigMap and the existing app-server exporter ConfigMap. Its generated settings include:

   ```yaml
   data_mode: otel
   data_dir: /input
   otel_active_file: traces.jsonl
   ledger_file: /state/ledger.json
   archive_path_prefix: v1/
   collection_interval: 300
   cleanup_after_send: false
   service_id: ols # generated as rhos-lightspeed when the CR has openstack.org/lightspeed-owner-id
   ingress_server_url: https://console.redhat.com/api/ingress/v1/upload
   ```

   The sidecar runs with `--mode openshift` for Kubernetes authentication; `data_mode: otel` selects the ingestion format. It obtains the telemetry token and cluster identity through the OpenShift API, not from the ConfigMap. Its service-ID selection and ingress URL match the existing app-server exporter. ConfigMap content changes roll the OTel Collector Deployment.
9. The Collector mounts the source volume read/write at `/var/lib/lightspeed-data/otel`; the exporter mounts that same volume read-only at `/input`, so the active file and rotated siblings appear directly under `/input`. A separate writable pod-local `emptyDir` is mounted at `/state` for the exporter ledger. The exporter does not delete or modify Collector source files.
10. The exporter polls every 300 seconds and considers rotated trace backups only. It excludes the active `traces.jsonl`, so an active file is not ingested until rotation. Upload is best-effort, not a delivery guarantee; successful acknowledgements are recorded in `/state/ledger.json`.
11. FileExporter rotates by size and applies the configured 40-backup and one-day retention limits. Retention may remove a backup before it is uploaded. When an inventory no longer contains a source file, the exporter prunes that file's ledger entry. A quiet active file can remain unuploaded until it rotates.
12. Both the source and ledger volumes are pod-local `emptyDir` volumes. They persist across container restarts within a Pod, but not Pod replacement; a rollout can therefore lose unuploaded backups and ledger state.
13. A serialized trace write larger than 10MiB fails as an export error; FileExporter does not split the trace batch. The existing 20MiB OTLP receiver request limit does not ensure a serialized FileExporter write fits within 10MiB. Stock FileExporter errors can propagate through the shared trace request and affect delivery to other trace destinations.
14. The Collector image must contain the stock contrib FileExporter v0.159.0; `--otel-collector-image` selects it. The OTel Dataverse sidecar reuses `r.GetDataverseExporterImage()` and the existing `--dataverse-exporter-image` override. It requires an exporter image with PR #147's OTel ingestion support. The current released default pin lacks that support, so the existing override can select a compatible test image for this rollout. Before merging or releasing the follow-up, update the released default pin to a compatible released exporter; do not pin a temporary CI image.
15. The separate app-server Dataverse exporter and its configuration remain unchanged. The existing `lightspeed-agentic-configuration` handoff continues to publish its current OTLP/admin endpoints and CA Secret names.

## Cross-References

- [`crd-api.md`](crd-api.md) — existing transcript opt-out field
- [`templog.md`](templog.md) — Collector OTLP pipelines and PostgreSQL logs
- [`observability.md`](observability.md) — collection gates and behavior
- [`security.md`](security.md) — exporter token, RBAC, and mount isolation
- [`resource-lifecycle.md`](resource-lifecycle.md) — telemetry-secret gate changes
- [`agentic-sandbox-profile.md`](agentic-sandbox-profile.md) — unchanged handoff
- [`deployment-generation.md`](../how/deployment-generation.md) — Collector source/state volumes, Dataverse sidecar configuration, and image selection

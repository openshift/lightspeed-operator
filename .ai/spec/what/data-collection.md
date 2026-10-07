# Trace Data Collection — Operator

This specification defines Collector-side storage of selected native OTLP traces by the lightspeed-operator.

## Behavioral Rules

1. Trace-file collection is enabled when the existing `spec.ols.userDataCollection.transcriptsDisabled` field is `false` or unset; `true` disables this trace branch. The operator adds no CRD field. Collector trace-file storage does not require telemetry credentials; the `cloud.openshift.com` pull-secret auth-entry check applies only to the separate app-server Dataverse exporter.
2. The `routing/data_collection` connector selects OTLP traces whose resource `service.name` is exactly `lightspeed-agentic-operator` or `lightspeed-agentic-sandbox` and routes them to `traces/data_collection`.

   ```yaml
   routing/data_collection:
     table:
       - context: resource
         condition: attributes["service.name"] == "lightspeed-agentic-operator" or attributes["service.name"] == "lightspeed-agentic-sandbox"
         pipelines: [traces/data_collection]
   ```

3. `traces/data_collection` sends selected traces directly to `file/data_collection` without a batch processor. When trace forwarding is configured, its separate pipeline applies batching independently. The file branch collects traces and their native span events; OTLP logs remain on the existing PostgreSQL path.
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

5. The Collector Deployment mounts its `data-collection` `emptyDir` at `/var/lib/lightspeed-data` in the Collector container only, with `sizeLimit: 500Mi`. The Collector's `file_storage` queue uses a separate volume.
6. One active file plus up to forty 10MiB backups implies about 410MiB of active-plus-backup data. This is a nominal sizing estimate, not a quota; the pod-local `data-collection` `emptyDir` has `sizeLimit: 500Mi`.
7. FileExporter rotates by size. Count/age backup eviction is best-effort; age-based cleanup is asynchronous. Under low activity, a quiet active file can remain unrotated through inactivity or shutdown, so active-file consumption remains a follow-up concern. Rotation and retention manage local source files; they do not acknowledge downstream receipt.
8. A serialized trace write larger than 10MiB fails as an export error; FileExporter does not split the trace batch. The existing 20MiB OTLP receiver request limit does not ensure a serialized FileExporter write fits within 10MiB. Stock FileExporter errors can propagate through the shared trace request and affect delivery to other trace destinations.
9. The source files are stored on the Collector pod-local `emptyDir` and do not survive removal of that pod or volume.
10. This trace branch does not generate a Dataverse sidecar; conversion, upload, and checkpointing are outside this operator behavior. The separate app-server Dataverse exporter (`lightspeed-to-dataverse-exporter`) and its configuration remain unchanged. The `lightspeed-agentic-configuration` handoff continues to publish its existing OTLP/admin endpoints and CA Secret names.
11. The selected Collector image must contain the stock contrib FileExporter v0.159.0. The existing `--otel-collector-image` override selects the runtime image.

## Cross-References

- [`crd-api.md`](crd-api.md) — existing collection opt-out field
- [`templog.md`](templog.md) — Collector OTLP pipelines and PostgreSQL logs
- [`observability.md`](observability.md) — collection behavior summary
- [`agentic-sandbox-profile.md`](agentic-sandbox-profile.md) — existing handoff
- [`deployment-generation.md`](../how/deployment-generation.md) — Collector volume and image selection

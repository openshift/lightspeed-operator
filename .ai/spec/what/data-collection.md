# Trace Data Collection — Operator

This specification defines how the lightspeed-operator configures Collector-side capture of selected native OTLP traces and their upload to Dataverse.

[PLANNED: OLS-4493] The released operator stores selected traces with the stock FileExporter on a pod-local volume, and an OTel Dataverse exporter sidecar uploads rotated backups. OLS-4493 replaces that file transport with the OLS `dataverse` exporter inside the Collector. The rules below describe the target. The parent contract is `ols/.ai/spec/what/agentic-data-collection.md` rules 24-37 and decision 0052.

## Behavioral Rules

1. [PLANNED: OLS-4493] The product branch is enabled only when transcripts are enabled (`!spec.ols.userDataCollection.transcriptsDisabled`) and `openshift-config/pull-secret` is successfully read with valid `.dockerconfigjson` JSON containing a nonempty (after trimming whitespace) `.dockerconfigjson.auths["cloud.openshift.com"].auth` token. Feedback enablement does not gate it. The operator adds no CRD field.
2. [PLANNED: OLS-4493] When the gate is off (transcripts disabled, pull-secret object NotFound, or valid JSON with an absent, empty, or whitespace-only token), the operator omits `routing/data_collection`, `traces/data_collection`, the `dataverse` exporter, the Collector credential mount, the trusted CA bundle ConfigMap and mount, and the exporter-only RBAC: ClusterRoles `lightspeed-otel-dataverse-exporter` and `lightspeed-otel-dataverse-exporter-pull-secret`, ClusterRoleBinding `lightspeed-otel-dataverse-exporter-binding`, and RoleBinding `lightspeed-otel-dataverse-exporter-pull-secret` in `openshift-config`. No local trace copy exists without upload credentials. When credential checking runs (with transcripts enabled), a failed API read other than NotFound, a missing `.dockerconfigjson` key on an existing Secret, or malformed JSON is a reconciliation error, not a disabled-auth result, and does not trigger cleanup. The existing app-server auth-entry-presence gate and its invalid-pull-secret Phase 1 error behavior remain unchanged.
3. The `routing/data_collection` connector selects OTLP traces whose resource `service.name` is exactly `lightspeed-agentic-operator` or `lightspeed-agentic-sandbox` and routes them to `traces/data_collection`.

   ```yaml
   routing/data_collection:
     table:
       - context: resource
         condition: attributes["service.name"] == "lightspeed-agentic-operator" or attributes["service.name"] == "lightspeed-agentic-sandbox"
         pipelines: [traces/data_collection]
   ```

4. [PLANNED: OLS-4493] `traces/data_collection` sends selected traces directly to the `dataverse` exporter without a batch processor; the exporter batches with its own sending queue. When trace forwarding is configured, its separate pipeline applies batching independently. OTLP logs remain in the existing `templog` pipeline: they are exported to PostgreSQL when `spec.audit.logging` is enabled and use the `nop` exporter otherwise. OTLP logs are never sent to Dataverse.
5. [PLANNED: OLS-4493] The operator generates the `dataverse` exporter in the Collector runtime ConfigMap with these settings. Values not shown use the exporter defaults defined in the Collector repository spec.

   ```yaml
   dataverse:
     endpoint: https://console.redhat.com/api/ingress/v1/upload
     tls:
       min_version: "1.2"          # from the effective TLS profile (rule 6)
       cipher_suites: [ ... ]      # from the effective TLS profile, IANA names
       ca_file: /etc/certs/trusted-ca/ca-bundle.crt
     service_id: ols               # rhos-lightspeed when the CR has openstack.org/lightspeed-owner-id
     archive_path_prefix: agentic-v1/
     pull_secret:
       namespace: openshift-config
       name: pull-secret
       registry: cloud.openshift.com
   ```

   The service-ID selection and ingress URL match the existing app-server exporter. The operator never sets `proxy_url`. The configuration holds no credentials.
6. [PLANNED: OLS-4493] The operator resolves the effective TLS profile with the existing resolver: `spec.ols.tlsSecurityProfile`, then the cluster `APIServer` profile, then the default. It writes the profile's minimum version as `tls.min_version` and converts its OpenSSL cipher names to the IANA names that `configtls` accepts. It leaves out and logs ciphers Go does not support. A profile change regenerates the Collector ConfigMap and rolls the Collector.
7. [PLANNED: OLS-4493] The operator creates a ConfigMap labeled `config.openshift.io/inject-trusted-cabundle: "true"` in the operator namespace and mounts it read-only in the Collector container. The Cluster Network Operator fills it with the system CAs plus the cluster proxy's trusted CA. The operator does not overwrite the injected data during reconcile. `tls.ca_file` points at its `ca-bundle.crt`.
8. [PLANNED: OLS-4493] The Collector container keeps the cluster-wide proxy environment variables from `utils.GetProxyEnvVars()`; the exporter's uploads follow them. `spec.ols.proxyConfig` does not apply to the exporter.
9. [PLANNED: OLS-4493] The Collector Pod keeps `automountServiceAccountToken: false`. While the gate is on, the operator mounts the projected ServiceAccount token, `kube-root-ca.crt` CA, and namespace read-only at the default in-cluster auth path in the Collector container. The exporter reads the pull-secret token and `ClusterVersion` cluster ID through the Kubernetes API with that token. The operator does not copy the pull-secret token.
10. [PLANNED: OLS-4493] The operator keeps the exporter RBAC from rule 2, bound to the Collector ServiceAccount: cluster-version `get`, and `get` on the named pull secret only.
11. [PLANNED: OLS-4493] The operator generates no OTel Dataverse exporter sidecar, no dedicated OTel exporter ConfigMap or its version annotation, no `data-collection` source volume, no `dataverse-exporter-state` ledger volume, and no `file/data_collection` exporter. On upgrade, the operator removes the old sidecar ConfigMap. The old pod's unuploaded files are lost once with its `emptyDir`.
12. [PLANNED: OLS-4493] Exporter failures do not change `OLSConfig` status and raise no alert. `transcriptsDisabled: true` is the only user control. The Collector log level stays as generated today, so the exporter's info-level upload lines with `request_id` appear in Collector logs.
13. [PLANNED: OLS-4493] The Collector image must contain the OLS `dataverse` exporter; `--otel-collector-image` selects it, and the image and generated configuration ship in one bundle. The operator no longer uses `r.GetDataverseExporterImage()` for the Collector Pod; the app-server exporter still uses it.
14. The separate app-server Dataverse exporter and its configuration remain unchanged. The existing `lightspeed-agentic-configuration` handoff continues to publish its current OTLP/admin endpoints and CA Secret names.

## Planned Changes

| Ticket | Summary |
|---|---|
| OLS-4493 | Replace the FileExporter branch and the OTel Dataverse exporter sidecar with the in-Collector `dataverse` exporter. Gate the whole product branch on transcripts plus the pull-secret token. Generate the exporter config with the TLS profile and trusted CA bundle. Mount the projected token into the Collector container. Remove the sidecar, its ConfigMap, and both data volumes. |

## Cross-References

- [`crd-api.md`](crd-api.md) — existing transcript opt-out field
- [`templog.md`](templog.md) — Collector OTLP pipelines and PostgreSQL logs
- [`observability.md`](observability.md) — collection gates and behavior
- [`security.md`](security.md) — exporter token, RBAC, and mount isolation
- [`tls.md`](tls.md) — TLS security profile resolution
- [`resource-lifecycle.md`](resource-lifecycle.md) — telemetry-secret gate changes
- [`agentic-sandbox-profile.md`](agentic-sandbox-profile.md) — unchanged handoff
- [`deployment-generation.md`](../how/deployment-generation.md) — Collector credential and trusted CA mounts, and image selection

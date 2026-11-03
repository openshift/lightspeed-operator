# OLS 2.0 ARM64 packaging

Tracked by [OLS-4631](https://redhat.atlassian.net/browse/OLS-4631).

## Build configuration

The shared OLS 2.0 bundle contains both the classic and agentic operator deployments. It is built by `.tekton/ols-bundle-{pull-request,push}.yaml`; the agentic operator repository does not publish a separate OLM bundle.

The OLM bundle PipelineRuns target only `linux/x86_64`, disable image index creation, and publish a single-architecture image manifest. Enterprise Contract's `olm.olm_bundle_multi_arch` policy rejects bundle indexes, even when the bundle contents are architecture-independent. FBC PipelineRuns still target `linux/x86_64` and `linux/arm64` for OCP 4.16 through 4.22 and 5.0. Their build matrices produce platform-specific intermediate tags, and index assembly publishes combined catalog image indexes. Snapshots and promotion must use the bundle's single-image digest and each catalog's index digest, not an architecture-specific catalog child digest.

Historical OLS 1.x bundle content is unchanged. These changes enable packaging and do not by themselves certify ARM support or add ARM E2E provisioning.

## Local checks

```sh
make test
git diff --check
```

Review the two bundle descriptors for a single platform with index creation and platform-suffixed image tags disabled. Review the 16 FBC descriptors for explicit platform lists, effective index enablement, build matrix and index-input wiring, intermediate-tag uniqueness, published index results, and a descriptor-change trigger. Local checks do not execute CEL, validate remote task implementations, or prove image builds succeed. OCP 5.0 descriptor-only changes now trigger builds, matching the other catalogs.

## Release validation checklist

Before advertising ARM support:

- Inspect every exact `FROM` reference in `bundle.Dockerfile` and the eight catalog Dockerfiles for both amd64 and arm64 variants. Authenticate using the normal registry login flow; do not put credentials in scripts or this document.
- Run actual Konflux PR/push builds: the bundle on amd64 only, catalogs on both architectures (including catalog `opm` cache generation and applicable policy checks). Validate the bundle and each catalog with the supported operator-sdk/opm tooling.
- Inspect the bundle image's top-level media type to confirm it is a single image manifest, and inspect catalog image indexes for `linux/amd64` and `linux/arm64` entries.
- Confirm snapshots, staging/production promotion, and catalog-generation automation preserve the bundle's image digest and each catalog's index digest. Review external `konflux-release-data` configuration and downstream catalog architecture policy; local FBC builds do not prove downstream ARM availability.
- Review supported-architecture metadata/certification requirements; if labels are required, update the generation source and regenerate artifacts rather than editing only the generated CSV.
- Inspect the exact released image references in the shared bundle, including both operator deployments and required enabled operands. Check both deployments for incompatible architecture scheduling rules. Existing dual-arch runtime build configuration is not evidence that every pinned released digest is multi-arch.
- Verify staging publication exposes OLS 2.0 for ARM on each listed OCP catalog. Record the OCP versions actually tested; do not imply cluster qualification for untested versions.
- On an ARM-only cluster, install through OLM from the staging catalog and verify both classic and agentic controllers start and reconcile. Exercise an agentic run to verify sandbox startup and inter-operator handoff.
- Run an amd64 installation regression check. Report unresolved operand, OCP-version, or release-policy blockers explicitly.

## Verification recorded during implementation

Local `make test` passed after the configuration changes. A temporary packaging regression test demonstrated the expected failures before descriptor changes and passed afterward; that test and its Makefile target were subsequently removed at user request.

After registry authentication, `skopeo inspect --raw docker://IMAGE` successfully verified **21 unique image references**, each containing both `linux/amd64` and `linux/arm64` manifests:

- All nine base references: the bundle's `ubi9/ubi-minimal:9.8-1788939036`, the seven `openshift4/ose-operator-registry-rhel9:v4.16` through `:v4.22` references, and `openshift5/ose-operator-registry-rhel9:v5.0`.
- All twelve digest-pinned `spec.relatedImages` in the current shared bundle: classic operator, agentic operator, service API, classic console, agentic console, sandbox, alerts adapter, OTel collector, PostgreSQL, dataverse exporter, OpenShift MCP server, and RHOKP. Both operator deployment image references match the checked related images.

This verifies platform availability at the checked references, not successful startup or compatibility. Base references use tags, so their contents can change and must be rechecked when releasing. Konflux builds, downstream release validation, and ARM/amd64 cluster installation checks have not been performed as part of this local change.

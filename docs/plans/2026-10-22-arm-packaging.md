# OLS-4631 ARM64 Packaging Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Enable dual-architecture packaging of the shared OLS 2.0 bundle and catalogs for OCP 4.16–4.22 and 5.0.

**Architecture:** Reuse existing Tekton platform matrices and index assembly. Both classic and agentic operator deployments remain in the single shared bundle. Preserve historical catalog entries and existing image versions.

**Tech Stack:** Tekton, Pipelines-as-Code CEL, Go testing, sigs.k8s.io/yaml, Konflux, OLM.

## Task 1: Packaging configuration review

The temporary regression test and its Makefile target were removed at user request after red/green verification. Retain the existing Makefile unchanged.

1. Review all 18 descriptors for explicit x86/ARM targets, effective image-index enablement, build matrix wiring, unique intermediate tags, index inputs, published index results, and descriptor-change triggers.
2. Verify local configuration with an ad hoc inspection; do not add a persistent packaging test target.

## Task 2: Bundle builds

Modify `.tekton/ols-bundle-pull-request.yaml` and `.tekton/ols-bundle-push.yaml`:

1. Add `linux/arm64` alongside `linux/x86_64` in PipelineRun `build-platforms`.
2. Set PipelineRun `build-image-index` to `"true"`.
3. Set build task `IMAGE_APPEND_PLATFORM` to `"true"`.
4. Keep index assembly and downstream references to index results unchanged.

## Task 3: Catalog builds

Modify both PR and push descriptors for `fbc-v4-16`, `fbc-v4-17`, `fbc-v4-18`, `fbc-v4-19`, `fbc-v4-20`, `fbc-v4-21`, `fbc-v4-22`, and `fbc-v5-0` under `.tekton/`.

1. Add `linux/arm64` to each explicit PipelineRun platform list; retain x86.
2. Add each 5.0 descriptor's own path to its CEL path-change disjunction. Remove the obsolete empty-catalog trigger comment now that the catalog contains 2.0.0.
3. Preserve index/tag wiring and all historical bundle entries.
4. Inspect all 18 descriptors and verify the platform, index, tagging, and trigger settings.

## Task 4: Local and external verification

1. Run `make test` and `git diff --check`; expect success.
2. Inspect every unique `FROM` reference used by `bundle.Dockerfile` and the eight catalog Dockerfiles with `skopeo inspect --raw docker://IMAGE`; verify linux amd64/arm64 manifests. Record authentication/network failures as unverified gates, not success.
3. Document release gates in `docs/arm-packaging.md`: real dual-arch Konflux builds, index-preserving promotion, downstream publication for all eight OCP versions, required architecture metadata review, exact released operand manifests, both controllers on ARM, agentic sandbox/handoff smoke test, amd64 regression, and tested-version evidence.
4. Review the complete diff. Do not modify runtime operators, historical bundles, published digests, version labels, or create new ARM E2E provisioning pipelines.
5. Do not push or publish artifacts as part of local verification. Use OLS-4631 for any subsequent commit/PR title and the fork workflow when explicitly requested.

## Completion boundary

Local configuration/test completion is distinct from release qualification. Registry access, Konflux runs, release configuration in external repositories, and ARM cluster access may require human coordination. Do not declare full ARM product support based on passing local tests.

# Credential Hot-Reload (RFE-9380)

## Overview

When `credentialHotReload` is enabled on the OLSConfig CR, the operator skips
rolling restarts of the app-server pod when LLM credential secret **data**
changes. The service re-reads credential files from disk on every LLM request,
so rotated tokens take effect without downtime.

This feature is a companion to
[lightspeed-service PR #2955](https://github.com/openshift/lightspeed-service/pull/2955),
which adds the in-process credential reload on the service side.

## Configuration

```yaml
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
spec:
  ols:
    credentialHotReload: true   # default: false
```

## Behavior

| Event | `credentialHotReload: false` (default) | `credentialHotReload: true` |
|---|---|---|
| LLM secret **data** rotated (same secret name) | Rolling restart | **No restart** — service picks up new credentials on next request |
| LLM secret **ref** changed in CR (different secret name) | Rolling restart | Rolling restart (deployment volume spec changes) |
| TLS / MCP / Postgres secret changed | Rolling restart | Rolling restart (unchanged) |

## Prerequisites

- The lightspeed-service image must include the `get_credentials()` hot-reload
  support (lightspeed-service >= the version containing PR #2955). If an older
  service image is used with this flag enabled, rotated credentials will not be
  picked up until the pod is manually restarted.

## How It Works

1. During reconciliation, the operator reads `spec.ols.credentialHotReload` from
   the OLSConfig CR.

2. When `credentialHotReload` is enabled, the operator **removes the watcher
   annotation** from LLM provider secrets (those with a `llm-provider-*` source)
   via `removeSecretAnnotationIfNeeded`. Without the annotation, the secret
   watcher ignores data changes on these secrets, so no rolling restart occurs.

3. The operator propagates `credential_hot_reload: true` into the generated
   `olsconfig.yaml` ConfigMap. The service reads this flag and re-reads
   credential files from disk on every LLM request.

4. When `credentialHotReload` is disabled (or toggled back to false), the
   operator re-annotates the LLM secrets, restoring the normal watch-and-restart
   behavior.

5. Non-LLM secrets (TLS, MCP headers, Postgres) are always annotated and always
   trigger restarts regardless of the flag.

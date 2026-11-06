# 0003: Deterministic Generated Configuration

- **Status:** Accepted; implemented under [OLS-4563](https://redhat.atlassian.net/browse/OLS-4563).
- **Date:** 2026-11-05
- **Behavioral contract:** [TLS additional CA](../what/tls.md#additional-ca-certificates) rule 3; [Config generation](../how/config-generation.md#deterministic-generated-data); [Deployment change detection](../how/deployment-generation.md#change-detection-pattern).

## Context

`addAdditionalCAFileNames` built the `extra_ca` list by ranging over a Go map. With two or more keys, the order changed on every reconcile. The generated `olsconfig` ConfigMap therefore differed byte-for-byte, its resourceVersion changed, and the app-server Deployment restarted on every reconcile. OLS-4563 produced 1,057 ReplicaSets in 15 minutes; the serving pod itself did not restart because no replacement became ready.

## Decision

Generated configuration MUST be deterministic: identical inputs produce identical bytes. Lists built from maps are sorted before marshaling. The operator keeps byte-wise `ConfigMapEqual` comparison and resourceVersion-based change detection. A whole-config determinism test guards the class; new map-backed inputs are added to its fixture.

## Alternatives

1. **Semantic ConfigMap comparison:** parse and compare the YAML, ignoring reordering. Rejected: broad, needs a definition of equality for every field, and hides nondeterministic generators.
2. **Content-hash restart trigger:** hash the ConfigMap data instead of tracking its resourceVersion. Rejected for this bug: the data hash still changes when the order flips. It is useful hardening for metadata-only changes, not a fix.
3. **Debounce or rate-limit restarts:** rejected: masks the loop and delays legitimate configuration changes.

## Consequences

- Every new list built from a map MUST be sorted or canonicalized before marshaling.
- The determinism guard test must include new map-backed inputs; otherwise a new section can regress unnoticed.
- Byte-wise comparison remains intentionally strict. Whitespace changes in `Data` or `BinaryData` count as content changes. Metadata-only changes do not, because `ConfigMapEqual` does not compare metadata.
- `extra_ca` order is not semantically meaningful, so sorting is safe there. New list fields must be checked for order semantics before sorting.

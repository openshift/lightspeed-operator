# Agentic inputs for the unified OLS bundle

`config/manifests` includes this directory for every supported OpenShift
version. The one bundle CSV installs both the Classic and agentic controller
Deployments, owns the Classic and ten agentic CRDs, and includes the agentic
manager RBAC in `clusterPermissions`. The human-approver ClusterRole and
ClusterRoleBinding and the webhook Service are standalone bundle manifests;
webhook configurations are embedded in the CSV.

The agentic CRDs and manager/approver RBAC in `config/agentic/` are checked-in
inputs synced from lightspeed-agentic-operator at the **full pinned commit** in
the root `Makefile` (OLS-3189). Run `make sync-agentic-crds` explicitly only
when advancing the pin, review the resulting diff, and regenerate `bundle/`
with `make bundle`. The sync command verifies the exact source commit before
changing files. Routine bundle generation uses the checked-in inputs without
network access or an agentic-operator Go API dependency; do not hand-edit the
synced files. The manager Deployment (`manager.yaml`) has bundle-specific
naming and image wiring and is maintained separately.

The webhook configurations use OpenShift service-ca injection and the Service
uses a serving-cert Secret. A standalone NetworkPolicy cannot be emitted as a
registry+v1 bundle manifest by operator-sdk. Do not assume the proposed
runtime webhook ingress policy in agentic-operator #547 is available: that PR
was closed without merging. Verify admission, ingress and TLS on both OCP 4.x
and 5.x before releasing the bundle.

**Version-gating release blocker:** The intended unified-bundle contract gates
only the agentic console on OCP 4.x. As of this bundle candidate, Classic
operator main still gates the alerts adapter, handoff and client CA paths too;
the console-only change (#2145) closed without merging. The agentic operator's
broad backend gate (#546) was reverted by #559. This bundle PR does not fix
that mismatch or prove cross-version upgrades or downgrades. Do not publish
until the Classic gate is corrected and the combined behavior is validated.

Per-run sandbox RBAC is created at runtime, not packaged as static bundle RBAC.
Verify that the regenerated CSV contains both controller Deployments, all ten
owned agentic CRDs and agentic manager permissions, and that the approver RBAC
appears as standalone manifests. The `config/agentic/` inputs are not a
release artifact on their own.

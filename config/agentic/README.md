# Agentic controller inputs for the single OLS bundle

`config/manifests` includes this directory for every supported OpenShift version.
The single CSV installs both the Classic and agentic controller deployments and
owns the Classic and agentic CRDs. The agentic manager RBAC is a CSV cluster
permission; `agentic-run-approver` RBAC and the webhook Service are standalone
manifests. The webhook definitions are embedded in the CSV.

OpenShift 4.x installs these static resources, but the agentic runtime gate
keeps the controller healthy and inactive; Classic deploys no agentic operands.
The controller and agentic operands activate on a completed OpenShift 5.0+
upgrade. An unreadable or unknown version is inactive.

CRDs and RBAC are synced from the agentic-operator source, not hand-edited.
When the agentic API or controller RBAC changes, update the synced inputs and
regenerate `bundle/` using `make bundle`. The manager image and all related
image digests come from `related_images.json`; no bundle variant is selected.

The webhook configurations use OpenShift service-ca injection; the Service
uses a serving-cert Secret. NetworkPolicy cannot be emitted as a standalone registry+v1 bundle manifest
by operator-sdk. The agentic controller manages webhook ingress for its Service
at runtime on both 4.x and 5.x (see agentic-operator #547); it is deliberately
not included in this Kustomize input. Do not treat this directory as a release
artifact by itself.

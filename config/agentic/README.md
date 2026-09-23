# Agentic manifest inputs (OLS-3189)

`make sync-agentic-crds` fetches the ten agentic CRDs and manager/approver RBAC
from the agentic-operator repo at the exact commit pinned in the root `Makefile`.
The source URL and fetch ref are also recorded there. The ref points to the
upstream main branch, and the script verifies its full pinned commit SHA before
updating any files; moving that branch cannot silently change the bundle.
Advance the commit pin deliberately when resyncing.

The synced CRDs live in `config/agentic/crds/`; the RBAC inputs and service
account live in `config/agentic/`. These are checked-in source inputs, not
files to edit by hand. Run the sync target explicitly when the source pin
changes, review its CRD and RBAC diff, then regenerate the bundle. Routine
`make bundle` runs use checked-in inputs and do not need GitHub access. This
repo does not import the agentic-operator Go API module.

The single-bundle assembly in PR #2113 includes these inputs for **both** OCP
4.x and 5.x: the CSV owns the agentic CRDs, installs a second controller, and
includes its manager RBAC as `clusterPermissions`; the approver ClusterRole and
ClusterRoleBinding are standalone bundle manifests. The Classic operator currently gates the agentic console, alerts adapter,
and integration handoff by OCP version; the agentic operator does not gate its
backend on OCP version. Per-run sandbox RBAC is created by the agentic controller
at runtime, not included as static bundle RBAC. Until #2113 lands, this branch supplies only the synced inputs: its
current bundle generation does not prove that full composition.

The manager deployment (`manager.yaml`) has bundle-specific naming and image
wiring; update that separately from the CRD/RBAC sync. After #2113 merges,
run `make bundle` and verify the resulting single CSV contains both controller
deployments, all ten owned CRDs, the agentic manager permissions and the
standalone approver RBAC.

# Agentic controller bundle inputs (OLS-3188)

These manifests supply the **agentic layer** shipped only in the **v2 bundle**
(OCP >= 5.0). They are the kustomize inputs that `operator-sdk generate bundle`
folds into the v2 CSV, so the v2 CSV differs from v1 by:

- a **second deployment** — `lightspeed-operator-agentic-controller-manager`
  (`manager.yaml`), alongside the classic `lightspeed-operator-controller-manager`;
- agentic **`clusterPermissions`** — the `agentic-operator-manager-role` rules
  (`role.yaml` + `role_binding.yaml`);
- static v2 bundle RBAC manifests — the `agentic-run-approver` ClusterRole and
  `agentic-run-approver-binding` ClusterRoleBinding (`run_approver_role.yaml`,
  `run_approver_binding.yaml`) for human (cluster-admin) run approval.

The v1 (classic, OCP 4.x) bundle contains none of these.

## Source of truth / sync

Per `bundle-composition.md` (Constraint 2), the agentic CRDs and RBAC are owned
by the **agentic-operator** repo and synced here — do not hand-edit the rule or
spec content. These files were transcribed from:

| This file | agentic-operator source |
|---|---|
| `manager.yaml` | `config/manager/manager.yaml`, `config/rbac/service_account.yaml` |
| `role.yaml` | `config/rbac/role.yaml` |
| `role_binding.yaml` | `config/rbac/role_binding.yaml` |
| `secret_reader_role_binding.yaml` | `config/rbac/secret_reader_role_binding.yaml` |
| `run_approver_role.yaml` | `config/rbac/run_approver_role.yaml` |
| `run_approver_binding.yaml` | `config/rbac/run_approver_binding.yaml` |

The agentic source names follow the existing manager convention
(`agentic-controller-manager`); `config/default` applies the
`lightspeed-operator-` prefix. Like the classic manager configuration, `system`
remains a Kustomize namespace
placeholder; the v2 assembly supplies the deployed namespace, including RBAC
subjects. The controller receives that namespace through `POD_NAMESPACE`, since
Kustomize does not transform container arguments. The controller image is a
placeholder substituted by `update_bundle.sh v2` from `related_images.json`.

## Wiring status (IMPORTANT)

On `main`, this directory is **not yet referenced** by `config/default` or
`config/manifests`, and there is no v2 kustomize assembly to include it. The
selector-driven build (`update_bundle.sh v1|v2`, OLS-4008), the `-v1`/`-v2` CSV
base selection (OLS-4009), and the agentic CRD sync make target (OLS-3189) are
the pieces that wire these inputs into a buildable v2 bundle. That wiring is
applied when those changes land / at merge time.

The target runtime contract these inputs must satisfy is asserted by the
version-gating e2e (OLS-4012).

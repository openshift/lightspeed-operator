#!/usr/bin/env bash
# Offline contract test for the pinned Git fetch and manifest adaptation.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/source/config/crd/bases" "$tmp/source/config/rbac" \
  "$tmp/source/config/admission" "$tmp/source/config/default" \
  "$tmp/source/config/manager" "$tmp/source/config/webhook" \
  "$tmp/source/config/samples" "$tmp/source/config/extra" \
  "$tmp/target/config/agentic/upstream" "$tmp/target/hack"
git -C "$tmp/source" init -q
git -C "$tmp/target" init -q
cp "$root/hack/sync_agentic_manifests.sh" "$tmp/target/hack/"
for name in agenticolsconfigs agenticruns; do
  printf '%s\n' '---' 'kind: CustomResourceDefinition' "name: $name" > "$tmp/source/config/crd/bases/agentic.openshift.io_${name}.yaml"
done
{
  printf '%s\n' 'apiVersion: kustomize.config.k8s.io/v1beta1' 'kind: Kustomization' 'resources:'
  printf '%s\n' '  - bases/agentic.openshift.io_agenticolsconfigs.yaml' '  - bases/agentic.openshift.io_agenticruns.yaml'
} > "$tmp/source/config/crd/kustomization.yaml"
for base in role role_binding run_approver_role run_approver_binding service_account; do
  printf '%s\n' '---' 'namespace: __OPERATOR_NAMESPACE__' 'name: controller-manager' > "$tmp/source/config/rbac/${base}.yaml"
done
for dir in admission default manager; do
  printf 'kind: ConfigMap\n' > "$tmp/source/config/${dir}/example.yaml"
done
printf 'kind: ConfigMap\n' > "$tmp/source/config/samples/example.yaml"
printf 'kind: ConfigMap\n' > "$tmp/source/config/extra/example.yaml"
printf 'obsolete nested mirror\n' > "$tmp/target/config/agentic/upstream/old.yaml"
printf 'kind: ConfigMap\n' > "$tmp/source/config/webhook/obsolete.yaml"
printf '%s\n' '---' 'kind: MutatingWebhookConfiguration' \
  'namespace: __OPERATOR_NAMESPACE__' '---' 'kind: ValidatingWebhookConfiguration' \
  'namespace: __OPERATOR_NAMESPACE__' > "$tmp/source/config/webhook/manifests.yaml"
printf '%s\n' '---' 'kind: Service' 'namespace: __OPERATOR_NAMESPACE__' \
  'control-plane: controller-manager' > "$tmp/source/config/webhook/service.yaml"
printf '%s\n' 'kind: NetworkPolicy' 'namespace: __OPERATOR_NAMESPACE__' \
  > "$tmp/source/config/webhook/networkpolicy.yaml"
git -C "$tmp/source" add .
git -C "$tmp/source" -c user.name=sync-test -c user.email=sync-test@example.com commit -qm initial
export AGENTIC_OPERATOR_REPO="$tmp/source" AGENTIC_OPERATOR_REF=HEAD
export AGENTIC_OPERATOR_COMMIT=$(git -C "$tmp/source" rev-parse HEAD)
script="$tmp/target/hack/sync_agentic_manifests.sh"
(cd "$tmp/target" && bash "$script" > /dev/null)
crd="$tmp/target/config/agentic/crd/bases/agentic.openshift.io_agenticruns.yaml"
role="$tmp/target/config/agentic/role.yaml"
[[ $(grep -c '^  - bases/agentic.openshift.io_' "$tmp/target/config/agentic/crd/kustomization.yaml") == 2 ]]
[[ $(grep -c '^---$' "$crd") == 1 ]] # exact upstream CRD mirror keeps upstream document marker
grep -q 'namespace: openshift-lightspeed' "$role"
grep -q 'name: lightspeed-agentic-operator-controller-manager' "$tmp/target/config/agentic/service_account.yaml"
grep -q 'name: lightspeed-agentic-operator-controller-manager' "$tmp/target/config/agentic/role_binding.yaml"
for dir in admission crd default manager rbac webhook extra; do
  [[ -d "$tmp/target/config/agentic/${dir}" ]]
done
[[ ! -e "$tmp/target/config/agentic/upstream" ]]
[[ ! -e "$tmp/target/config/agentic/samples" ]]
cmp "$tmp/source/config/webhook/networkpolicy.yaml" "$tmp/target/config/agentic/webhook/networkpolicy.yaml"
webhook="$tmp/target/config/agentic/manifests.yaml"
service="$tmp/target/config/agentic/service.yaml"
[[ $(grep -c '^---$' "$webhook") == 1 ]] # Preserve the multi-document webhook configuration.
grep -q 'namespace: openshift-lightspeed' "$webhook"
grep -q 'control-plane: agentic-controller-manager' "$service"
[[ ! -e "$tmp/target/config/agentic/networkpolicy.yaml" ]] # Not a valid OLM bundle manifest.
old=$(sha256sum "$role")
webhook_old=$(sha256sum "$webhook")

# Moving the fetch ref must fail before updating any destination file.
printf '%s\n' '# changed' >> "$tmp/source/config/rbac/role.yaml"
rm "$tmp/source/config/crd/bases/agentic.openshift.io_agenticruns.yaml"
{
  printf '%s\n' 'apiVersion: kustomize.config.k8s.io/v1beta1' 'kind: Kustomization' 'resources:'
  printf '%s\n' '  - bases/agentic.openshift.io_agenticolsconfigs.yaml'
} > "$tmp/source/config/crd/kustomization.yaml"
rm "$tmp/source/config/webhook/obsolete.yaml"
rm -rf "$tmp/source/config/extra"
printf '%s\n' '# changed' >> "$tmp/source/config/webhook/manifests.yaml"
git -C "$tmp/source" add -A
git -C "$tmp/source" -c user.name=sync-test -c user.email=sync-test@example.com commit -qm updated
if (cd "$tmp/target" && bash "$script" > /dev/null 2>&1); then
  echo 'sync accepted a source ref that moved past the pinned SHA' >&2
  exit 1
fi
[[ $(sha256sum "$role") == "$old" ]]
[[ $(sha256sum "$webhook") == "$webhook_old" ]]
export AGENTIC_OPERATOR_COMMIT=$(git -C "$tmp/source" rev-parse HEAD)
(cd "$tmp/target" && bash "$script" > /dev/null)
grep -q '# changed' "$role"
grep -q '# changed' "$webhook"
[[ ! -e "$crd" ]]
[[ ! -e "$tmp/target/config/agentic/webhook/obsolete.yaml" ]]
[[ ! -e "$tmp/target/config/agentic/extra" ]]
[[ ! -e "$tmp/target/config/agentic/upstream" ]]
[[ $(grep -c '^  - bases/agentic.openshift.io_' "$tmp/target/config/agentic/crd/kustomization.yaml") == 1 ]]
echo 'pinned agentic manifest sync contract passed'

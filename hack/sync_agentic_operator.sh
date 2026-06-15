#!/usr/bin/env bash
# Sync CRDs, samples, and RBAC from lightspeed-agentic-operator.
# Pre-requisites: git
# Usage: ./hack/sync_agentic_operator.sh <repo-url> <git-ref> <crd-dest-dir>
set -euo pipefail

REPO="${1:?Usage: $0 <repo-url> <git-ref> <crd-dest-dir>}"
REF="${2:?Usage: $0 <repo-url> <git-ref> <crd-dest-dir>}"
DEST="${3:?Usage: $0 <repo-url> <git-ref> <crd-dest-dir>}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Syncing agentic operator from ${REPO} at ref ${REF}..."
git init "${TMPDIR}" --quiet
git -C "${TMPDIR}" remote add origin "${REPO}"
git -C "${TMPDIR}" fetch --depth 1 origin "${REF}" --quiet
git -C "${TMPDIR}" checkout FETCH_HEAD --quiet

# --- CRDs ---
SRC="${TMPDIR}/config/crd/bases"
if [ ! -d "${SRC}" ]; then
    echo "ERROR: ${SRC} not found in cloned repo" >&2
    exit 1
fi

mkdir -p "${DEST}"
rm -f "${DEST}"/agentic.openshift.io_*.yaml

count=0
for f in "${SRC}"/agentic.openshift.io_*.yaml; do
    [ -f "$f" ] || continue
    cp "$f" "${DEST}/"
    echo "  $(basename "$f")"
    count=$((count + 1))
done

if [ "${count}" -eq 0 ]; then
    echo "WARNING: no agentic CRD files found" >&2
    exit 1
fi

KUSTOMIZATION="config/crd/kustomization.yaml"
if [ -f "${KUSTOMIZATION}" ]; then
    sed -i '/^- bases\/agentic\.openshift\.io_/d' "${KUSTOMIZATION}"
    SCAFFOLD_LINE="#+kubebuilder:scaffold:crdkustomizeresource"
    for f in "${SRC}"/agentic.openshift.io_*.yaml; do
        [ -f "$f" ] || continue
        ENTRY="- bases/$(basename "$f")"
        sed -i "s|${SCAFFOLD_LINE}|${ENTRY}\n${SCAFFOLD_LINE}|" "${KUSTOMIZATION}"
    done
    echo "Updated ${KUSTOMIZATION}"
fi

echo "Synced ${count} agentic CRD files to ${DEST}/"

# --- Samples ---
SAMPLES_SRC="${TMPDIR}/config/samples"
SAMPLES_DEST="config/samples"
SAMPLES_KUSTOMIZATION="${SAMPLES_DEST}/kustomization.yaml"
if [ -d "${SAMPLES_SRC}" ]; then
    sed -i '/^- agentic_/d' "${SAMPLES_KUSTOMIZATION}" 2>/dev/null || true
    SAMPLES_SCAFFOLD="#+kubebuilder:scaffold:manifestskustomizesamples"
    sample_count=0
    for f in "${SAMPLES_SRC}"/agentic_*.yaml; do
        [ -f "$f" ] || continue
        cp "$f" "${SAMPLES_DEST}/"
        ENTRY="- $(basename "$f")"
        sed -i "s|${SAMPLES_SCAFFOLD}|${ENTRY}\n${SAMPLES_SCAFFOLD}|" "${SAMPLES_KUSTOMIZATION}"
        echo "  $(basename "$f")"
        sample_count=$((sample_count + 1))
    done
    if [ "${sample_count}" -gt 0 ]; then
        echo "Updated ${SAMPLES_KUSTOMIZATION}"
        echo "Synced ${sample_count} agentic sample files to ${SAMPLES_DEST}/"
    fi
fi

# --- RBAC ---
RBAC_SRC="${TMPDIR}/config/rbac"
RBAC_DEST="config/agentic-rbac"
if [ -d "${RBAC_SRC}" ]; then
    rbac_count=0
    for f in role.yaml role_binding.yaml service_account.yaml proposal_approver_role.yaml proposal_approver_binding.yaml; do
        [ -f "${RBAC_SRC}/${f}" ] || continue
        cp "${RBAC_SRC}/${f}" "${RBAC_DEST}/${f}"
        echo "  rbac: ${f}"
        rbac_count=$((rbac_count + 1))
    done
    # Adapt names for this repo's kustomize namePrefix convention:
    # - ClusterRole name: agentic-operator-manager-role -> agentic-manager-role
    #   (kustomize adds the lightspeed-operator- prefix)
    # - ServiceAccount name/namespace: controller-manager / __OPERATOR_NAMESPACE__
    #   -> agentic-controller-manager / system
    # - ClusterRoleBinding roleRef and subjects: match the renamed ClusterRole/SA
    sed -i 's/name: agentic-operator-manager-role/name: agentic-manager-role/' "${RBAC_DEST}/role.yaml"
    sed -i 's/name: agentic-operator-manager-rolebinding/name: agentic-manager-rolebinding/' "${RBAC_DEST}/role_binding.yaml"
    sed -i 's/name: agentic-operator-manager-role/name: agentic-manager-role/' "${RBAC_DEST}/role_binding.yaml"
    sed -i 's/name: controller-manager/name: agentic-controller-manager/' "${RBAC_DEST}/role_binding.yaml"
    sed -i 's/namespace: __OPERATOR_NAMESPACE__/namespace: system/' "${RBAC_DEST}/role_binding.yaml"
    sed -i 's/name: controller-manager/name: agentic-controller-manager/' "${RBAC_DEST}/service_account.yaml"
    sed -i 's/namespace: __OPERATOR_NAMESPACE__/namespace: system/' "${RBAC_DEST}/service_account.yaml"
    echo "Synced ${rbac_count} agentic RBAC files to ${RBAC_DEST}/"
else
    echo "WARNING: ${RBAC_SRC} not found, skipping RBAC sync" >&2
fi

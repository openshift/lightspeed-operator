#!/usr/bin/env bash
# Konflux operator upgrade: Tekton checks out ${COMMIT_SHA}; checkout related_images operator
# revision; run make test-upgrade. BUNDLE_IMAGE for `operator-sdk run bundle-upgrade` must be
# newer than what is on the cluster: Konflux passes SNAPSHOT + KONFLUX_COMPONENT_NAME so we use
# the same bundle image the pipeline would install second (upgrade target).
#
# Usage (from Tekton, from repo root after checkout of "${COMMIT_SHA}"):
#   bash .tekton/integration-tests/scripts/run-operator-upgrade-tests.sh "$(params.openshift-version-prefix)"
#
# Env (set by Tekton): COMMIT_SHA, OPENAI_PROVIDER_KEY_PATH, SNAPSHOT, KONFLUX_COMPONENT_NAME (optional
# but required together for Konflux). If SNAPSHOT is unset, BUNDLE_IMAGE must already be exported.

set -euo pipefail

openshift_version_prefix="${1:?usage: $0 <openshift-version-prefix e.g. 4.16.>}"

: "${COMMIT_SHA:?COMMIT_SHA must be set}"

# Same as run-operator-e2e-tests.sh: build/run tests from related_images "lightspeed-operator"
# revision so Makefile and CGO build tags match the snapshot.
TEST_SOURCE_COMMIT="$(
	git show "${COMMIT_SHA}:related_images.json" |
		jq -r '.[] | select(.name=="lightspeed-operator") | .revision'
)"
git fetch --depth=1 --filter=blob:none origin "${TEST_SOURCE_COMMIT}"
git checkout "${TEST_SOURCE_COMMIT}"

if [[ -n "${SNAPSHOT:-}" && -n "${KONFLUX_COMPONENT_NAME:-}" ]]; then
	BUNDLE_IMAGE="$(jq -r --arg component_name "${KONFLUX_COMPONENT_NAME}" \
		'.components[] | select(.name == $component_name) | .containerImage' <<<"${SNAPSHOT}")"
	export BUNDLE_IMAGE
	echo "Upgrade target BUNDLE_IMAGE from SNAPSHOT (bundle-upgrade): ${BUNDLE_IMAGE}"
elif [[ -n "${BUNDLE_IMAGE:-}" ]]; then
	echo "Using pre-set BUNDLE_IMAGE: ${BUNDLE_IMAGE}"
else
	PV="${openshift_version_prefix%.}"
	catalog_dir="lightspeed-catalog-${PV}"
	newest_ver="$(ls "${catalog_dir}"/bundle-v*.yaml | sed -n 's/.*bundle-v\(.*\)\.yaml/\1/p' | sort -V | tail -n1)"
	newest_file="${catalog_dir}/bundle-v${newest_ver}.yaml"
	BUNDLE_IMAGE="$(yq '.relatedImages[] | select(.name == "lightspeed-operator-bundle") | .image' "${newest_file}")"
	export BUNDLE_IMAGE
	echo "SNAPSHOT unset: using newest catalog bundle as upgrade target: ${newest_file}"
	echo "${BUNDLE_IMAGE}"
fi
echo "---------------------------------------------"
echo "---------------------------------------------"
echo "---------------------------------------------"

export LLM_TOKEN="$(cat "${OPENAI_PROVIDER_KEY_PATH}")"
export LLM_PROVIDER="openai"
export LLM_MODEL="gpt-4o-mini"
echo "starting tests for ${LLM_PROVIDER} ${LLM_MODEL}"
make test-upgrade

#!/usr/bin/env bash
# Konflux operator integration: Tekton performs git init + fetch of ${COMMIT_SHA};
# this script runs from inside that repo root afterward.
#
# Usage (from Tekton):
#   git show "${COMMIT_SHA}:.tekton/integration-tests/scripts/run-operator-e2e-tests.sh" | bash -s -- "latest-4.17"
#
# Env (set by Tekton): COMMIT_SHA, OPENAI_PROVIDER_KEY_PATH, AZUREOPENAI_PROVIDER_KEY_PATH,
# and standard paths for Azure Entra files under /var/run/azureopenai-entra-id/

set -euo pipefail

ocp_client_channel="${1:?usage: $0 <e.g. latest-4.17>}"

git show "${COMMIT_SHA}:.tekton/integration-tests/scripts/install-oc-if-missing.sh" | bash -s -- "${ocp_client_channel}"

TEST_SOURCE_COMMIT="$(
	git show "${COMMIT_SHA}:related_images.json" |
		jq -r '.[] | select(.name=="lightspeed-operator") | .revision'
)"
git fetch --depth=1 --filter=blob:none origin "${TEST_SOURCE_COMMIT}"
git checkout "${TEST_SOURCE_COMMIT}"

echo "---------------------------------------------"
echo "---------------------------------------------"
echo "---------------------------------------------"
export LLM_TOKEN="$(cat "${OPENAI_PROVIDER_KEY_PATH}")"
export LLM_PROVIDER="openai"
export LLM_MODEL="gpt-4o-mini"
echo "starting tests for ${LLM_PROVIDER} ${LLM_MODEL}"
make test-e2e
echo "---------------------------------------------"
echo "---------------------------------------------"
echo "---------------------------------------------"
export AZUREOPENAI_ENTRA_ID_TENANT_ID="$(cat /var/run/azureopenai-entra-id/tenant_id)"
export AZUREOPENAI_ENTRA_ID_CLIENT_ID="$(cat /var/run/azureopenai-entra-id/client_id)"
export AZUREOPENAI_ENTRA_ID_CLIENT_SECRET="$(cat /var/run/azureopenai-entra-id/client_secret)"
export LLM_TOKEN="$(cat "${AZUREOPENAI_PROVIDER_KEY_PATH}")"
export LLM_PROVIDER="azure_openai"
export LLM_MODEL="gpt-4o-mini"
echo "starting tests for ${LLM_PROVIDER} ${LLM_MODEL}"
make test-e2e

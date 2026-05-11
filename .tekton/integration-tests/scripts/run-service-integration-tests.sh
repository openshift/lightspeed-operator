#!/usr/bin/env bash
# Konflux lightspeed-service integration: run from lightspeed-operator repo root
# at ${BUNDLE_COMMIT_SHA} (Tekton performs git init/fetch/checkout before piping this script).
#
# Usage (from Tekton, cwd = lightspeed-operator repo root after checkout):
#   bash .tekton/integration-tests/scripts/run-service-integration-tests.sh "$(params.openshift-version-prefix)"
#
# Args:
#   $1  openshift-version-prefix with trailing dot (e.g. 4.19.) — same as pipeline param
#
# Env (set by Tekton step): KUBECONFIG, KONFLUX_BOOL, BUNDLE_IMAGE (SNAPSHOT containerImage for the
#   labeled Konflux component — bundle or operator image depending on scenario), BUNDLE_COMMIT_SHA,
#   ARTIFACT_DIR, OLS_IMAGE, BAM_PROVIDER_KEY_PATH, AZUREOPENAI_PROVIDER_KEY_PATH,
#   OPENAI_PROVIDER_KEY_PATH, WATSONX_PROVIDER_KEY_PATH,
#   Azure Entra paths under /var/run/azureopenai-entra-id/
#   OLS_NAMESPACE — optional; default openshift-lightspeed (matches pipeline params.namespace)

set -euo pipefail

openshift_version_prefix="${1:?usage: $0 <openshift-version-prefix e.g. 4.19.>}"
ver="${openshift_version_prefix%.}"
ocp_channel="latest-${ver}"

echo "---------------------------------------------"
echo "${KONFLUX_BOOL}"
echo "---------------------------------------------"
echo "${BUNDLE_IMAGE}"
echo "---------------------------------------------"

bash .tekton/integration-tests/scripts/install-oc-if-missing.sh "${ocp_channel}"

ols_ns="${OLS_NAMESPACE:-openshift-lightspeed}"
echo "Setting default namespace for oc (e2e harness uses commands without -n)"
oc project "${ols_ns}"

# Prior Tekton task installs the operator only (OLM bundle or direct/kustomize per pipeline).
# lightspeed-app-server is created after an OLSConfig CR is applied — that happens inside
# tests/scripts/test-e2e-cluster.sh → pytest / service installer (e.g. apply_olsconfig), not here.
# Do not wait for that deployment before running the service harness or this step will time out.

git config --global user.email olsci@redhat.com
git config --global user.name olsci

SERVICE_SHA="$(
	jq -r '.[] | select(.name=="lightspeed-service-api") | .revision' related_images.json
)"
git clone https://github.com/openshift/lightspeed-service.git
cd lightspeed-service
git fetch origin "${SERVICE_SHA}"
git checkout "${SERVICE_SHA}"
pip3.11 install --no-cache-dir --upgrade pip pdm
pdm config python.use_venv false
export CP_OFFLINE_TOKEN="$(cat /var/run/insights-stage-upload-offline-token/token)"
export AZUREOPENAI_ENTRA_ID_TENANT_ID="$(cat /var/run/azureopenai-entra-id/tenant_id)"
export AZUREOPENAI_ENTRA_ID_CLIENT_ID="$(cat /var/run/azureopenai-entra-id/client_id)"
export AZUREOPENAI_ENTRA_ID_CLIENT_SECRET="$(cat /var/run/azureopenai-entra-id/client_secret)"
tests/scripts/test-e2e-cluster.sh

#!/usr/bin/env bash
# Konflux console Cypress: run from lightspeed-operator repo root at ${COMMIT_SHA}
# (Tekton performs apt + git init/fetch/checkout before piping this script).
#
# Usage (from Tekton, cwd = /home/lightspeed-operator):
#   git show "${COMMIT_SHA}:.tekton/integration-tests/scripts/run-console-cypress-tests.sh" \
#     | bash -s -- "<related_images.json component name>" "<oc client channel e.g. latest-4.18>"
#
# Args:
#   $1  Name in related_images.json for the console plugin (e.g. lightspeed-console-plugin-pf5 or lightspeed-console-plugin)
#   $2  OpenShift client channel for install-oc-if-missing.sh (e.g. latest-4.18), aligned with the ephemeral cluster minor
#
# Env: COMMIT_SHA, CYPRESS_BASE_URL, CYPRESS_CONSOLE_IMAGE, CYPRESS_KUBECONFIG_PATH, PASSWORD_PATH, etc.

set -euo pipefail

console_component="${1:?usage: $0 <related_images component name> <ocp channel e.g. latest-4.18>}"
ocp_channel="${2:?usage: $0 <related_images name> <ocp channel>}"

echo "COMMIT_SHA: ${COMMIT_SHA}"
echo "CYPRESS_BASE_URL: ${CYPRESS_BASE_URL:-}"
echo "CYPRESS_CONSOLE_IMAGE: ${CYPRESS_CONSOLE_IMAGE:-}"
echo "---------------------------------------------"
export CYPRESS_LOGIN_PASSWORD="$(cat "${PASSWORD_PATH}")"
echo "(CYPRESS_LOGIN_PASSWORD set from PASSWORD_PATH; not echoed)"
echo "---------------------------------------------"

git show "${COMMIT_SHA}:.tekton/integration-tests/scripts/install-oc-if-missing.sh" | bash -s -- "${ocp_channel}"

echo "---------------------------------------------"
export OPERATOR_SDK_VERSION=1.36.1
case "$(uname -m)" in
x86_64) ARCH=amd64 ;;
aarch64) ARCH=arm64 ;;
*) ARCH="$(uname -m)" ;;
esac
export OPERATOR_SDK_DL_URL="https://github.com/operator-framework/operator-sdk/releases/download/v${OPERATOR_SDK_VERSION}"
wget --no-verbose -O /usr/local/bin/operator-sdk "${OPERATOR_SDK_DL_URL}/operator-sdk_linux_${ARCH}"
chmod +x /usr/local/bin/operator-sdk
echo "---------------------------------------------"
operator-sdk version
echo "---------------------------------------------"

# Valid XDG path for Cypress/Electron; must not reuse $PATH (breaks browser runtime).
XDG_RUNTIME_DIR="${HOME:-/root}/.cache/xdgr"
mkdir -p "${XDG_RUNTIME_DIR}"
export XDG_RUNTIME_DIR
echo "---------------------------------------------"

TEST_SOURCE_COMMIT="$(
	git show "${COMMIT_SHA}:related_images.json" |
		jq -r --arg n "${console_component}" '.[] | select(.name == $n) | .revision'
)"
cd /home
rm -rf lightspeed-console
git init lightspeed-console
cd lightspeed-console
git remote add origin https://github.com/openshift/lightspeed-console.git
git fetch --depth=1 --filter=blob:none origin "${TEST_SOURCE_COMMIT}"
git checkout "${TEST_SOURCE_COMMIT}"
echo "---------------------------------------------"
echo "npm version: $(npm -v)"
echo "---------------------------------------------"
NODE_OPTIONS=--max-old-space-size=4096 npm ci --omit=optional --no-fund
echo "---------------------------------------------"
export CYPRESS_LOGIN_PASSWORD="$(cat "${PASSWORD_PATH}")"
# Ephemeral clusters + console OAuth + plugin proxy are slow; before() often runs bundle then UI.
export CYPRESS_defaultCommandTimeout="${CYPRESS_defaultCommandTimeout:-120000}"
export CYPRESS_requestTimeout="${CYPRESS_requestTimeout:-120000}"
export CYPRESS_pageLoadTimeout="${CYPRESS_pageLoadTimeout:-180000}"
export CYPRESS_responseTimeout="${CYPRESS_responseTimeout:-180000}"
export CYPRESS_execTimeout="${CYPRESS_execTimeout:-600000}"

run_cypress() {
	NO_COLOR=1 npx cypress run "$@"
}

set +e
run_cypress
err_status=$?
if [[ "${err_status}" -ne 0 ]]; then
	echo "---------------------------------------------"
	echo "Cypress exited ${err_status}; waiting 30s for console/plugin then retrying once..."
	sleep 30
	run_cypress
	err_status=$?
fi
echo -n "${err_status}" >/workspace/cypress-exit-code
echo "---------------------------------------------"
ls ./gui_test_screenshots
mv ./gui_test_screenshots /workspace/artifacts/
set -e
echo "Cypress exit code: ${err_status}"
exit "${err_status}"

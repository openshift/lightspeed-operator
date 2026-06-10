#!/usr/bin/env bash
# Konflux console Playwright: run from lightspeed-operator repo root at ${COMMIT_SHA}
# (Tekton performs apt + git init/fetch/checkout before piping this script).
#
# Usage (from Tekton, cwd = /home/lightspeed-operator):
#   git show "${COMMIT_SHA}:.tekton/integration-tests/scripts/run-console-playwright-tests.sh" \
#     | bash -s -- "<related_images.json component name>" "<oc client channel e.g. latest-4.18>"
#
# Args:
#   $1  Name in related_images.json for the console plugin (e.g. lightspeed-console-plugin-pf5 or lightspeed-console-plugin)
#   $2  OpenShift client channel for install-oc-if-missing.sh (e.g. latest-4.18), aligned with the ephemeral cluster minor
#
# Env: COMMIT_SHA, BASE_URL, KUBECONFIG_PATH, PASSWORD_PATH, LOGIN_IDP, BUNDLE_IMAGE, etc.

set -euo pipefail

console_component="${1:?usage: $0 <related_images component name> <ocp channel e.g. latest-4.18>}"
ocp_channel="${2:?usage: $0 <related_images name> <ocp channel>}"

echo "COMMIT_SHA: ${COMMIT_SHA}"
echo "BASE_URL: ${BASE_URL:-}"
echo "CONSOLE_IMAGE: ${CONSOLE_IMAGE:-}"
echo "---------------------------------------------"
if [[ ! -r "${PASSWORD_PATH}" ]]; then
	echo "ERROR: PASSWORD_PATH '${PASSWORD_PATH}' is not readable" >&2
	exit 1
fi
LOGIN_PASSWORD="$(cat "${PASSWORD_PATH}")"
export LOGIN_PASSWORD
echo "(LOGIN_PASSWORD set from PASSWORD_PATH; not echoed)"
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

# Valid XDG path for Playwright/Chromium; must not reuse $PATH (breaks browser runtime).
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
npx cypress install
echo "---------------------------------------------"

# Install Playwright browsers (chromium only, with OS deps).
npx playwright install --with-deps chromium
echo "---------------------------------------------"

# Enable Playwright CI mode (forbidOnly, etc.).
export CI=true

run_playwright() {
	npx playwright test "$@"
}

set +e
run_playwright
err_status=$?
if [[ "${err_status}" -ne 0 ]]; then
	echo "---------------------------------------------"
	echo "Playwright exited ${err_status}; waiting 30s for console/plugin then retrying once..."
	sleep 30
	run_playwright
	err_status=$?
fi
echo -n "${err_status}" >/workspace/cypress-exit-code
echo "---------------------------------------------"
ls ./gui_test_screenshots
mv ./gui_test_screenshots /workspace/artifacts/
set -e
echo "Playwright exit code: ${err_status}"
exit "${err_status}"

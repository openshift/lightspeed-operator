#!/usr/bin/env bash
# Install OpenShift client (oc) when it is not already on PATH.
# Used by Konflux Tekton steps that may run on minimal images or run pytest
# harnesses that shell out to "oc".
#
# Tekton (after git fetch of the bundle commit, from repo root of clone):
#   git show "${COMMIT_SHA}:.tekton/integration-tests/scripts/install-oc-if-missing.sh" | bash -s -- "latest-4.17"
#
# Usage: install-oc-if-missing.sh <ocp-client-channel>
# Example: install-oc-if-missing.sh latest-4.17
#
# Channel must match mirror layout, see:
# https://mirror.openshift.com/pub/openshift-v4/<arch>/clients/ocp/<channel>/

set -euo pipefail

channel="${1:?usage: $0 <e.g. latest-4.17>}"

if command -v oc >/dev/null 2>&1; then
	oc version --client
	exit 0
fi

arch=$(uname -m)
case "${arch}" in
x86_64) ocp_arch=amd64 ;;
aarch64) ocp_arch=arm64 ;;
*) ocp_arch="${arch}" ;;
esac

work=$(mktemp -d)
cleanup() {
	rm -rf "${work}"
}
trap cleanup EXIT

curl -fSL -o "${work}/oc.tgz" \
	"https://mirror.openshift.com/pub/openshift-v4/${ocp_arch}/clients/ocp/${channel}/openshift-client-linux-${ocp_arch}-rhel9.tar.gz"
tar -C "${work}" -xzf "${work}/oc.tgz" oc kubectl
cp -f "${work}/oc" "${work}/kubectl" /usr/local/bin/
chmod 0755 /usr/local/bin/oc /usr/local/bin/kubectl
oc version --client

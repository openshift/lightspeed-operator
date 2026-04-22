#!/usr/bin/env bash
# Shared helpers for hack/install/*.sh (source from those scripts, do not execute directly).

# Namespace used by OLS operator E2E and default deploy manifests.
install::default_namespace() {
	echo "${OLS_NAMESPACE:-openshift-lightspeed}"
}

install::require_cmd() {
	local c="$1"
	if ! command -v "$c" >/dev/null 2>&1; then
		echo "error: required command not found: $c" >&2
		exit 1
	fi
}

install::ensure_oc_kubectl() {
	install::require_cmd oc
	# kubectl is optional if oc kubectl works; Makefile uses $(KUBECTL) defaulting to kubectl
	if ! command -v kubectl >/dev/null 2>&1; then
		echo "warn: kubectl not in PATH; ensure Makefile KUBECTL/oc wrapper is satisfied" >&2
	fi
}

install::ensure_namespace() {
	local ns="$1"
	oc create namespace "$ns" --dry-run=client -o yaml | oc apply -f -
	oc label namespaces "$ns" openshift.io/cluster-monitoring=true --overwrite=true 2>/dev/null || true
}

# OLM gives CSV InstallCheckFailed / "install timeout" (~5m) if the CSV deployment never becomes
# Available. This does not follow operator-sdk --timeout. Dump state for ImagePull / probe / RBAC.
install::olm_failure_diagnostics() {
	local ns="${1:-}"
	if [[ -z "$ns" ]]; then
		return 0
	fi
	echo "---- OLM / operator install diagnostics (namespace=${ns}) ----" >&2
	oc get csv -n "$ns" -o wide 2>/dev/null || true
	oc get subscription,installplan,catalogsource -n "$ns" -o wide 2>/dev/null || true
	oc get deployment,pods -n "$ns" -o wide 2>/dev/null || true
	for d in $(oc get deploy -n "$ns" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
		[[ -n "$d" ]] || continue
		echo "---- oc describe deployment/${d} -n ${ns} ----" >&2
		oc describe "deployment/${d}" -n "$ns" 2>/dev/null || true
	done
	for p in $(oc get pods -n "$ns" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
		[[ -n "$p" ]] || continue
		echo "---- oc describe pod/${p} -n ${ns} ----" >&2
		oc describe "pod/${p}" -n "$ns" 2>/dev/null || true
	done
	echo "---- recent events -n ${ns} ----" >&2
	oc get events -n "$ns" --sort-by=.metadata.creationTimestamp 2>/dev/null | tail -80 || true
	echo "---- end diagnostics ----" >&2
}

# Install operator-sdk to PATH if missing (matches Konflux e2e pipelines).
install::ensure_operator_sdk() {
	local ver="${OPERATOR_SDK_VERSION:-1.36.1}"
	if command -v operator-sdk >/dev/null 2>&1; then
		return 0
	fi
	install::require_cmd curl
	local arch
	case "$(uname -m)" in
	x86_64) arch=amd64 ;;
	aarch64) arch=arm64 ;;
	*) arch="$(uname -m)" ;;
	esac
	local url="https://github.com/operator-framework/operator-sdk/releases/download/v${ver}/operator-sdk_linux_${arch}"
	local dest="${OPERATOR_SDK_BIN:-/usr/local/bin/operator-sdk}"
	if [[ ! -w "$(dirname "$dest")" ]]; then
		echo "error: operator-sdk not in PATH and cannot write to $(dirname "$dest"); set OPERATOR_SDK_BIN to a writable path" >&2
		exit 1
	fi
	echo "installing operator-sdk v${ver} to ${dest}..."
	curl -fsSL -o "$dest" "$url"
	chmod +x "$dest"
	export PATH="$(dirname "$dest"):${PATH}"
}

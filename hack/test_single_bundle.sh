#!/usr/bin/env bash
# Validate the generated single-bundle contract without an OpenShift cluster.
set -euo pipefail
csv=bundle/manifests/lightspeed-operator.clusterserviceversion.yaml
expected=$(jq -r '.[] | select(.snapshot_source != "bundle") | .name + "=" + .image' related_images.json | sort)
actual=$(yq -o=json '.spec.relatedImages' "$csv" | jq -r '.[] | .name + "=" + .image' | sort)
if [[ "$expected" != "$actual" ]]; then
  echo 'bundle relatedImages differ from related_images.json' >&2
  diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || :
  exit 1
fi
csv_version=$(yq -r '.spec.version' "$csv")
dockerfile_version=$(grep '^LABEL version=' bundle.Dockerfile | cut -d= -f2-)
if [[ "$csv_version" != "$dockerfile_version" ]]; then
  echo "bundle CSV version $csv_version differs from Dockerfile label $dockerfile_version" >&2
  exit 1
fi
if [[ $(yq -r '.spec.install.spec.deployments | length' "$csv") != 2 ]]; then
  echo 'single bundle must install two controller deployments' >&2
  exit 1
fi
agentic=$(jq -r '.[] | select(.name == "lightspeed-agentic-operator") | .image' related_images.json)
if [[ $(yq -r '.spec.install.spec.deployments[] | select(.name == "lightspeed-agentic-operator-controller-manager") | .spec.template.spec.containers[0].image' "$csv") != "$agentic" ]]; then
  echo 'agentic controller digest differs from related_images.json' >&2
  exit 1
fi
if ! yq -o=json '.spec.install.spec.clusterPermissions' "$csv" | jq -e '
  any(.[]; .serviceAccountName == "lightspeed-agentic-operator-controller-manager" and
    any(.rules[]; (.apiGroups | index("networking.k8s.io")) != null and
      (.resources | index("networkpolicies")) != null and
      (.verbs | index("create")) != null))
' >/dev/null; then
  echo 'agentic controller lacks runtime webhook NetworkPolicy permissions' >&2
  exit 1
fi
if grep -q '__REPLACE_\|lightspeed-agentic-operator:latest' "$csv"; then
  echo 'bundle has unsubstituted image placeholders' >&2
  exit 1
fi
if [[ $(yq -r '.annotations."com.redhat.openshift.versions"' bundle/metadata/annotations.yaml) != '>=v4.16' ]]; then
  echo 'bundle annotation must cover both 4.x and 5.0' >&2
  exit 1
fi
echo 'single bundle image and deployment contract passed'

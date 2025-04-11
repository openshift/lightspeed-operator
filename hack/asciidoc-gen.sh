#!/bin/sh

set -e

mkdir -p _tmp

oc get --raw /openapi/v2 | jq . > _tmp/openapi.1.json

jq 'del(.definitions."io.openshift.ols.v1alpha1.OLSConfig".properties.status)
    | del(.definitions."io.openshift.ols.v1alpha1.OLSConfig".properties.metadata."$ref")
    | .definitions."io.openshift.ols.v1alpha1.OLSConfig".properties.metadata += {type:"object"}' \
   _tmp/openapi.1.json > _tmp/openapi.2.json

openshift-apidocs-gen build -c hack/asciidoc-gen-config.yaml _tmp/openapi.2.json


amend_doc() {
  local filename=$1

  mv _tmp/ols_openshift_io/$filename docs/$filename

  sed -i -r 's/^:_content-type: ASSEMBLY$/:_mod-docs-content-type: REFERENCE/' docs/$filename
  sed -i -r 's/^\[id="olsconfig-ols-openshift-io-v1alpha1"\]$/[id="openshift-lightspeed-olsconfig-api-specifications_{context}"]/' docs/$filename
  sed -i -r 's/= OLSConfig \[ols.openshift.io.*/= OLSConfig API specifications/' docs/$filename
  sed -i -r '/^:toc: macro$/d ' docs/$filename
  sed -i -r '/^:toc-title:$/d ' docs/$filename
  sed -i -r '/^toc::\[\]$/d ' docs/$filename
  sed -i -r '/^== Specification$/d ' docs/$filename
  sed -i -r 's/^==/=/g' docs/$filename
  sed -i -r '/^= API endpoints/Q' docs/$filename
  sed -i -r 's/OpenShift/{product-title}/g' docs/$filename
  sed -i -r 's/<br>/ +\n/g' docs/$filename
  sed -i -r 's/<i>/_/g' docs/$filename
  sed -i -r 's/<\/i>/_/g' docs/$filename
  sed -i -r 's/ may / might /g' docs/$filename
  # Our asciidoc gen doesn't handle arrays very well, producing duplicate fields... so remove one of them
  sed -i -r '/^\| `.+\[\]`$/,+3d' docs/$filename
}

amend_doc "olsconfig-ols-openshift-io-v1alpha1.adoc"

#!/usr/bin/env bash
# Deploy Jaeger all-in-one with TLS on OpenShift using service-ca certificates.
# The OTEL Collector's otlp/tracing exporter can then send traces securely.
#
# Usage: hack/deploy-jaeger-tls.sh [NAMESPACE]
#   NAMESPACE defaults to "observability"
#
# To remove: hack/deploy-jaeger-tls.sh --delete [NAMESPACE]

set -euo pipefail

NAMESPACE="${2:-${1:-observability}}"
JAEGER_IMAGE="quay.io/jaegertracing/jaeger:latest"

if [[ "${1:-}" == "--delete" ]]; then
  echo "Removing Jaeger from namespace ${NAMESPACE}..."
  oc delete route jaeger-query -n "${NAMESPACE}" --ignore-not-found
  oc delete service jaeger-query jaeger-otlp-grpc -n "${NAMESPACE}" --ignore-not-found
  oc delete deployment jaeger -n "${NAMESPACE}" --ignore-not-found
  oc delete configmap jaeger-config -n "${NAMESPACE}" --ignore-not-found
  oc delete serviceaccount jaeger -n "${NAMESPACE}" --ignore-not-found
  echo "Done. Namespace ${NAMESPACE} left in place."
  exit 0
fi

echo "Deploying Jaeger all-in-one with TLS in namespace: ${NAMESPACE}"

# Create namespace if it doesn't exist
oc get namespace "${NAMESPACE}" &>/dev/null || oc create namespace "${NAMESPACE}"

# ServiceAccount
oc apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: jaeger
  namespace: ${NAMESPACE}
EOF

# Service with service-ca annotation — OpenShift will auto-generate TLS certs
# in a Secret named jaeger-otlp-tls, signed by the cluster's service-ca.
oc apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: jaeger-otlp-grpc
  namespace: ${NAMESPACE}
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: jaeger-otlp-tls
  labels:
    app: jaeger
spec:
  selector:
    app: jaeger
  ports:
    - name: otlp-grpc
      port: 4317
      targetPort: 4317
      protocol: TCP
EOF

# Jaeger query UI service
oc apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: jaeger-query
  namespace: ${NAMESPACE}
  labels:
    app: jaeger
spec:
  selector:
    app: jaeger
  ports:
    - name: query-http
      port: 16686
      targetPort: 16686
      protocol: TCP
EOF

# Wait for the service-ca to generate the TLS secret
echo "Waiting for service-ca to issue TLS certificate..."
for i in $(seq 1 30); do
  if oc get secret jaeger-otlp-tls -n "${NAMESPACE}" &>/dev/null; then
    echo "TLS secret jaeger-otlp-tls is ready."
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "ERROR: Timed out waiting for jaeger-otlp-tls secret. Is the service-ca operator running?"
    exit 1
  fi
  sleep 2
done

# Jaeger v2 config — uses OTEL Collector format internally
oc apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: jaeger-config
  namespace: ${NAMESPACE}
  labels:
    app: jaeger
data:
  config.yaml: |
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: "0.0.0.0:4317"
            tls:
              cert_file: /etc/tls/tls.crt
              key_file: /etc/tls/tls.key
    exporters:
      jaeger_storage_exporter:
        trace_storage: memstore
    extensions:
      jaeger_storage:
        backends:
          memstore:
            memory:
              max_traces: 100000
      jaeger_query:
        http:
          endpoint: "0.0.0.0:16686"
        grpc:
          endpoint: "0.0.0.0:16685"
        storage:
          traces: memstore
    service:
      extensions: [jaeger_storage, jaeger_query]
      pipelines:
        traces:
          receivers: [otlp]
          exporters: [jaeger_storage_exporter]
EOF

# Deployment — Jaeger v2 with TLS on the OTLP gRPC collector endpoint
oc apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jaeger
  namespace: ${NAMESPACE}
  labels:
    app: jaeger
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jaeger
  template:
    metadata:
      labels:
        app: jaeger
    spec:
      serviceAccountName: jaeger
      containers:
        - name: jaeger
          image: ${JAEGER_IMAGE}
          args:
            - --config=/etc/jaeger/config.yaml
          ports:
            - name: otlp-grpc
              containerPort: 4317
            - name: query-http
              containerPort: 16686
          volumeMounts:
            - name: tls-certs
              mountPath: /etc/tls
              readOnly: true
            - name: jaeger-config
              mountPath: /etc/jaeger
              readOnly: true
          readinessProbe:
            httpGet:
              path: /
              port: 16686
            initialDelaySeconds: 5
            periodSeconds: 10
      volumes:
        - name: tls-certs
          secret:
            secretName: jaeger-otlp-tls
        - name: jaeger-config
          configMap:
            name: jaeger-config
EOF

# Route for the Jaeger UI
oc apply -f - <<EOF
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: jaeger-query
  namespace: ${NAMESPACE}
  labels:
    app: jaeger
spec:
  to:
    kind: Service
    name: jaeger-query
  port:
    targetPort: query-http
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
EOF

# Wait for rollout
echo "Waiting for Jaeger deployment to be ready..."
oc rollout status deployment/jaeger -n "${NAMESPACE}" --timeout=120s

# Print connection info
ROUTE_HOST=$(oc get route jaeger-query -n "${NAMESPACE}" -o jsonpath='{.spec.host}')

cat <<DONE

========================================
  Jaeger deployed successfully!
========================================

OTLP gRPC endpoint (for OLSConfig CR):
  tracingEndpoint: "jaeger-otlp-grpc.${NAMESPACE}.svc.cluster.local:4317"

Jaeger UI:
  https://${ROUTE_HOST}

Example OLSConfig patch:
  oc patch olsconfig cluster --type=merge -p '{"spec":{"audit":{"tracingEndpoint":"jaeger-otlp-grpc.${NAMESPACE}.svc.cluster.local:4317"}}}'

To remove:
  hack/deploy-jaeger-tls.sh --delete ${NAMESPACE}

DONE

#!/bin/bash

# Define variables
CERT_DIR="./certs"
CA_KEY="$CERT_DIR/ca.key"
CA_CERT="$CERT_DIR/ca.crt"
PRIVATE_KEY="$CERT_DIR/tls.key"
CERTIFICATE="$CERT_DIR/tls.crt"
DAYS_VALID=365
SECRET_NAME="lightspeed-tls"
NAMESPACE="openshift-lightspeed"

# Create directory for certificates if it doesn't exist
mkdir -p "$CERT_DIR"

# Generate CA private key and self-signed CA certificate
openssl req -x509 -newkey rsa:4096 -sha256 -days "$DAYS_VALID" -nodes \
    -keyout "$CA_KEY" -out "$CA_CERT" -subj "/CN=MyCA" \
    -addext "subjectAltName=DNS:MyCA"

echo "CA certificate and private key have been generated in $CERT_DIR"

# Generate private key and certificate signing request (CSR) for the server
openssl req -new -newkey rsa:4096 -nodes -keyout "$PRIVATE_KEY" -out "$CERT_DIR/server.csr" \
    -subj "/CN=lightspeed-app-server" -addext "subjectAltName=DNS:lightspeed-app-server,DNS:lightspeed-app-server.openshift-lightspeed.svc.cluster.local,IP:127.0.0.1,IP:::1"

# Sign the server certificate with the CA certificate
openssl x509 -req -in "$CERT_DIR/server.csr" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
    -out "$CERTIFICATE" -days "$DAYS_VALID" -sha256 -extfile <(echo "subjectAltName=DNS:lightspeed-app-server,DNS:lightspeed-app-server.openshift-lightspeed.svc.cluster.local,IP:127.0.0.1,IP:::1")

echo "Server certificate signed by CA has been generated in $CERT_DIR"

# Generate the Kubernetes Secret YAML manifest for the TLS certificate and key for the ols-server
cat <<EOF > "$CERT_DIR/$SECRET_NAME.yaml"
apiVersion: v1
kind: Secret
metadata:
  name: $SECRET_NAME
  namespace: $NAMESPACE
type: kubernetes.io/tls
data:
  tls.crt: $(base64 < "$CERTIFICATE")
  tls.key: $(base64 < "$PRIVATE_KEY")
EOF

echo "Kubernetes Secret manifest for TLS has been generated at $CERT_DIR/$SECRET_NAME.yaml"
#!/bin/bash

# Create a Private Certificate Authority for IP-based certificates
# This is suitable for production environments with controlled clients

set -e

CA_DIR="./ca"
CERTS_DIR="./certs"

# Create directories
mkdir -p "$CA_DIR" "$CERTS_DIR"

echo "🏢 Creating Private Certificate Authority..."

# Generate CA private key
openssl genrsa -out "$CA_DIR/ca.key" 4096

# Generate CA certificate
openssl req -new -x509 -days 3650 -key "$CA_DIR/ca.key" -out "$CA_DIR/ca.crt" \
    -subj "/C=US/ST=CA/L=San Francisco/O=Arrakis CA/CN=Arrakis Root CA"

# Get the VM's internal IP address
INTERNAL_IP=$(hostname -I | awk '{print $1}')
EXTERNAL_IP="35.211.173.121"
echo "🌐 Internal IP detected: $INTERNAL_IP"
echo "🌐 External IP configured: $EXTERNAL_IP"

# Generate server private key
openssl genrsa -out "$CERTS_DIR/server.key" 2048

# Generate certificate signing request
openssl req -new -key "$CERTS_DIR/server.key" -out "$CERTS_DIR/server.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=Arrakis/CN=$EXTERNAL_IP"

# Generate server certificate signed by our CA with IP SAN
openssl x509 -req -days 365 -in "$CERTS_DIR/server.csr" \
    -CA "$CA_DIR/ca.crt" -CAkey "$CA_DIR/ca.key" -CAcreateserial \
    -out "$CERTS_DIR/server.crt" \
    -extensions v3_req -extfile <(
    echo '[v3_req]'
    echo 'basicConstraints = CA:FALSE'
    echo 'keyUsage = nonRepudiation, digitalSignature, keyEncipherment'
    echo 'subjectAltName = @alt_names'
    echo '[alt_names]'
    echo "IP.1 = $EXTERNAL_IP"
    echo "IP.2 = $INTERNAL_IP"
    echo 'IP.3 = 127.0.0.1'
    echo 'DNS.1 = localhost'
)

# Clean up
rm "$CERTS_DIR/server.csr"

# Set permissions
chmod 600 "$CERTS_DIR/server.key" "$CA_DIR/ca.key"
chmod 644 "$CERTS_DIR/server.crt" "$CA_DIR/ca.crt"

echo "✅ Private CA and IP-based certificate created!"
echo ""
echo "📋 Files created:"
echo "  CA Certificate: $CA_DIR/ca.crt"
echo "  CA Private Key: $CA_DIR/ca.key"
echo "  Server Certificate: $CERTS_DIR/server.crt"
echo "  Server Private Key: $CERTS_DIR/server.key"
echo ""
echo "🔧 To trust on client machines:"
echo "  Linux: sudo cp $CA_DIR/ca.crt /usr/local/share/ca-certificates/arrakis-ca.crt && sudo update-ca-certificates"
echo "  Browser: Import $CA_DIR/ca.crt as a trusted Certificate Authority"
echo ""
echo "🌐 Your services will be accessible at:"
echo "  External: https://$EXTERNAL_IP:PORT"
echo "  Internal: https://$INTERNAL_IP:PORT"

#!/bin/bash

# Script to generate self-signed SSL certificates for Arrakis services

set -e

CERT_DIR="./certs"
CERT_FILE="$CERT_DIR/server.crt"
KEY_FILE="$CERT_DIR/server.key"

# Create certs directory if it doesn't exist
mkdir -p "$CERT_DIR"

# Generate private key
echo "Generating private key..."
openssl genrsa -out "$KEY_FILE" 2048

# Generate certificate signing request
echo "Generating certificate signing request..."
openssl req -new -key "$KEY_FILE" -out "$CERT_DIR/server.csr" -subj "/C=US/ST=CA/L=San Francisco/O=Arrakis/CN=localhost"

# Generate self-signed certificate
echo "Generating self-signed certificate..."
openssl x509 -req -days 365 -in "$CERT_DIR/server.csr" -signkey "$KEY_FILE" -out "$CERT_FILE" \
    -extensions v3_req -extfile <(
    echo '[v3_req]'
    echo 'basicConstraints = CA:FALSE'
    echo 'keyUsage = nonRepudiation, digitalSignature, keyEncipherment'
    echo 'subjectAltName = @alt_names'
    echo '[alt_names]'
    echo 'DNS.1 = localhost'
    echo 'DNS.2 = *.localhost'
    echo 'IP.1 = 127.0.0.1'
    echo 'IP.2 = ::1'
)

# Clean up CSR file
rm "$CERT_DIR/server.csr"

# Set appropriate permissions
chmod 600 "$KEY_FILE"
chmod 644 "$CERT_FILE"

echo "SSL certificates generated successfully!"
echo "Certificate: $CERT_FILE"
echo "Private Key: $KEY_FILE"
echo ""
echo "To trust the certificate (optional):"
echo "  sudo cp $CERT_FILE /usr/local/share/ca-certificates/arrakis.crt"
echo "  sudo update-ca-certificates"
echo ""
echo "Note: This is a self-signed certificate. Browsers will show security warnings."
echo "For production use, obtain certificates from a trusted CA."

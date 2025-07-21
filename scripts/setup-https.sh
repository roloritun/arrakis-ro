#!/bin/bash

# Arrakis HTTPS Setup Script
# This script generates SSL certificates and rebuilds the services with HTTPS support

set -e

echo "🔒 Setting up HTTPS for Arrakis services..."

# Generate SSL certificates
echo "📜 Generating SSL certificates..."
./scripts/generate-certs.sh

echo "🔧 Building Arrakis services with HTTPS support..."
make clean
make all

echo "✅ HTTPS setup complete!"
echo ""
echo "📋 What was configured:"
echo "  ✓ Generated self-signed SSL certificates in ./certs/"
echo "  ✓ Updated configuration to enable TLS for all services"
echo "  ✓ REST Server: HTTPS on port 7000"
echo "  ✓ NoVNC Server: HTTPS on port 6080"
echo "  ✓ CDP Server: HTTPS on port 9222"
echo "  ✓ Client: configured to use HTTPS"
echo ""
echo "🚀 To start the services:"
echo "  ./out/arrakis-restserver"
echo ""
echo "🌐 Access URLs (HTTPS):"
echo "  REST API: https://localhost:7000/v1/health"
echo "  NoVNC: https://localhost:6080 (when VM is running)"
echo "  CDP: https://localhost:9222/json (when VM is running)"
echo ""
echo "⚠️  Note: Your browser will show security warnings for self-signed certificates."
echo "   You can safely proceed or add the certificate to your trusted store."
echo ""
echo "🔓 To disable HTTPS, set 'enabled: false' in config.yaml under tls sections."

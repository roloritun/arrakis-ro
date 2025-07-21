#!/bin/bash

# NoVNC Installation Script for Arrakis
# This script installs NoVNC web client to fix missing menu icons

set -e

echo "🖥️  Installing NoVNC for Arrakis..."

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then
    echo "❌ This script needs to be run with sudo privileges"
    echo "Usage: sudo $0"
    exit 1
fi

# Install Git if not already installed
echo "📦 Installing Git..."
apt update
apt install -y git

# Create /opt directory if it doesn't exist
echo "📁 Creating /opt directory..."
mkdir -p /opt

# Remove existing NoVNC if present
if [ -d "/opt/novnc" ]; then
    echo "🗑️  Removing existing NoVNC installation..."
    rm -rf /opt/novnc
fi

# Clone NoVNC repository
echo "📥 Cloning NoVNC repository..."
cd /opt
git clone https://github.com/novnc/noVNC.git novnc

# Create index.html symlink
echo "🔗 Creating index.html symlink..."
cd /opt/novnc
ln -sf vnc.html index.html

# Set proper permissions
echo "🔒 Setting proper permissions..."
chown -R root:root /opt/novnc
chmod -R 755 /opt/novnc

# Verify installation
echo "✅ Verifying NoVNC installation..."
if [ -f "/opt/novnc/vnc.html" ] && [ -f "/opt/novnc/index.html" ]; then
    echo "✅ NoVNC installed successfully!"
    echo ""
    echo "📋 Installation Details:"
    echo "  📂 Location: /opt/novnc"
    echo "  🔗 Main file: /opt/novnc/vnc.html"
    echo "  🔗 Index link: /opt/novnc/index.html"
    echo ""
    echo "🚀 NoVNC is now ready for use with Arrakis!"
    echo "   The menu icons and UI elements should now display correctly."
    echo ""
    echo "🔄 Next steps:"
    echo "   1. Build and start your Arrakis services"
    echo "   2. Access NoVNC web interface via your VM"
    echo "   3. Menu icons should now be visible"
else
    echo "❌ NoVNC installation failed!"
    exit 1
fi

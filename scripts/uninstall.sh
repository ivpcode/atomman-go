#!/usr/bin/env bash
# Remove the AtomMan Go daemon and all installed files.
set -e

SERVICE="atomman-go"
INSTALL_DIR="/opt/atomman-go"

echo "==> Stopping and disabling service..."
sudo systemctl stop ${SERVICE} 2>/dev/null || true
sudo systemctl disable ${SERVICE} 2>/dev/null || true

echo "==> Removing service file..."
sudo rm -f /etc/systemd/system/${SERVICE}.service
sudo systemctl daemon-reload

echo "==> Removing installed files..."
sudo rm -rf "$INSTALL_DIR"

echo ""
echo "✓ AtomMan Go uninstalled."
echo "  The serial group membership (dialout) was NOT removed."

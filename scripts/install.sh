#!/usr/bin/env bash
# Install the AtomMan Go daemon as a systemd service.
set -e

BINARY="atomman"
INSTALL_DIR="/opt/atomman-go"
SERVICE="atomman-go"
USER="sysadmin"

# Source root is one level up from this script
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> Building..."
cd "$REPO_ROOT"
GO_BIN=$(command -v go 2>/dev/null || echo "$HOME/go/bin/go")
"$GO_BIN" build -o "$BINARY" .

echo "==> Installing to $INSTALL_DIR..."
sudo mkdir -p "$INSTALL_DIR"
sudo cp "$BINARY" "$INSTALL_DIR/"
sudo cp config.yaml "$INSTALL_DIR/"

echo "==> Adding $USER to dialout group (serial access)..."
sudo usermod -aG dialout "$USER"

echo "==> Creating systemd service..."
sudo tee /etc/systemd/system/${SERVICE}.service > /dev/null <<EOF
[Unit]
Description=AtomMan Display Daemon (Go)
After=network.target

[Service]
ExecStart=${INSTALL_DIR}/${BINARY} -config ${INSTALL_DIR}/config.yaml
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=5
User=${USER}
Group=dialout

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable ${SERVICE}
sudo systemctl restart ${SERVICE}

echo ""
echo "✓ Installed. Status:"
sudo systemctl status ${SERVICE} --no-pager

echo ""
echo "Useful commands:"
echo "  sudo systemctl status ${SERVICE}      # status"
echo "  journalctl -u ${SERVICE} -f           # live logs"
echo "  sudo systemctl stop ${SERVICE}        # stop"
echo "  sudo systemctl restart ${SERVICE}     # restart"
echo "  sudo nano ${INSTALL_DIR}/config.yaml  # edit config"

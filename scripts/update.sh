#!/usr/bin/env bash
# Rebuild the binary and redeploy without touching the config.
set -e

BINARY="atomman"
INSTALL_DIR="/opt/atomman-go"
SERVICE="atomman-go"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> Building..."
cd "$REPO_ROOT"
GO_BIN=$(command -v go 2>/dev/null || echo "$HOME/go/bin/go")
"$GO_BIN" build -o "$BINARY" .

echo "==> Deploying binary..."
sudo cp "$BINARY" "$INSTALL_DIR/"

echo "==> Restarting service..."
sudo systemctl restart ${SERVICE}

echo ""
echo "✓ Updated. Status:"
sudo systemctl status ${SERVICE} --no-pager | head -8

#!/usr/bin/env bash
# Installa il daemon AtomMan Go e lo configura come servizio systemd.
set -e

BINARY="atomman"
INSTALL_DIR="/opt/atomman-go"
SERVICE="atomman-go"
USER="sysadmin"

echo "==> Build..."
cd "$(dirname "$0")"
# Cerca Go nell'ordine: PATH, ~/go/bin, /usr/local/go/bin
GO_BIN=$(command -v go 2>/dev/null || echo "$HOME/go/bin/go")
"$GO_BIN" build -o "$BINARY" .

echo "==> Installazione in $INSTALL_DIR..."
sudo mkdir -p "$INSTALL_DIR"
sudo cp "$BINARY" "$INSTALL_DIR/"
sudo cp config.yaml "$INSTALL_DIR/"

echo "==> Aggiunta utente $USER al gruppo dialout (accesso seriale)..."
sudo usermod -aG dialout "$USER"

echo "==> Creazione servizio systemd..."
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
echo "✓ Installato. Stato:"
sudo systemctl status ${SERVICE} --no-pager

echo ""
echo "Comandi utili:"
echo "  sudo systemctl status ${SERVICE}    # stato"
echo "  journalctl -u ${SERVICE} -f         # log live"
echo "  sudo nano ${INSTALL_DIR}/config.yaml # modifica config"
echo "  sudo systemctl restart ${SERVICE}   # applica modifiche"

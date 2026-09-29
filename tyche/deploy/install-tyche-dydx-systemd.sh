#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
ROOT="${TYCHE_ROOT:-$default_root}"
CONFIG="${TYCHE_CONFIG:-$ROOT/tyche.toml}"
LISTEN="${TYCHE_LISTEN:-127.0.0.1:8081}"
SERVICE_USER="${TYCHE_SERVICE_USER:-${SUDO_USER:-$(id -un)}}"
START_NOW="${TYCHE_START_NOW:-0}"
RENDER_CONFIG="${TYCHE_RENDER_CONFIG:-1}"
SERVICE_FILE="/etc/systemd/system/tyche-dydx.service"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run with sudo or as root, for example:" >&2
  echo "  sudo TYCHE_SERVICE_USER=\$(id -un) bash deploy/install-tyche-dydx-systemd.sh" >&2
  exit 1
fi

if [[ ! -d "$ROOT" ]]; then
  echo "missing Tyche root: $ROOT" >&2
  exit 1
fi

if [[ "$SERVICE_USER" == "root" && -n "${SUDO_USER:-}" ]]; then
  SERVICE_USER="$SUDO_USER"
fi

cd "$ROOT"
mkdir -p "$ROOT/logs"

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  echo "service user does not exist: $SERVICE_USER" >&2
  exit 1
fi

chmod +x "$ROOT/deploy/render-tyche-dydx-config.sh" "$ROOT/deploy/start-tyche-dydx.sh" "$ROOT/deploy/build-tyche.sh"

if [[ "$RENDER_CONFIG" == "1" ]]; then
  TYCHE_ROOT="$ROOT" TYCHE_CONFIG="$CONFIG" TYCHE_LISTEN="$LISTEN" "$ROOT/deploy/render-tyche-dydx-config.sh"
elif [[ ! -f "$CONFIG" ]]; then
  echo "missing Tyche config with TYCHE_RENDER_CONFIG=0: $CONFIG" >&2
  exit 1
fi

TYCHE_ROOT="$ROOT" TYCHE_GO_BIN="${TYCHE_GO_BIN:-}" "$ROOT/deploy/build-tyche.sh"

chown -R "$SERVICE_USER":"$SERVICE_USER" "$ROOT"

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Tyche dYdX trading service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$ROOT
Environment=PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
Environment=TYCHE_ROOT=$ROOT
Environment=TYCHE_CONFIG=$CONFIG
ExecStart=/usr/bin/env bash $ROOT/deploy/start-tyche-dydx.sh
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable tyche-dydx.service

if [[ "$START_NOW" == "1" ]]; then
  systemctl restart tyche-dydx.service
fi

echo "installed $SERVICE_FILE"
echo "service user: $SERVICE_USER"
echo "listen: $LISTEN"
echo "config: $CONFIG"
echo "start now: systemctl restart tyche-dydx.service"

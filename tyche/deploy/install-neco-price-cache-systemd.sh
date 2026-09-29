#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
ROOT="${TYCHE_ROOT:-$default_root}"
CONFIG="${TYCHE_CONFIG:-$ROOT/tyche.toml}"
SERVICE_USER="${TYCHE_SERVICE_USER:-${SUDO_USER:-$(id -un)}}"
START_NOW="${NECO_PRICE_CACHE_START_NOW:-0}"
SERVICE_FILE="/etc/systemd/system/neco-price-cache.service"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run with sudo or as root, for example:" >&2
  echo "  sudo TYCHE_SERVICE_USER=\$(id -un) bash deploy/install-neco-price-cache-systemd.sh" >&2
  exit 1
fi

if [[ ! -d "$ROOT" ]]; then
  echo "missing Tyche root: $ROOT" >&2
  exit 1
fi

if [[ "$SERVICE_USER" == "root" && -n "${SUDO_USER:-}" ]]; then
  SERVICE_USER="$SUDO_USER"
fi

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  echo "service user does not exist: $SERVICE_USER" >&2
  exit 1
fi

cd "$ROOT"
chmod +x "$ROOT/deploy/build-neco-price-cache.sh" "$ROOT/deploy/start-neco-price-cache.sh"
TYCHE_ROOT="$ROOT" TYCHE_GO_BIN="${TYCHE_GO_BIN:-}" "$ROOT/deploy/build-neco-price-cache.sh"
chown -R "$SERVICE_USER":"$SERVICE_USER" "$ROOT"

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=NECO swap price cache worker
After=network-online.target redis.service
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$ROOT
Environment=PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
Environment=TYCHE_ROOT=$ROOT
Environment=TYCHE_CONFIG=$CONFIG
Environment=NECO_PRICE_CACHE_INTERVAL=${NECO_PRICE_CACHE_INTERVAL:-1s}
Environment=NECO_PRICE_CACHE_TTL=${NECO_PRICE_CACHE_TTL:-30s}
Environment=NECO_PRICE_CACHE_ASSETS=${NECO_PRICE_CACHE_ASSETS:-}
ExecStart=/usr/bin/env bash $ROOT/deploy/start-neco-price-cache.sh
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable neco-price-cache.service

if [[ "$START_NOW" == "1" ]]; then
  systemctl restart neco-price-cache.service
fi

echo "installed $SERVICE_FILE"
echo "service user: $SERVICE_USER"
echo "config: $CONFIG"
echo "interval: ${NECO_PRICE_CACHE_INTERVAL:-1s}"
echo "ttl: ${NECO_PRICE_CACHE_TTL:-30s}"
echo "assets: ${NECO_PRICE_CACHE_ASSETS:-auto from tyche.toml}"
echo "start now: systemctl restart neco-price-cache.service"

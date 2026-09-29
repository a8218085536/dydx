#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

detect_default_base() {
  if [[ -d "$script_dir/../../tyche" ]]; then
    cd "$script_dir/../.." && pwd
    return
  fi
  if [[ "$(basename "$(pwd)")" == "tyche" && -d "$(pwd)/deploy" ]]; then
    cd "$(pwd)/.." && pwd
    return
  fi
  if [[ -d "$(pwd)/tyche" ]]; then
    pwd
    return
  fi
  if [[ -d "/home/ec2-user/flydex/dydx/tyche" ]]; then
    printf '%s\n' "/home/ec2-user/flydex/dydx"
    return
  fi
  printf '%s\n' ""
}

default_base="$(detect_default_base)"

BASE="${TYCHE_DEPLOY_BASE:-$default_base}"
PACKAGE_NAME="${TYCHE_PACKAGE_NAME:-tyche-dydx-deploy-20260815-final.zip}"
TMP_PACKAGE="${TYCHE_PACKAGE_PATH:-/tmp/$PACKAGE_NAME}"
LOCAL_PACKAGE="$BASE/$PACKAGE_NAME"
ROOT="$BASE/tyche"
SERVICE_NAME="${TYCHE_SERVICE_NAME:-tyche-dydx.service}"
PRICE_CACHE_SERVICE_NAME="${NECO_PRICE_CACHE_SERVICE_NAME:-neco-price-cache.service}"
LISTEN="${TYCHE_LISTEN:-0.0.0.0:8081}"
BACKUP_DIR="${TYCHE_BACKUP_DIR:-$BASE/backup}"
MARKET_PRICE_URL="${TYCHE_DYDX_MARKET_PRICE_URL:-http://127.0.0.1:8090/prices}"
MARKET_ORDER_SLIPPAGE_BPS="${TYCHE_MARKET_ORDER_SLIPPAGE_BPS:-100}"
RETAINED_COLLATERAL_USD="${TYCHE_DYDX_RETAINED_COLLATERAL_USD:-1}"
USDT_TOKEN_ADDRESS="${TYCHE_USDT_TOKEN_ADDRESS:-}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run with sudo or as root, for example:" >&2
  echo "  sudo TYCHE_SERVICE_USER=ec2-user bash deploy/update-tyche-dydx-from-tmp.sh" >&2
  exit 1
fi

if [[ ! -f "$TMP_PACKAGE" ]]; then
  echo "missing package: $TMP_PACKAGE" >&2
  echo "upload the package to /tmp first, or set TYCHE_PACKAGE_PATH=/path/to/package.zip" >&2
  exit 1
fi

if [[ -z "$BASE" ]]; then
  echo "could not detect Tyche deploy base" >&2
  echo "set it explicitly, for example:" >&2
  echo "  sudo TYCHE_DEPLOY_BASE=/home/ec2-user/flydex/dydx bash /tmp/update-tyche-dydx-from-tmp.sh" >&2
  exit 1
fi

if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip was not found in PATH" >&2
  exit 1
fi

if [[ ! -d "$ROOT" ]]; then
  echo "missing current Tyche root: $ROOT" >&2
  echo "set TYCHE_DEPLOY_BASE=/home/ec2-user/flydex/dydx if your deploy root is elsewhere" >&2
  exit 1
fi

SERVICE_USER="${TYCHE_SERVICE_USER:-}"
if [[ -z "$SERVICE_USER" ]]; then
  SERVICE_USER="$(stat -c '%U' "$ROOT")"
fi
if [[ "$SERVICE_USER" == "root" && -n "${SUDO_USER:-}" && "$SUDO_USER" != "root" ]]; then
  SERVICE_USER="$SUDO_USER"
fi
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  echo "service user does not exist: $SERVICE_USER" >&2
  exit 1
fi

stamp="$(date +%Y%m%d-%H%M%S)"
ROOT_BACKUP="$BASE/tyche.bak.$stamp"
CONFIG_BACKUP="$BACKUP_DIR/tyche.toml.$stamp"
EXTRACT_DIR="$BASE/.tyche-update.$stamp"

cleanup_extract_dir() {
  if [[ -n "${EXTRACT_DIR:-}" && "$EXTRACT_DIR" == "$BASE"/.tyche-update.* && -d "$EXTRACT_DIR" ]]; then
    rm -rf "$EXTRACT_DIR"
  fi
}
trap cleanup_extract_dir EXIT

price_cache_unit_exists() {
  systemctl list-unit-files "$PRICE_CACHE_SERVICE_NAME" --no-legend 2>/dev/null | grep -q "^$PRICE_CACHE_SERVICE_NAME"
}

mkdir -p "$BACKUP_DIR" "$EXTRACT_DIR"

if [[ -f "$ROOT/tyche.toml" ]]; then
  cp "$ROOT/tyche.toml" "$CONFIG_BACKUP"
else
  echo "missing current config: $ROOT/tyche.toml" >&2
  exit 1
fi

echo "moving package: $TMP_PACKAGE -> $LOCAL_PACKAGE"
mv -f "$TMP_PACKAGE" "$LOCAL_PACKAGE"

echo "validating package by extracting under $EXTRACT_DIR"
unzip -q "$LOCAL_PACKAGE" -d "$EXTRACT_DIR"
if [[ ! -d "$EXTRACT_DIR/tyche" ]]; then
  echo "package did not contain a normal tyche/ directory" >&2
  echo "the zip may have been created with Windows backslash path separators" >&2
  echo "current Tyche root was not moved; upload a fixed zip and retry" >&2
  exit 1
fi

echo "stopping $SERVICE_NAME"
systemctl stop "$SERVICE_NAME" || true

PRICE_CACHE_WAS_INSTALLED=0
if price_cache_unit_exists; then
  PRICE_CACHE_WAS_INSTALLED=1
  echo "stopping $PRICE_CACHE_SERVICE_NAME"
  systemctl stop "$PRICE_CACHE_SERVICE_NAME" || true
fi

echo "backing up current Tyche root: $ROOT -> $ROOT_BACKUP"
mv "$ROOT" "$ROOT_BACKUP"

echo "installing extracted Tyche root: $EXTRACT_DIR/tyche -> $ROOT"
mv "$EXTRACT_DIR/tyche" "$ROOT"

echo "restoring config: $CONFIG_BACKUP -> $ROOT/tyche.toml"
cp "$CONFIG_BACKUP" "$ROOT/tyche.toml"

find "$ROOT/deploy" -type f -name '*.sh' -exec sed -i 's/\r$//' {} +
sed -i 's/\r$//' "$ROOT/tyche.toml"
chmod +x "$ROOT/deploy/"*.sh

set_toml_scalar() {
  local key="$1"
  local value="$2"
  local file="$3"
  if grep -q "^${key}[[:space:]]*=" "$file"; then
    sed -i "s#^${key}[[:space:]]*=.*#${key} = ${value}#" "$file"
  else
    sed -i "/^\[tyche\]/a ${key} = ${value}" "$file"
  fi
}

set_toml_scalar "readTimeout" "30" "$ROOT/tyche.toml"
set_toml_scalar "writeTimeout" "120" "$ROOT/tyche.toml"
set_toml_scalar "maxHeaderBytes" "1048576" "$ROOT/tyche.toml"
set_toml_scalar "dydxRetainedCollateralUsd" "\"$RETAINED_COLLATERAL_USD\"" "$ROOT/tyche.toml"

ensure_dedx_usd_market() {
  local file="$1"
  if grep -q 'market[[:space:]]*=[[:space:]]*"FDEX-USD"' "$file"; then
    return
  fi
  cat >> "$file" <<'EOF'

[[tyche.dydxMarkets]]
market = "FDEX-USD"
clobPairId = 302
quantumsPerUnit = "1000000"
stepBaseQuantums = "1000000"
subticksPerUnit = "1000000000"
subticksPerTick = "1000000"
EOF
}

toml_string_value() {
  local key="$1"
  local file="$2"
  sed -n "s#^[[:space:]]*${key}[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*#\1#p" "$file" | head -n 1
}

disable_neco_trading_asset() {
  local file="$1"
  local temp="${file}.asset-mapping.$$"
  awk '
    /^\[\[tyche\.assetMappings\]\]/ { in_mapping = 1; is_neco = 0 }
    /^\[\[tyche\./ && $0 !~ /^\[\[tyche\.assetMappings\]\]/ { in_mapping = 0; is_neco = 0 }
    in_mapping && /^[[:space:]]*necoAsset[[:space:]]*=[[:space:]]*"NECO"/ { is_neco = 1 }
    in_mapping && is_neco && /^[[:space:]]*enabledForTrading[[:space:]]*=/ { $0 = "enabledForTrading = false" }
    { print }
  ' "$file" > "$temp"
  mv "$temp" "$file"
}

ensure_usdt_asset_mapping() {
  local file="$1"
  local token_address="$2"
  if [[ -z "$token_address" ]]; then
    echo "cannot add USDT asset mapping: usdToken is empty; set TYCHE_USDT_TOKEN_ADDRESS explicitly" >&2
    exit 1
  fi
  if grep -q 'necoAsset[[:space:]]*=[[:space:]]*"USDT"' "$file"; then
    return
  fi
  cat >> "$file" <<EOF

[[tyche.assetMappings]]
necoTokenAddress = "$token_address"
necoAsset = "USDT"
dydxAsset = "USD"
decimals = 18
minUnit = "1"
priceRoute = "USDT-USD"
enabledForTrading = true
EOF
}

ensure_usdt_usd_route() {
  local file="$1"
  if grep -q 'name[[:space:]]*=[[:space:]]*"USDT-USD"' "$file"; then
    return
  fi
  cat >> "$file" <<'EOF'

[[tyche.priceRoutes]]
name = "USDT-USD"
asset = "USDT"
usdAsset = "USD"
EOF
}

if ! grep -q '^dydxMarketPriceURL[[:space:]]*=' "$ROOT/tyche.toml"; then
  if grep -q '^dydxEventSourceURL[[:space:]]*=' "$ROOT/tyche.toml"; then
    sed -i "/^dydxEventSourceURL[[:space:]]*=/a dydxMarketPriceURL = \"$MARKET_PRICE_URL\"" "$ROOT/tyche.toml"
  else
    sed -i "/^\[tyche\]/a dydxMarketPriceURL = \"$MARKET_PRICE_URL\"" "$ROOT/tyche.toml"
  fi
fi
if ! grep -q '^marketOrderSlippageBps[[:space:]]*=' "$ROOT/tyche.toml"; then
  sed -i "/^dydxMarketPriceURL[[:space:]]*=/a marketOrderSlippageBps = $MARKET_ORDER_SLIPPAGE_BPS" "$ROOT/tyche.toml"
fi
ensure_dedx_usd_market "$ROOT/tyche.toml"
disable_neco_trading_asset "$ROOT/tyche.toml"
if [[ -z "$USDT_TOKEN_ADDRESS" ]]; then
  USDT_TOKEN_ADDRESS="$(toml_string_value "usdToken" "$ROOT/tyche.toml")"
fi
ensure_usdt_asset_mapping "$ROOT/tyche.toml" "$USDT_TOKEN_ADDRESS"
ensure_usdt_usd_route "$ROOT/tyche.toml"

echo "installing refreshed systemd service"
TYCHE_SERVICE_USER="$SERVICE_USER" \
TYCHE_LISTEN="$LISTEN" \
TYCHE_ROOT="$ROOT" \
TYCHE_CONFIG="$ROOT/tyche.toml" \
TYCHE_RENDER_CONFIG=0 \
bash "$ROOT/deploy/install-tyche-dydx-systemd.sh"

if [[ "$PRICE_CACHE_WAS_INSTALLED" == "1" ]]; then
  echo "installing refreshed $PRICE_CACHE_SERVICE_NAME"
  TYCHE_SERVICE_USER="$SERVICE_USER" \
  TYCHE_ROOT="$ROOT" \
  TYCHE_CONFIG="$ROOT/tyche.toml" \
  NECO_PRICE_CACHE_START_NOW=1 \
  bash "$ROOT/deploy/install-neco-price-cache-systemd.sh"
fi

echo "starting $SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
systemctl status "$SERVICE_NAME" --no-pager -l

echo
echo "update complete"
echo "package: $LOCAL_PACKAGE"
echo "config backup: $CONFIG_BACKUP"
echo "old Tyche backup: $ROOT_BACKUP"

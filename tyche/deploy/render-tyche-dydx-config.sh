#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
root="${TYCHE_ROOT:-$default_root}"
root_parent="$(cd "$root/.." && pwd)"

template="${TYCHE_TEMPLATE:-$root/deploy/tyche.dydx.single-node.toml}"
output="${TYCHE_CONFIG:-$root/tyche.toml}"
dydx_env="${DYDX_TRADING_ENV:-$root_parent/dydxtrading/.env}"

if [[ ! -f "$template" ]]; then
  echo "missing Tyche template: $template" >&2
  exit 1
fi

if [[ ! -f "$dydx_env" ]]; then
  echo "missing dydxtrading env file: $dydx_env" >&2
  exit 1
fi

read_env() {
  local key="$1"
  local file="$2"
  local line
  line="$(grep -E "^${key}=" "$file" | tail -n 1 || true)"
  line="${line#*=}"
  line="${line//$'\r'/}"
  printf '%s' "$line"
}

token="${TYCHE_INTERNAL_API_KEY:-$(read_env DYDX_TRADING_BEARER_TOKEN "$dydx_env")}"
if [[ ${#token} -lt 32 ]]; then
  echo "DYDX_TRADING_BEARER_TOKEN/TYCHE_INTERNAL_API_KEY must be at least 32 characters" >&2
  exit 1
fi

admin_md5="${TYCHE_ADMIN_PASSWORD_MD5:-}"
redis_uri="${TYCHE_REDIS_URI:-}"
listen="${TYCHE_LISTEN:-}"
market_price_url="${TYCHE_DYDX_MARKET_PRICE_URL:-}"
market_slippage_bps="${TYCHE_MARKET_ORDER_SLIPPAGE_BPS:-}"
admin_md5="${admin_md5//$'\r'/}"
redis_uri="${redis_uri//$'\r'/}"
listen="${listen//$'\r'/}"
market_price_url="${market_price_url//$'\r'/}"
market_slippage_bps="${market_slippage_bps//$'\r'/}"

mkdir -p "$(dirname "$output")"
install -m 600 "$template" "$output"

python3 - "$output" "$token" "$admin_md5" "$redis_uri" "$listen" "$market_price_url" "$market_slippage_bps" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
token = sys.argv[2]
admin_md5 = sys.argv[3]
redis_uri = sys.argv[4]
listen = sys.argv[5]
market_price_url = sys.argv[6]
market_slippage_bps = sys.argv[7]

text = path.read_text()
text = text.replace("CHANGE_ME_COPY_DYDX_TRADING_BEARER_TOKEN_32PLUS", token)
if admin_md5:
    text = text.replace("CHANGE_ME_ADMIN_PASSWORD_MD5", admin_md5)
if redis_uri:
    text = text.replace('URI = "redis://127.0.0.1:6379/2"', f'URI = "{redis_uri}"')
if listen:
    text = text.replace('listen = "127.0.0.1:8081"', f'listen = "{listen}"')
if market_price_url:
    text = text.replace('dydxMarketPriceURL = "http://127.0.0.1:8090/prices"', f'dydxMarketPriceURL = "{market_price_url}"')
if market_slippage_bps:
    text = text.replace('marketOrderSlippageBps = 100', f'marketOrderSlippageBps = {market_slippage_bps}')
path.write_text(text)
PY

echo "wrote Tyche config: $output"
if grep -q 'CHANGE_ME_ADMIN_PASSWORD_MD5' "$output"; then
  echo "warning: expectedMD5Password is still a placeholder; set TYCHE_ADMIN_PASSWORD_MD5 before public exposure" >&2
fi

#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
ROOT="${TYCHE_ROOT:-$default_root}"
CONFIG="${TYCHE_CONFIG:-$ROOT/tyche.toml}"
INTERVAL="${NECO_PRICE_CACHE_INTERVAL:-1s}"
TTL="${NECO_PRICE_CACHE_TTL:-30s}"
ASSETS="${NECO_PRICE_CACHE_ASSETS:-}"

cd "$ROOT"
mkdir -p "$ROOT/logs"

if [[ ! -f "$CONFIG" ]]; then
  echo "missing config: $CONFIG" >&2
  exit 1
fi

if [[ ! -x "$ROOT/build/neco-price-cache" ]]; then
  echo "missing binary: $ROOT/build/neco-price-cache" >&2
  echo "run: bash deploy/build-neco-price-cache.sh" >&2
  exit 1
fi

args=(-config "$CONFIG" -interval "$INTERVAL" -ttl "$TTL")
if [[ -n "$ASSETS" ]]; then
  args+=(-assets "$ASSETS")
fi

exec "$ROOT/build/neco-price-cache" "${args[@]}"

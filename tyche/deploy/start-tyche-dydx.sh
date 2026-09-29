#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
ROOT="${TYCHE_ROOT:-$default_root}"
CONFIG="${TYCHE_CONFIG:-$ROOT/tyche.toml}"

cd "$ROOT"
mkdir -p "$ROOT/logs"

if [[ ! -f "$CONFIG" ]]; then
  if [[ -f "$ROOT/deploy/render-tyche-dydx-config.sh" ]]; then
    TYCHE_CONFIG="$CONFIG" bash "$ROOT/deploy/render-tyche-dydx-config.sh"
  else
    echo "missing config: $CONFIG" >&2
    echo "copy deploy/tyche.dydx.single-node.toml to $CONFIG and replace CHANGE_ME values" >&2
    exit 1
  fi
fi

if grep -q 'CHANGE_ME_COPY_DYDX_TRADING_BEARER_TOKEN_32PLUS' "$CONFIG"; then
  echo "Tyche internal API key placeholder is still present in $CONFIG" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  if [[ ! -x "$ROOT/build/tyche" ]]; then
    echo "go was not found in PATH and $ROOT/build/tyche is missing" >&2
    exit 1
  fi
  exec "$ROOT/build/tyche"
fi

if [[ -x "$ROOT/build/tyche" ]]; then
  exec "$ROOT/build/tyche"
fi

exec go run ./cmd/tyche

#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
ROOT="${TYCHE_ROOT:-$default_root}"
CONFIG="${TYCHE_CONFIG:-$ROOT/tyche.toml}"
MOCK_LISTEN="${MOCK_DYDX_LISTEN:-127.0.0.1:8090}"

cd "$ROOT"
mkdir -p "$ROOT/logs"

if [ ! -f "$CONFIG" ]; then
  echo "missing config: $CONFIG" >&2
  echo "copy deploy/tyche.mock.toml to $CONFIG and replace CHANGE_ME values" >&2
  exit 1
fi

go run ./cmd/mock-dydx-sidecar -listen "$MOCK_LISTEN" &
MOCK_PID=$!
trap 'kill "$MOCK_PID" 2>/dev/null || true' EXIT

go run ./cmd/tyche

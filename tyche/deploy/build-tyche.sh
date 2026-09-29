#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
default_root="$(cd "$script_dir/.." && pwd)"
ROOT="${TYCHE_ROOT:-$default_root}"
OUT="${TYCHE_BUILD_OUT:-$ROOT/build/tyche}"
GO_BIN="${TYCHE_GO_BIN:-}"

cd "$ROOT"
mkdir -p "$(dirname "$OUT")"

if [[ -z "$GO_BIN" ]]; then
  if command -v go >/dev/null 2>&1; then
    GO_BIN="$(command -v go)"
  else
    for candidate in /usr/local/go/bin/go /usr/bin/go /usr/local/bin/go /opt/go/bin/go; do
      if [[ -x "$candidate" ]]; then
        GO_BIN="$candidate"
        break
      fi
    done
  fi
fi

if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
  echo "go was not found; set TYCHE_GO_BIN=/path/to/go or install Go" >&2
  exit 1
fi

"$GO_BIN" build -trimpath -o "$OUT" ./cmd/tyche
chmod 755 "$OUT"
echo "built Tyche binary: $OUT"

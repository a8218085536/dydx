#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

detect_default_base() {
  if [[ -d "$script_dir/../../dydxtrading" ]]; then
    cd "$script_dir/../.." && pwd
    return
  fi
  if [[ "$(basename "$(pwd)")" == "dydxtrading" && -f "$(pwd)/docker-compose.yml" ]]; then
    cd "$(pwd)/.." && pwd
    return
  fi
  if [[ -d "$(pwd)/dydxtrading" ]]; then
    pwd
    return
  fi
  if [[ -d "/home/ec2-user/flydex/dydx/dydxtrading" ]]; then
    printf '%s\n' "/home/ec2-user/flydex/dydx"
    return
  fi
  printf '%s\n' ""
}

default_base="$(detect_default_base)"

BASE="${DYDXTRADING_DEPLOY_BASE:-$default_base}"
PACKAGE_NAME="${DYDXTRADING_PACKAGE_NAME:-dydxtrading-deploy-20260815-final.zip}"
TMP_PACKAGE="${DYDXTRADING_PACKAGE_PATH:-/tmp/$PACKAGE_NAME}"
LOCAL_PACKAGE="$BASE/$PACKAGE_NAME"
ROOT="$BASE/dydxtrading"
BACKUP_DIR="${DYDXTRADING_BACKUP_DIR:-$BASE/backup}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run with sudo or as root, for example:" >&2
  echo "  sudo DYDXTRADING_DEPLOY_BASE=/home/ec2-user/flydex/dydx bash /tmp/update-dydxtrading-from-tmp.sh" >&2
  exit 1
fi

if [[ -z "$BASE" ]]; then
  echo "could not detect dydxtrading deploy base" >&2
  echo "set it explicitly, for example:" >&2
  echo "  sudo DYDXTRADING_DEPLOY_BASE=/home/ec2-user/flydex/dydx bash /tmp/update-dydxtrading-from-tmp.sh" >&2
  exit 1
fi

if [[ ! -f "$TMP_PACKAGE" ]]; then
  echo "missing package: $TMP_PACKAGE" >&2
  echo "upload the package to /tmp first, or set DYDXTRADING_PACKAGE_PATH=/path/to/package.zip" >&2
  exit 1
fi

if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip was not found in PATH" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker was not found in PATH" >&2
  exit 1
fi

if [[ ! -d "$ROOT" ]]; then
  echo "missing current dydxtrading root: $ROOT" >&2
  exit 1
fi

stamp="$(date +%Y%m%d-%H%M%S)"
ROOT_BACKUP="$BASE/dydxtrading.bak.$stamp"
ENV_BACKUP="$BACKUP_DIR/dydxtrading.env.$stamp"
EXTRACT_DIR="$BASE/.dydxtrading-update.$stamp"

cleanup_extract_dir() {
  if [[ -n "${EXTRACT_DIR:-}" && "$EXTRACT_DIR" == "$BASE"/.dydxtrading-update.* && -d "$EXTRACT_DIR" ]]; then
    rm -rf "$EXTRACT_DIR"
  fi
}
trap cleanup_extract_dir EXIT

mkdir -p "$BACKUP_DIR" "$EXTRACT_DIR"

if [[ -f "$ROOT/.env" ]]; then
  cp "$ROOT/.env" "$ENV_BACKUP"
else
  echo "missing current sidecar env: $ROOT/.env" >&2
  exit 1
fi

echo "moving package: $TMP_PACKAGE -> $LOCAL_PACKAGE"
mv -f "$TMP_PACKAGE" "$LOCAL_PACKAGE"

echo "validating package by extracting under $EXTRACT_DIR"
unzip -q "$LOCAL_PACKAGE" -d "$EXTRACT_DIR"
if [[ ! -d "$EXTRACT_DIR/dydxtrading" ]]; then
  echo "package did not contain a normal dydxtrading/ directory" >&2
  echo "current dydxtrading root was not moved; upload a fixed zip and retry" >&2
  exit 1
fi

echo "stopping dydxtrading docker service"
(
  cd "$ROOT"
  docker compose --env-file .env -f docker-compose.yml down || true
)

echo "backing up current dydxtrading root: $ROOT -> $ROOT_BACKUP"
mv "$ROOT" "$ROOT_BACKUP"

echo "installing extracted dydxtrading root: $EXTRACT_DIR/dydxtrading -> $ROOT"
mv "$EXTRACT_DIR/dydxtrading" "$ROOT"

echo "restoring env: $ENV_BACKUP -> $ROOT/.env"
cp "$ENV_BACKUP" "$ROOT/.env"

if [[ -d "$ROOT_BACKUP/data" ]]; then
  echo "restoring data directory: $ROOT_BACKUP/data -> $ROOT/data"
  mv "$ROOT_BACKUP/data" "$ROOT/data"
fi

find "$ROOT" -type f -name '*.sh' -exec sed -i 's/\r$//' {} +

echo "starting dydxtrading docker service"
(
  cd "$ROOT"
  docker compose --env-file .env -f docker-compose.yml up -d --build
  docker compose --env-file .env -f docker-compose.yml ps
)

echo
echo "update complete"
echo "package: $LOCAL_PACKAGE"
echo "env backup: $ENV_BACKUP"
echo "old dydxtrading backup: $ROOT_BACKUP"

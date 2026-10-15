#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: provision-market-maker-accounts.sh \
  --base-url URL --accounts-file FILE --seed-key-file FILE --target AMOUNT
EOF
}

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

base_url=""
accounts_file=""
seed_key_file=""
target=""
while (($#)); do
  case "$1" in
    --base-url) base_url=${2:-}; shift 2 ;;
    --accounts-file) accounts_file=${2:-}; shift 2 ;;
    --seed-key-file) seed_key_file=${2:-}; shift 2 ;;
    --target) target=${2:-}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $1" ;;
  esac
done

[[ -n "$base_url" && -n "$accounts_file" && -n "$seed_key_file" && -n "$target" ]] \
  || { usage >&2; die "all arguments are required"; }
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v jq >/dev/null 2>&1 || die "jq is required"
[[ -r "$accounts_file" ]] || die "accounts file is not readable"
[[ -r "$seed_key_file" ]] || die "seed key file is not readable"

outcome_marker="${accounts_file}.outcome-unknown"
[[ ! -e "$outcome_marker" ]] || die \
  "an earlier deposit has unknown outcome; reconcile the authoritative Assets Engine state before removing $outcome_marker"

case "$base_url" in
  https://*) ;;
  http://127.0.0.1:*|http://localhost:*|http://\[::1\]:*) ;;
  *) die "base URL must use HTTPS or loopback HTTP" ;;
esac
base_url=${base_url%/}

decimal_to_fp() {
  local value=$1 whole fraction
  [[ "$value" =~ ^([0-9]+)(\.([0-9]{1,8}))?$ ]] || return 1
  whole=${BASH_REMATCH[1]}
  fraction=${BASH_REMATCH[3]:-}
  while ((${#fraction} < 8)); do fraction+="0"; done
  printf '%s\n' "$((10#$whole * 100000000 + 10#${fraction:-0}))"
}

fp_to_decimal() {
  local value=$1
  printf '%d.%08d' "$((value / 100000000))" "$((value % 100000000))"
}

target_fp=$(decimal_to_fp "$target") || die "target must be a non-negative decimal with at most 8 digits"
((target_fp > 0)) || die "target must be positive"

seed_key=$(tr -d '\r\n' < "$seed_key_file")
[[ "$seed_key" =~ ^[A-Za-z0-9._~-]+$ ]] || die "seed key file contains an invalid key"

curl_config=$(mktemp)
snapshot=$(mktemp)
cleanup() {
  rm -f "$curl_config" "$snapshot"
}
trap cleanup EXIT HUP INT TERM
chmod 600 "$curl_config"
{
  printf 'header = "X-API-Key: %s"\n' "$seed_key"
  printf 'header = "Content-Type: application/json"\n'
} > "$curl_config"
unset seed_key

api_get() {
  curl --silent --show-error --fail-with-body --config "$curl_config" --url "$1"
}

api_post() {
  curl --silent --show-error --fail-with-body --config "$curl_config" \
    --request POST --url "$1" --data "$2"
}

mark_outcome_unknown() {
  local label=$1 user_id=$2 asset_id=$3 current=$4 delta=$5 marker_tmp
  marker_tmp="${outcome_marker}.tmp.$$"
  umask 077
  {
    printf 'status=OUTCOME_UNKNOWN\n'
    printf 'label=%s\n' "$label"
    printf 'userId=%s\n' "$user_id"
    printf 'assetId=%s\n' "$asset_id"
    printf 'observedBefore=%s\n' "$current"
    printf 'requestedDelta=%s\n' "$delta"
    printf 'createdAtUtc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "$marker_tmp"
  chmod 600 "$marker_tmp"
  mv -f "$marker_tmp" "$outcome_marker"
}

declare -a labels=()
declare -a user_ids=()
declare -A seen_user_ids=()
line_number=0
while IFS=, read -r label user_id extra; do
  ((line_number += 1))
  label=${label%$'\r'}
  user_id=${user_id%$'\r'}
  if ((line_number == 1)) && [[ "$label" == "label" && "$user_id" == "userId" ]]; then
    continue
  fi
  [[ -z "${extra:-}" ]] || die "accounts CSV line $line_number has too many columns"
  [[ -n "$label" ]] || die "accounts CSV line $line_number has an empty label"
  [[ "$label" =~ ^[A-Za-z0-9._-]+$ ]] || die "accounts CSV line $line_number has an invalid label"
  [[ "$user_id" =~ ^[1-9][0-9]*$ ]] || die "accounts CSV line $line_number has an invalid userId"
  [[ -z "${seen_user_ids[$user_id]:-}" ]] || die "duplicate userId: $user_id"
  seen_user_ids[$user_id]=1
  labels+=("$label")
  user_ids+=("$user_id")
done < "$accounts_file"
[[ ${#user_ids[@]} -eq 10 ]] || die "accounts file must contain exactly 10 accounts"

assets=(0 1 2 3)

# Preflight every account before the first balance mutation.
for i in "${!user_ids[@]}"; do
  label=${labels[$i]}
  user_id=${user_ids[$i]}
  balances=$(api_get "$base_url/api/v1/accounts/$user_id") \
    || die "could not read balances for $label ($user_id)"
  orders=$(api_get "$base_url/api/v1/orders?userId=$user_id") \
    || die "could not read active orders for $label ($user_id)"
  active_count=$(printf '%s' "$orders" | jq -r \
    'if type == "array" then length elif has("orders") then (.orders | length) else 0 end') \
    || die "invalid active-order response for $label ($user_id)"
  [[ "$active_count" =~ ^[0-9]+$ ]] || die "invalid active-order count for $label ($user_id)"
  ((active_count == 0)) || die "$label ($user_id) has active orders"

  for asset_id in "${assets[@]}"; do
    row=$(printf '%s' "$balances" | jq -r --argjson assetId "$asset_id" \
      '.assets[]? | select((.assetId | tonumber) == $assetId) | [.available, .locked, .total] | @tsv') \
      || die "invalid balance response for $label ($user_id)"
    [[ -n "$row" ]] || die "$label ($user_id) is missing assetId $asset_id"
    IFS=$'\t' read -r available locked total <<< "$row"
    available_fp=$(decimal_to_fp "$available") || die "invalid available balance for $label assetId $asset_id"
    locked_fp=$(decimal_to_fp "$locked") || die "invalid locked balance for $label assetId $asset_id"
    total_fp=$(decimal_to_fp "$total") || die "invalid total balance for $label assetId $asset_id"
    ((available_fp + locked_fp == total_fp)) \
      || die "$label ($user_id) assetId $asset_id has inconsistent balances"
    ((locked_fp == 0)) || die "$label ($user_id) assetId $asset_id has a locked balance"
    ((total_fp <= target_fp)) || die "$label ($user_id) assetId $asset_id exceeds target"
    printf '%s\t%s\t%s\t%s\n' "$label" "$user_id" "$asset_id" "$total_fp" >> "$snapshot"
  done
done

printf 'Preflight PASS: 10 accounts, no open orders, no locked or excess balances\n'

while IFS=$'\t' read -r label user_id asset_id current_fp; do
  delta_fp=$((target_fp - current_fp))
  current=$(fp_to_decimal "$current_fp")
  if ((delta_fp == 0)); then
    printf '%s userId=%s assetId=%s current=%s deposit=0.00000000 status=SKIP\n' \
      "$label" "$user_id" "$asset_id" "$current"
    continue
  fi
  delta=$(fp_to_decimal "$delta_fp")
  body=$(printf '{"assetId":%s,"amount":"%s"}' "$asset_id" "$delta")
  if ! api_post "$base_url/api/v1/accounts/$user_id/deposit" "$body" >/dev/null; then
    mark_outcome_unknown "$label" "$user_id" "$asset_id" "$current" "$delta"
    die "deposit outcome unknown for $label ($user_id) assetId $asset_id; reconcile Assets Engine before retrying"
  fi
  printf '%s userId=%s assetId=%s current=%s deposit=%s status=FUNDED\n' \
    "$label" "$user_id" "$asset_id" "$current" "$delta"
done < "$snapshot"

for i in "${!user_ids[@]}"; do
  label=${labels[$i]}
  user_id=${user_ids[$i]}
  balances=$(api_get "$base_url/api/v1/accounts/$user_id") \
    || die "could not verify balances for $label ($user_id)"
  for asset_id in "${assets[@]}"; do
    row=$(printf '%s' "$balances" | jq -r --argjson assetId "$asset_id" \
      '.assets[]? | select((.assetId | tonumber) == $assetId) | [.available, .locked, .total] | @tsv') \
      || die "invalid verification response for $label ($user_id)"
    [[ -n "$row" ]] || die "$label ($user_id) is missing assetId $asset_id after funding"
    IFS=$'\t' read -r available locked total <<< "$row"
    available_fp=$(decimal_to_fp "$available") || die "invalid verification available balance"
    locked_fp=$(decimal_to_fp "$locked") || die "invalid verification locked balance"
    total_fp=$(decimal_to_fp "$total") || die "invalid verification total balance"
    ((available_fp == target_fp && locked_fp == 0 && total_fp == target_fp)) \
      || die "$label ($user_id) assetId $asset_id did not reach the exact target"
  done
done

printf 'Verification PASS: all 10 accounts hold the exact target in assets 0-3\n'

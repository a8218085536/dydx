#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
PROVISION="$ROOT/scripts/spot/provision-market-maker-accounts.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

[[ -x "$PROVISION" ]] || fail "provisioning script does not exist or is not executable"

mkdir -p "$TMP/bin"
cat > "$TMP/bin/jq" <<'PY'
#!/usr/bin/env python
import json, sys

args = sys.argv[1:]
asset_id = None
i = 0
while i < len(args):
    if args[i] == "--argjson":
        if args[i + 1] == "assetId":
            asset_id = int(args[i + 2])
        i += 3
    elif args[i].startswith("-"):
        i += 1
    else:
        break
expr = args[i] if i < len(args) else ""
data = json.load(sys.stdin)
if ".assets[]" in expr:
    for item in data.get("assets", []):
        if int(item.get("assetId", -1)) == asset_id:
            print("\t".join(str(item[k]) for k in ("available", "locked", "total")))
            break
elif "type == \"array\"" in expr:
    print(len(data if isinstance(data, list) else data.get("orders", [])))
else:
    raise SystemExit("unsupported jq expression: " + expr)
PY
chmod +x "$TMP/bin/jq"

cat > "$TMP/bin/chmod" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "600" && -n "${2:-}" ]]; then
  printf '%s\n' "$2" > "$FAKE_CHMOD_FILE"
fi
/usr/bin/chmod "$@"
SH
chmod +x "$TMP/bin/chmod"

cat > "$TMP/bin/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
url=""
method="GET"
data=""
config=""
while (($#)); do
  case "$1" in
    --url) url=$2; shift 2 ;;
    --config) config=$2; shift 2 ;;
    --request|-X) method=$2; shift 2 ;;
    --data|--data-raw) data=$2; shift 2 ;;
    --silent|--show-error|--fail|--fail-with-body) shift ;;
    *) shift ;;
  esac
done

[[ -n "$config" ]] || exit 90
[[ -s "$FAKE_CHMOD_FILE" && "$(cat "$FAKE_CHMOD_FILE")" == "$config" ]] || exit 92
printf 'CONFIG_MODE 600\n' >> "$FAKE_CALLS"
printf '%s %s %s\n' "$method" "$url" "$data" >> "$FAKE_CALLS"

balance_for() {
  local user=$1 asset=$2 found
  found=$(awk -F: -v u="$user" -v a="$asset" '$1==u && $2==a {v=$3} END {print v}' "$FAKE_STATE")
  if [[ -n "$found" ]]; then printf '%s' "$found"; return; fi
  case "$FAKE_MODE" in
    exact) printf '100' ;;
    partial) [[ "$user:$asset" == "910001:0" ]] && printf '40' || printf '100' ;;
    over) [[ "$user:$asset" == "910001:0" ]] && printf '101' || printf '100' ;;
    *) printf '0' ;;
  esac
}

if [[ "$method" == "GET" && "$url" =~ /api/v1/accounts/([0-9]+)$ ]]; then
  user=${BASH_REMATCH[1]}
  printf '{"userId":%s,"assets":[' "$user"
  for asset in 0 1 2 3; do
    value=$(balance_for "$user" "$asset")
    locked=0
    if [[ "$FAKE_MODE" == "locked" && "$user:$asset" == "910001:0" ]]; then locked=1; fi
    ((asset > 0)) && printf ','
    printf '{"assetId":%s,"available":"%s","locked":"%s","total":"%s"}' \
      "$asset" "$value" "$locked" "$value"
  done
  printf ']}\n'
  exit 0
fi

if [[ "$method" == "GET" && "$url" == *"/api/v1/orders?"* ]]; then
  if [[ "$FAKE_MODE" == "active" && "$url" == *"userId=910001"* ]]; then
    printf '[{"omsOrderId":7,"status":"NEW"}]\n'
  else
    printf '[]\n'
  fi
  exit 0
fi

if [[ "$method" == "POST" && "$url" =~ /api/v1/accounts/([0-9]+)/deposit$ ]]; then
  [[ "$FAKE_MODE" != "failed" ]] || exit 22
  user=${BASH_REMATCH[1]}
  asset=$(printf '%s' "$data" | sed -n 's/.*"assetId":\([0-9][0-9]*\).*/\1/p')
  printf '%s:%s:100\n' "$user" "$asset" >> "$FAKE_STATE"
  printf '{"success":true}\n'
  exit 0
fi

exit 91
SH
chmod +x "$TMP/bin/curl"

ACCOUNTS="$TMP/accounts.csv"
{
  printf 'label,userId\n'
  for n in $(seq 1 10); do printf 'maker-%02d,%d\n' "$n" "$((910000+n))"; done
} > "$ACCOUNTS"
KEY_FILE="$TMP/seed.key"
SECRET='super-secret-seed-key'
printf '%s\n' "$SECRET" > "$KEY_FILE"
chmod 600 "$KEY_FILE"

run_case() {
  local mode=$1 output=$2 calls=$3 state=$4
  : > "$calls"
  : > "$state"
  : > "$TMP/chmod.file"
  set +e
  FAKE_MODE="$mode" FAKE_CALLS="$calls" FAKE_STATE="$state" \
    FAKE_CHMOD_FILE="$TMP/chmod.file" \
    PATH="$TMP/bin:$PATH" bash "$PROVISION" \
      --base-url http://127.0.0.1:8080 \
      --accounts-file "$ACCOUNTS" \
      --seed-key-file "$KEY_FILE" \
      --target 100 > "$output" 2>&1
  local status=$?
  set -e
  ! grep -Fq "$SECRET" "$output" || fail "$mode leaked the seed key"
  ! grep -Fq "$SECRET" "$calls" || fail "$mode leaked the seed key into curl arguments"
  printf '%s' "$status"
}

for mode in zero partial exact over locked active failed; do
  output="$TMP/$mode.out"
  calls="$TMP/$mode.calls"
  state="$TMP/$mode.state"
  status=$(run_case "$mode" "$output" "$calls" "$state")
  case "$mode" in
    zero)
      if [[ "$status" != 0 ]]; then
        sed 's/^/  /' "$output" >&2
        fail "zero-balance provisioning failed"
      fi
      [[ $(grep -c '^POST .*deposit ' "$calls") == 40 ]] || fail "zero case did not make 40 deposits"
      ;;
    partial)
      [[ "$status" == 0 ]] || fail "partial retry failed"
      grep -Eq 'POST .*/accounts/910001/deposit .*"assetId":0.*"amount":"60(\.0+)?"' "$calls" \
        || fail "partial retry did not deposit target-current"
      ;;
    exact)
      [[ "$status" == 0 ]] || fail "exact-target verification failed"
      ! grep -q '^POST ' "$calls" || fail "exact target made a deposit"
      ;;
    over|locked|active)
      [[ "$status" != 0 ]] || fail "$mode unsafe account was accepted"
      ! grep -q '^POST ' "$calls" || fail "$mode mutated an unsafe account"
      ;;
    failed)
      [[ "$status" != 0 ]] || fail "failed deposit was ignored"
      ;;
  esac
  grep -q '^CONFIG_MODE 600$' "$calls" || fail "$mode curl config was not mode 0600"
done

printf 'PASS: provisioning safety, retry, and redaction cases\n'

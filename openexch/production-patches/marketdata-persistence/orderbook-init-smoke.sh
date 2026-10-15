#!/usr/bin/env bash
set -euo pipefail

stage="startup"
trap 'printf "FAILED_STAGE=%s LINE=%s\n" "$stage" "$LINENO" >&2' ERR

ROOT="${NECO_ROOT:-/home/ec2-user/flydex/neco-exchange}"
OMS_URL="${OMS_URL:-http://127.0.0.1:28081}"
HANDOFF="${MARKET_MAKER_HANDOFF:-$ROOT/secrets/market-maker-handoff.csv}"
SMOKE_PRICE="${SMOKE_PRICE:-2300.00000000}"

line="$(awk -F, '$1 == "mm01" { print; exit }' "$HANDOFF")"
IFS=, read -r label user_id api_key <<< "$line"
test -n "$api_key"

balances() {
  curl -fsS -H "X-API-Key: $api_key" "$OMS_URL/api/v1/balances" \
    | jq -c '[.assets[] | {assetId, available, locked, total}] | sort_by(.assetId)'
}

stage="read-before-balances"
before="$(balances)"
order_id="${EXISTING_ORDER_ID:-}"
existing_cleanup=false
if [ -z "$order_id" ]; then
  client_id="marketdata-init-$(date +%s)"
  stage="create-order"
  created="$(curl -fsS \
    -H "X-API-Key: $api_key" \
    -H 'Content-Type: application/json' \
    -d "{\"userId\":$user_id,\"marketId\":1,\"side\":\"BUY\",\"orderType\":\"LIMIT\",\"timeInForce\":\"GTC\",\"price\":\"$SMOKE_PRICE\",\"quantity\":\"0.01000000\",\"clientOrderId\":\"$client_id\"}" \
    "$OMS_URL/api/v1/orders")"
  printf 'CREATE_RESULT=%s\n' "$(jq -c '{accepted, duplicate, omsOrderId, status, rejectReason}' <<< "$created")"
  test "$(jq -r .accepted <<< "$created")" = true
  order_id="$(jq -r .omsOrderId <<< "$created")"
else
  existing_cleanup=true
fi

stage="wait-working"
status=""
for _ in $(seq 1 80); do
  status="$(curl -fsS -H "X-API-Key: $api_key" \
    "$OMS_URL/api/v1/orders/$order_id" | jq -r .status)"
  case "$status" in
    NEW|PARTIALLY_FILLED|CANCELLED|REJECTED|EXPIRED|FILLED) break ;;
  esac
  sleep 0.25
done
if [ "$existing_cleanup" = true ]; then
  case "$status" in
    CANCELLED|REJECTED|EXPIRED|FILLED)
      detail="$(curl -fsS -H "X-API-Key: $api_key" "$OMS_URL/api/v1/orders/$order_id")"
      printf 'ORDER_ID=%s\nORDER_STATUS=%s\nEXISTING_ORDER_CLEANED=true\n' "$order_id" "$status"
      printf 'ORDER_DETAIL=%s\n' "$(jq -c '{status, rejectReason, price, quantity, filledQty}' <<< "$detail")"
      exit 0
      ;;
  esac
fi
case "$status" in
  NEW|PARTIALLY_FILLED) ;;
  CANCELLED) printf 'ORDER_ID=%s\nORDER_STATUS=CANCELLED\n' "$order_id"; exit 0 ;;
  *) printf 'order did not become cancellable: status=%s\n' "$status" >&2; exit 1 ;;
esac

stage="cancel-order"
curl -fsS -X DELETE -H "X-API-Key: $api_key" \
  "$OMS_URL/api/v1/orders/$order_id" >/dev/null

stage="wait-cancelled"
for _ in $(seq 1 80); do
  status="$(curl -fsS -H "X-API-Key: $api_key" \
    "$OMS_URL/api/v1/orders/$order_id" | jq -r .status)"
  [ "$status" = "CANCELLED" ] && break
  sleep 0.25
done
test "$status" = "CANCELLED"
if [ "$existing_cleanup" = true ]; then
  printf 'ORDER_ID=%s\nORDER_STATUS=%s\nEXISTING_ORDER_CLEANED=true\n' "$order_id" "$status"
  exit 0
fi
stage="compare-balances"
test "$before" = "$(balances)"

printf 'ORDER_ID=%s\nORDER_STATUS=%s\nBALANCES_UNCHANGED=true\n' "$order_id" "$status"

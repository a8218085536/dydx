#!/usr/bin/env bash
set -euo pipefail

CONTAINER="${MARKETDATA_CONTAINER:-flydex-marketdata}"
OMS_DB="${OMS_DB:-oms}"

current_count="$(docker exec "$CONTAINER" psql -U market -d marketdata -Atc \
  'SELECT count(*) FROM trades')"
if [ "$current_count" != "0" ]; then
  echo "refusing backfill: marketdata.trades already contains $current_count row(s)" >&2
  exit 1
fi

(cd /tmp && sudo -n -u postgres psql -d "$OMS_DB" -v ON_ERROR_STOP=1 -c "\copy (
  SELECT min(executed_at) AS time,
         market_id,
         (price::numeric / 100000000)::double precision AS price,
         (quantity::numeric / 100000000)::double precision AS quantity,
         1 AS trade_count,
         CASE max(side) FILTER (WHERE NOT is_maker)
           WHEN 'BUY' THEN true
           WHEN 'SELL' THEN false
           ELSE NULL
         END AS taker_side
  FROM executions
  GROUP BY trade_id, market_id, price, quantity
  ORDER BY min(executed_at)
) TO STDOUT WITH (FORMAT csv)") \
  | docker exec -i "$CONTAINER" psql -U market -d marketdata -v ON_ERROR_STOP=1 -c \
      "\copy trades(time, market_id, price, quantity, trade_count, taker_side) FROM STDIN WITH (FORMAT csv)"

for aggregate in candles_1m candles_5m candles_15m candles_1h candles_4h candles_1d; do
  docker exec "$CONTAINER" psql -U market -d marketdata -v ON_ERROR_STOP=1 -c \
    "CALL refresh_continuous_aggregate('$aggregate', NULL, NULL);"
done

docker exec "$CONTAINER" psql -U market -d marketdata -Atc \
  'SELECT market_id, count(*), min(time), max(time) FROM trades GROUP BY market_id ORDER BY market_id'

# Spot market-data persistence

This production overlay runs a dedicated TimescaleDB 2.15.3/PostgreSQL 13
container for the spot market gateway. It keeps market-data storage isolated
from the OMS and NECO databases and exposes PostgreSQL only on
`127.0.0.1:5436`.

## Runtime layout

- Compose directory: `/home/ec2-user/flydex/neco-exchange/marketdata`
- Password file: `/home/ec2-user/flydex/neco-exchange/secrets/marketdata-postgres-password`
- Container: `flydex-marketdata`
- Database: `marketdata`
- JDBC URL: `jdbc:postgresql://127.0.0.1:5436/marketdata`
- Systemd unit: `flydex-marketdata.service`

`neco-exchange.service` requires and starts after the database unit. The market
gateway must use `MARKET_PERSISTENCE_MODE=timescale`.

## Historical backfill

`backfill-oms-executions.sh` converts the two OMS execution rows for each trade
(maker and taker) into one market-data trade row. It refuses to run when the
target table is non-empty, then refreshes all six continuous aggregates.

## Verification

Run `orderbook-init-smoke.sh` with a non-crossing price inside the configured
price-protection range:

```bash
SMOKE_PRICE=2300.00000000 ./orderbook-init-smoke.sh
```

The script creates and cancels one small limit order and verifies that account
balances return to their original values. It never prints the API key.

After restarting the market gateway, verify that `/api/trades` and
`/api/candles` still return the same database-backed history.

## Rollback

Restore the backed-up `neco-exchange.env`, restart the market gateway, and keep
the TimescaleDB volume intact for investigation. Do not delete the Docker
volume during rollback.

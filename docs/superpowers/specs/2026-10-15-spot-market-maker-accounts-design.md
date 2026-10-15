# Spot Market-Maker Internal Accounts Design

Date: 2026-10-15

## Goal

Create ten independent internal spot-exchange accounts for a colleague's
market-making program. Each account receives exactly:

- 1,000,000 USDT (asset ID 0)
- 1,000,000 NETH (asset ID 1)
- 1,000,000 NBNB (asset ID 2)
- 1,000,000 NBTC (asset ID 3)

The balances are internal liquidity inventory. They are not deposited from
NECO and must never be withdrawable to NECO or another chain. The accounts
submit, amend, cancel, and query orders through the normal OMS and matching
paths.

## Non-Goals

- Do not edit `account_balances` or any other balance table directly.
- Do not create or distribute NECO wallet private keys.
- Do not change matching, settlement, market definitions, fees, or trading
  strategy.
- Do not give the market-making program administrator, cross-account, deposit,
  or withdrawal permission.

## Existing Constraints

- The Assets Engine is the money authority. PostgreSQL `account_balances` is a
  query projection only.
- Production currently accepts Tyche wallet-login sessions. This path must
  continue working without client changes.
- The existing static API-key provider is a separate authentication mode, so it
  cannot currently coexist with Tyche authentication.
- Existing authorization treats all self-scoped account actions alike. That is
  insufficient because a self-scoped market-maker key must trade but must not
  call internal deposit or withdrawal paths.
- The production OMS package contains extensions beyond the clean local base.
  Deployment must preserve those extensions and use a production-compatible
  overlay rather than replacing the package with an unrelated base build.

## Authentication And Authorization

Add a hybrid Tyche plus API-key authentication mode:

- Existing wallet users continue to authenticate with their Tyche bearer
  session and `X-Neco-Address`.
- Each market-maker account receives one independent, random API key.
- REST clients send the key in `X-API-Key`.
- WebSocket clients carry the same key with the existing `bearer` WebSocket
  subprotocol convention.
- An explicit invalid `X-API-Key` fails authentication and never falls back to
  Tyche authentication.
- API keys are loaded from a root-readable server file, are redacted from logs,
  and are not stored in Git.

Add a `MARKET_MAKER` role and separate authorization actions for account read,
trading, and funding. A market-maker principal may only act as its assigned
user ID and may:

- read its identity, balances, orders, executions, and positions;
- create, amend, and cancel its own orders;
- subscribe to its own private WebSocket channels;
- consume public market data.

It must receive `403 Forbidden` for:

- internal deposit or internal withdrawal;
- NECO direct or bridge withdrawal;
- another user's account, orders, executions, positions, or WebSocket data;
- administrator and risk-control APIs;
- any unclassified private action.

Apply the same funding denial to REST and gRPC so a direct private-network call
cannot bypass the HTTP restriction.

## Account Allocation

Before allocation, inspect the production user-ID space and reject any ID that
already has a balance, order, execution, Tyche mapping, or API-key assignment.
Use a reserved range below the Tyche sequence (which starts at 1,000,000) and
outside the demo range 900000-900999. Record the selected ten IDs in the
operator manifest.

An account is created implicitly when its first Assets Engine deposit is
accepted. No user or balance row is manually inserted.

## Funding Procedure

Use the existing OMS deposit command so every credit is applied and journaled
by the Assets Engine. A temporary server-local operator key may seed the ten
accounts through the authenticated deposit endpoint. It is never delivered to
the colleague and is removed immediately after verification.

The seeding procedure is retry-safe:

1. Keep all market-maker credentials undistributed during funding.
2. Query every selected account and require zero locked balances and no open
   orders.
3. For each asset, credit only `target - current` when current is below the
   target.
4. Abort if a balance exceeds the target or an account is already active.
5. Verify all forty balances equal exactly 1,000,000.00000000.
6. Remove the temporary operator key, restart the OMS, and verify that the key
   is rejected.

The resulting system-wide internal inventory increase is 10,000,000 units of
each asset.

## Colleague API Surface

Base HTTPS URL for the test deployment is `https://testapp.flydex.ai`.
The final handoff document must use the routes actually exposed by the current
Nginx configuration.

Private trading and account APIs:

```text
GET    /api/v1/auth/me
GET    /api/v1/balances
GET    /api/v1/accounts/{userId}
POST   /api/v1/orders
GET    /api/v1/orders
GET    /api/v1/orders/{omsOrderId}
GET    /api/v1/orders/history
PUT    /api/v1/orders/{omsOrderId}
DELETE /api/v1/orders/{omsOrderId}
GET    /api/v1/executions
GET    /api/v1/positions
WS     /ws/v1 (private channels: orders, executions, balances)
```

Public market-data APIs:

```text
GET    /api/v1/markets
GET    /api/v1/index-prices/{market}
GET    /spot/api/v1/market-tickers
GET    /spot/api/v1/market-stats
GET    /spot/api/v1/sparklines?timePeriod=ONE_DAY
GET    /spot/api/orderbook?marketId=
GET    /spot/api/trades?marketId=&limit=
GET    /spot/api/candles?marketId=&interval=&limit=&before=
WS     /ws/v1 (public channels: orderbook, trades, candles)
```

The handoff explicitly excludes every deposit, withdrawal, bridge, admin, and
risk endpoint.

## Credential Handoff

Produce two separate artifacts:

1. A non-secret Chinese API guide containing authentication headers, market and
   asset mappings, REST examples, WebSocket examples, response fields, retry
   rules, rate-limit guidance, and the forbidden endpoint list.
2. A secret account manifest containing account label, OMS user ID, and API
   key. Store it outside Git with restrictive filesystem permissions. Do not
   print full keys in logs or the public guide.

Each account has a distinct key so one account can be revoked or rotated
without stopping the other nine.

## Verification

Automated tests must cover:

- Tyche authentication regression;
- API-key parsing and hybrid-provider routing;
- self-scoped market-maker reads and order actions;
- rejection of cross-account access;
- rejection of internal deposit, internal withdrawal, NECO withdrawal, bridge
  withdrawal, admin, and gRPC funding actions;
- authenticated private WebSocket subscription and anonymous public channels;
- key redaction in errors and audit logs.

Production verification must prove:

- all ten accounts have exactly four balances of 1,000,000.00000000;
- each key can query only its own account;
- a small limit order can be created and cancelled through the normal matching
  path without changing the final balance;
- funding and withdrawal attempts with a market-maker key return 403 and create
  no Assets Engine or withdrawal record;
- existing Tyche users can still authenticate and trade;
- the temporary operator key no longer authenticates.

## Deployment And Rollback

Back up the production OMS package, service environment, API-key file, and
relevant Nginx configuration before deployment. Build and test against the
production-compatible source overlay, restart only the OMS unless another
component is demonstrably required, and verify health plus Assets Engine
projection readiness before funding.

Rollback restores the previous OMS package and authentication configuration.
The credited Assets Engine balances are financial state and are not removed by
binary rollback; if account creation must be reversed, use authenticated Assets
Engine withdrawal commands while the accounts have no orders or locked funds,
then verify every balance is zero.

# Mock dYdX Runbook

This runbook is for the pre-RPC phase. Tyche is the only service deployed from
this repository. The `protocol` directory is reference material and is not
uploaded to the test server.

## Local Mock Mode

1. Start the mock dYdX sidecar:

   ```bash
   go run ./cmd/mock-dydx-sidecar -listen 127.0.0.1:8090
   ```

2. Copy the mock config and replace all `CHANGE_ME` values:

   ```bash
   cp deploy/tyche.mock.toml tyche.toml
   ```

3. Start Tyche:

   ```bash
   go run ./cmd/tyche
   ```

The mock sidecar exposes:

- `POST /dydx/trading` for native `params.cosmosTx` submissions.
- `GET /status` for mock block height.
- `GET /fills`, `GET /order-statuses`, and `GET /positions` for sync worker polling.

## Server Mock Mode

Deploy Tyche only and run tests:

```bash
python scripts/ssh_deploy_tyche.py --test
```

Upload service templates and mock config to `/data/dydx/services`:

```bash
python scripts/ssh_install_tyche_services.py
```

On the server, prepare config:

```bash
cp /data/dydx/services/tyche.mock.toml /data/dydx/tyche/tyche.toml
```

Then replace or confirm:

- Redis URI and DB number.
- Redis namespace/domain.
- `internalAPIKey` and `dydxEventSourceAPIKey`.
- NECO RPC, deposit address, token mappings, and price oracle settings for the
  target environment.
- NECO hot wallet settings only if this Tyche deployment is responsible for
  NECO withdrawals.
- NECO price oracle settings:
  `necoRpcURL`, `[[tyche.assetMappings]]`, and `[[tyche.priceRoutes]]`.
  The current setup uses the NECO FDEX/USDT pair directly; the legacy
  `dedxNecoPair`, `necoUsdPair`, `dedxToken`, `necoToken`, and `usdToken`
  fields stay empty when routes are configured.
- `dydxWalletCreateURL` should point to the sidecar `/wallet/create` endpoint.
- Do not put a dYdX user trading private key in Tyche. User proxy wallet keys
  belong in the `dydxtrading` keystore; the optional system wallet key is only
  for gas top-up inside `dydxtrading`.

## NECO Price Oracle

Tyche can quote any configured NECO-chain trading asset to USD through
`[[tyche.priceRoutes]]`. The single-node template currently enables:

- `FDEX-USDT`: FDEX priced through the direct FDEX/USDT pair.
- `USDT-USD`: USDT priced as a direct stable USD route.

When `tyche.necoPriceOracle.enabled = true`, Trading ignores the client-provided
`marginAmount` as the final Ledger charge. It reads the configured route pair
contracts, computes the selected `marginAsset` USD price, checks
`minLiquidityUsd`, and charges the user's Ledger in that asset using the
server-side quote. The order stores `marginUsd`, `priceSnapshotId`,
`assetUsdPrice`, and `priceRoute` for later refund and settlement tracing.

The user-facing flow should not be presented as a manual deposit-first flow.
Show the user a combined source for the selected trading asset instead:

- `ledgerAvailable`: the asset already credited to Tyche's trading ledger.
- `necoWalletAvailable`: the asset still visible in the user's NECO wallet.
- `displayAvailable`: the sum shown as the user's apparent trading source.

Use `POST /tyche/api/trading/balance/overview` to load this display balance.

When a dYdX order needs more of the selected asset than `ledgerAvailable`, Tyche returns the
order with `status = "funding_required"` and funding instruction fields such as
`fundingAction`, `fundingAmount`, `fundingTokenAddress`, and
`fundingToAddress`. The frontend should then trigger the NECO wallet
confirmation automatically:

- `fundingAction = "transfer"`: ask the user to transfer the required asset to
  Tyche's configured `tyche.necoDeposit.depositAddress`.
- After NECO confirmation and deposit sync credit the Ledger, submit the same
  `clientOrderId` again; Tyche resumes the order and sends it to dYdX.

Opening orders are funded with at least
`tyche.dydxMinStatefulOrderMarginUsd` USD. The default is `20`, matching the
single-node dYdX chain's first non-zero stateful-order equity tier. This avoids
chain rejections such as `equity tier limit of 0` for tiny orders.
Tyche also supports `tyche.dydxRetainedCollateralUsd`, a small per
dYdX account/subaccount buffer that remains in the dYdX subaccount so later
orders have enough base collateral to pass chain checks. It is system-retained
collateral, not user available balance, and it is not paid out on cancel or
close settlement.

Keep the mock price oracle disabled until real NECO RPC and pair addresses are
ready. After enabling it, do not expose NECO RPC publicly unless the NECO team
has explicitly approved that boundary.

For manual mock startup:

```bash
/data/dydx/services/start-tyche-mock.sh
```

The systemd files in `/data/dydx/services` are templates. Install or enable
them only after the config file has real environment values.

## Single-Node dYdX Tyche Config

When the dYdX node and `dydxtrading` already run on the same server, use:

```bash
cd /data/dydx/tyche
bash deploy/render-tyche-dydx-config.sh
```

If you installed files into `/data/dydx/services` instead of keeping them under
the Tyche release directory, use:

```bash
TYCHE_TEMPLATE=/data/dydx/services/tyche.dydx.single-node.toml \
TYCHE_CONFIG=/data/dydx/tyche/tyche.toml \
bash /data/dydx/services/render-tyche-dydx-config.sh
```

The script reads `/data/dydx/dydxtrading/.env`, copies
`deploy/tyche.dydx.single-node.toml` to `tyche.toml`, and fills Tyche
`internalAPIKey` plus `dydxEventSourceAPIKey` from
`DYDX_TRADING_BEARER_TOKEN`. This keeps Tyche and `dydxtrading` on the same
internal secret without printing it in deployment notes.

Default single-node boundaries:

- Tyche API: `127.0.0.1:8081`.
- `dydxtrading`: `127.0.0.1:8090`.
- dYdX CometBFT status: `127.0.0.1:26657/status`.
- Redis: `127.0.0.1:6379`, DB `2`, namespace `flydex:tyche`.

The single-node Tyche template exposes the current mainnet genesis CLOB markets
from `BTC-USD` through `XRP-USD` as the tradable whitelist. It intentionally
does not expose `FDEX-USD`, `USDT-USD`, or `DYDX-USD`: those are not usable
Tyche user trading markets in the current flow. `EIGEN-USD` and `BOME-USD`
should be added only after the running chain has matching CLOB pairs, not just
price-market entries in another test genesis.

If Redis has a password, run with:

```bash
TYCHE_REDIS_URI='redis://:PASSWORD@127.0.0.1:6379/2' bash deploy/render-tyche-dydx-config.sh
```

Optional admin hash:

```bash
TYCHE_ADMIN_PASSWORD_MD5='<32-char-md5>' bash deploy/render-tyche-dydx-config.sh
```

The single-node template is prefilled for the current NECO test environment:

- NECO RPC: `https://chainrpc.necochain.com/`.
- NECO chain id: `7260`.
- Token decimals from RPC: DEDX `18`, NECO `18`, USD `18`.
- Deposit sync starts at the configured `tyche.necoDeposit.startBlock`.

When deploying a different NECO environment, update those values before startup.
If the deposit address already received DEDX before Tyche starts, set
`startBlock` to a block before the first relevant deposit.

Start Tyche after the chain and `dydxtrading` are already running:

```bash
bash deploy/start-tyche-dydx.sh
```

or, with the uploaded service helper:

```bash
bash /data/dydx/services/start-tyche-dydx.sh
```

## Switching To Real dYdX RPC

Keep NECO as the only external deposit and withdrawal chain. dYdX remains
trading-only.

Replace these fields:

- `dydxTradingURL`: real `dydxtrading` sidecar `/dydx/trading` endpoint.
- `dydxWalletCreateURL`: real `dydxtrading` sidecar `/wallet/create` endpoint.
- `dydxEventSourceURL`: real `dydxtrading` sidecar base URL. Tyche uses it
  for raw event reads (`/fills`, `/order-statuses`, `/positions`) and for the
  chain reconcile endpoint (`/reconcile/order`).
- `dydxChainID`: official chain ID.
- `dydxHeightURL`: official CometBFT status endpoint.
- `dydxMarkets`: official market to `clobPairId` and precision mapping.

Keep Tyche `account` and `privateKey` empty. Do not set `dydxBroadcastURL`,
`dydxChainBroadcastURL`, or `dydxAutoAddressPrefix` in dYdX mode. User proxy
wallets and user order signing belong only in the `dydxtrading` keystore.
`dydxtrading` accepts either `DYDX_SYSTEM_MNEMONIC` or
`DYDX_SYSTEM_PRIVATE_KEY_HEX` for the optional system gas wallet. This wallet is
only for proxy-wallet gas top-up, not for signing user orders.

Stop the mock sidecar after real endpoints are configured.

## Management APIs

All management APIs are protected by `X-Tyche-Internal-Key` when
`internalAPIKey` is configured.

- `POST /tyche/api/wallet-binding/overview`: user binding overview.
- `POST /tyche/api/trading/markets`: configured dYdX market list.
- `POST /tyche/api/trading/assets`: enabled NECO funding assets.
- `POST /tyche/api/trading/quote`: server-side pre-order funding quote.
- `POST /tyche/api/trading/balance/overview`: combined Ledger + NECO wallet asset view.
- `POST /tyche/api/trading/order/list`: user order list.
- `POST /tyche/api/dydx/sync/failures`: failed/rejected sync events.
- `POST /tyche/api/dydx/sync/retry`: safe retry-read hook for a failed sync event.

The retry endpoint does not mutate Ledger by itself. Money-changing actions are
still guarded by existing Ledger `RefID`, sync `eventId`, order `clientOrderId`,
and withdrawal idempotency.

## Reconciliation

The reconciliation service is read-only. It compares Ledger balances, Trading
orders, and dYdX sync snapshots and emits issues for manual review. It does not
refund, debit, cancel, or resubmit orders automatically.

# Tyche Multi Asset Trading Funding

Tyche can now use more than one NECO-chain asset as the funding asset for
dYdX trading. NECO remains the only external deposit/withdraw chain; dYdX
orders still trade USD-quoted perpetual markets.

## Configuration

Each tradable NECO asset needs an `assetMappings` entry:

```toml
[[tyche.assetMappings]]
necoTokenAddress = "0xUsdtTokenOnNECO"
necoAsset = "USDT"
dydxAsset = "USD"
decimals = 18
minUnit = "1"
priceRoute = "USDT-USD"
enabledForTrading = true
```

Each asset used for trading also needs a route to USD:

```toml
[[tyche.priceRoutes]]
name = "USDT-USD"
asset = "USDT"
usdAsset = "USD"
```

Stable USD assets such as USDT can use a direct zero-hop route.

One-hop pool routes are supported. The current FDEX setup uses the NECO
FDEX/USDT pair directly:

```toml
[[tyche.priceRoutes]]
name = "FDEX-USDT"
asset = "FDEX"
usdAsset = "USD"

[[tyche.priceRoutes.hops]]
pair = "0xFdexUsdtPair"
fromToken = "0xFdexToken"
toToken = "0xUsdtToken"
fromDecimals = 18
toDecimals = 18
```

Multi-hop routes are still supported when a future asset needs to route through
more than one pool.

Set `enabledForTrading = false` for assets that should not be used for dYdX
order funding. Add non-trading deposit-only assets under `tyche.necoDeposit`
tokens instead of `assetMappings`.

## Frontend APIs

Available dYdX markets:

```http
POST /tyche/api/trading/markets
```

Tradable funding assets:

```http
POST /tyche/api/trading/assets
```

Pre-order quote:

```http
POST /tyche/api/trading/quote
Content-Type: application/json
```

```json
{
  "market": "ETH-USD",
  "price": "3000",
  "size": "0.01",
  "marginAsset": "USDT"
}
```

Response data includes `chargeAsset`, `chargeAmount`, `marginUsd`,
`assetUsdPrice`, `priceRoute`, and `priceSnapshotId`.

## Order Flow

When `tyche.necoPriceOracle.enabled = true`, Tyche ignores any client-provided
`marginAmount` as the final charge. It calculates:

```text
orderUsd = order price * order size
marginUsd = max(orderUsd, tyche.dydxMinStatefulOrderMarginUsd)
chargeAmount = ceil(marginUsd / assetUsdPrice * 10^assetDecimals)
```

The selected `marginAsset` is charged from Ledger. If Ledger is short but the
user's NECO wallet has enough of that token, Tyche returns
`status = "funding_required"` with transfer instructions for that asset.
The default dYdX minimum is `20` USD because the current single-node chain
allows zero open stateful orders below that equity tier.
Tyche may also top up `tyche.dydxRetainedCollateralUsd` once per
dYdX account/subaccount as a non-user-visible collateral buffer. That buffer is
not counted in trading balance, does not reduce the NECO amount charged for the
next order, and is excluded from cancel/close settlement amounts.

For normal USD-quoted markets, closing a position settles back to the same
NECO-chain asset used for entry. For the chain market `FDEX-USD` only, Tyche
settles to the opposite supported NECO asset: FDEX entry settles as USDT, and
USDT entry settles as FDEX. The dYdX market name remains `FDEX-USD` so the
chain does not need to be redeployed just because the NECO token display name
changed to FDEX.

## Adding Another Asset

1. Add the NECO token to `[[tyche.assetMappings]]`.
2. Set `dydxAsset = "USD"` for dYdX trading funding.
3. Add one or more `[[tyche.priceRoutes.hops]]` to reach USD.
4. Confirm pair token order and decimals.
5. Enable `enabledForTrading = true`.
6. Restart Tyche.
7. Check `/tyche/api/trading/assets` and `/tyche/api/trading/quote`.

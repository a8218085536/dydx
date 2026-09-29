# Tyche Multi Asset Price Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Tyche use configurable NECO-chain assets such as DEDX and NECO as dYdX trading funding assets by converting each asset to USD through configured swap pair routes.

**Architecture:** Keep NECO deposits and Ledger asset accounting asset-native. Add configurable price routes that convert a configured `marginAsset` to USD, then have the trading margin quoter pick the route from `assetMappings`. Expose read-only frontend endpoints for configured markets, tradable funding assets, and pre-trade quotes.

**Tech Stack:** Go, Gin, go-ethereum ABI calls, existing Tyche config/TOML patterns, existing in-memory test fakes.

## Global Constraints

- NECO remains the only external deposit/withdraw chain.
- dYdX receives USD/USDC-margin order intent; Tyche charges the selected NECO-side asset.
- Asset price routes must be configuration driven and must reject unsupported or disabled assets.
- Existing DEDX route must keep working.
- Tests must cover DEDX two-hop route, NECO one-hop route, unsupported asset rejection, and frontend query endpoints.

---

### Task 1: Config And Price Route Types

**Files:**
- Modify: `tyche/tyche/asset_mapping.go`
- Modify: `tyche/tyche/config.go`
- Test: `tyche/cmd/tyche/app/config_test.go`

**Interfaces:**
- Produces: `AssetMapping.PriceRoute`, `AssetMapping.EnabledForTrading`, `PriceRouteConfig`, `PriceRouteHopConfig`.

- [ ] Add failing config parse assertions for DEDX and NECO routes.
- [ ] Add fields and structs.
- [ ] Verify config tests pass.

### Task 2: Generic Asset USD Oracle

**Files:**
- Create/Modify: `tyche/tyche/neco_price_oracle.go`
- Test: `tyche/tyche/neco_price_oracle_test.go`

**Interfaces:**
- Produces: `AssetUSDOracle.AssetUSDPrice(ctx, asset)`.
- Consumes: `PriceRouteConfig`.

- [ ] Add failing tests for `DEDX-NECO-USD` and `NECO-USD`.
- [ ] Implement route hop evaluation over V2 pair reserves.
- [ ] Keep existing DEDX oracle behavior by wrapping the generic oracle.

### Task 3: Multi Asset Margin Quoter

**Files:**
- Modify: `tyche/tyche/neco_price_oracle.go`
- Modify: `tyche/tyche/trading.go`
- Test: `tyche/tyche/trading_test.go`

**Interfaces:**
- Produces: `NewAssetMarginQuoter(mapper, oracle)`.
- Consumes: request `MarginAsset`.

- [ ] Add failing test that a NECO order charges NECO using the NECO route.
- [ ] Add failing test for disabled or missing `marginAsset`.
- [ ] Implement asset-aware quote selection and keep idempotent order behavior.

### Task 4: Frontend Read APIs

**Files:**
- Modify: `tyche/cmd/tyche/app/api_trading.go`
- Modify: `tyche/cmd/tyche/app/router.go`
- Test: `tyche/cmd/tyche/app/api_trading_test.go`

**Interfaces:**
- Produces: `POST /tyche/api/trading/markets`, `POST /tyche/api/trading/assets`, `POST /tyche/api/trading/quote`.

- [ ] Add failing API tests.
- [ ] Implement handlers from loaded config and trading quoter.
- [ ] Keep public config output free of private keys and internal API keys.

### Task 5: Deployment Config And Full Verification

**Files:**
- Modify: `tyche/deploy/tyche.dydx.single-node.toml`
- Modify: `tyche/cmd/tyche/tyche.dydx.example.toml`
- Modify docs if API docs are stale.

- [ ] Add DEDX and NECO `assetMappings`.
- [ ] Add route config for `DEDX-NECO-USD` and `NECO-USD`.
- [ ] Run `go test ./...` inside `tyche`.

# dYdX Tyche Format Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Tyche ready to submit native dYdX place/cancel order requests once the real chain RPC and sidecar URLs are available.

**Architecture:** Tyche remains the only deployed component for this task. It builds native Cosmos-style `params.cosmosTx` payloads locally and posts them to a future dYdX trading sidecar; the local `protocol` tree is reference-only and is not uploaded to the Linux server.

**Tech Stack:** Go 1.18+ for Tyche, standard `net/http` tests, existing in-memory Tyche services, Redis-backed production stores unchanged.

## Global Constraints

- Do not deploy `protocol` to the Linux server.
- Do not require real dYdX RPC during this task.
- Keep NECO as the only external deposit/withdrawal entry.
- dYdX is trading-only from Tyche's perspective.
- Preserve existing Tyche API routes and Redis store shapes unless a tested change requires otherwise.

---

### Task 1: Trading Payload Contract Tests

**Files:**
- Modify: `tyche/tyche/dydx_trading_http_client_test.go`
- Modify: `tyche/tyche/dydx_trading_http_client.go`

**Interfaces:**
- Consumes: `NewDYDXTradingHTTPClient`, `DYDXTradingHTTPClient.SubmitCosmosTx`, `DYDXCosmosTxRequest`
- Produces: Stable HTTP request contract for the future dYdX sidecar.

- [ ] **Step 1: Write failing tests**

Add tests proving `SubmitCosmosTx` posts:

```json
{
  "id": "order-1",
  "action": "place_order",
  "account": "dydx1owner",
  "params": {
    "cosmosTx": {
      "id": "order-1",
      "action": "place_order",
      "account": "dydx1owner",
      "chainId": "dydx-local-1",
      "signMode": "SIGN_MODE_DIRECT",
      "broadcastMode": "BROADCAST_MODE_SYNC",
      "message": {
        "typeUrl": "/dydxprotocol.clob.MsgPlaceOrder"
      }
    }
  }
}
```

Also assert `Authorization: Bearer <internal key>` is present and that non-200 HTTP statuses become errors.

- [ ] **Step 2: Verify red**

Run: `go test ./tyche -run TestDYDXTradingHTTPClient`

Expected: at least one new test fails because the HTTP client currently does not reject non-2xx HTTP status before decoding the body.

- [ ] **Step 3: Implement minimal behavior**

Update `post` so it rejects HTTP status outside `200..299` with an error message containing the status code.

- [ ] **Step 4: Verify green**

Run: `go test ./tyche -run TestDYDXTradingHTTPClient`

Expected: PASS.

### Task 2: Native Order Format Tests

**Files:**
- Modify: `tyche/tyche/dydx_cosmos_tx_test.go`
- Modify: `tyche/tyche/dydx_cosmos_tx.go`

**Interfaces:**
- Consumes: `DYDXCosmosOrderBuilder.BuildPlaceOrder`, `DYDXCosmosOrderBuilder.BuildCancelOrder`
- Produces: Tested conversion from Tyche trading request fields to dYdX CLOB message JSON.

- [ ] **Step 1: Write failing tests**

Add tests for:

```go
// place order
ClientOrderID "123" -> clientId 123
OrderFlags -> 0
SubaccountNumber -> configured value
Market "DEDX-USD" -> clobPairId 302
Size "1.25" with quantumsPerUnit "1000000" -> 1250000
Price "2500.5" with subticksPerUnit "1000000" -> 2500500000
GoodTilBlock -> current height + configured delta

// cancel order
Cancel uses the same native orderId tuple:
owner, subaccount number, clientId, orderFlags, clobPairId
```

Add one new test for `post_only` mapping to `DYDXTimeInForcePostOnly` if not already covered.

- [ ] **Step 2: Verify red**

Run: `go test ./tyche -run TestDYDXCosmos`

Expected: new coverage should fail only if a field is not currently wired.

- [ ] **Step 3: Implement minimal behavior**

If a field is missing, set it in `DYDXCosmosOrderBuilder`; otherwise keep code unchanged.

- [ ] **Step 4: Verify green**

Run: `go test ./tyche -run TestDYDXCosmos`

Expected: PASS.

### Task 3: Runner Integration Contract

**Files:**
- Modify: `tyche/tyche/dydx_runner_test.go`
- Modify: `tyche/tyche/dydx_runner.go`

**Interfaces:**
- Consumes: `DYDXRunner.Submit`, `newDYDXCosmosRequestFactoryFromConfig`
- Produces: Tested behavior that configured `dydxTradingURL` sends `params.cosmosTx` instead of queueing old intent JSON.

- [ ] **Step 1: Write failing tests**

Add a cancel-order integration test that configures `dydxTradingURL`, `dydxChainID`, `dydxHeightURL`, `dydxMarkets`, and a static height provider, then asserts the outgoing payload is `params.cosmosTx` with `/dydxprotocol.clob.MsgCancelOrder`.

- [ ] **Step 2: Verify red**

Run: `go test ./tyche -run TestDYDXRunner`

Expected: fail only if cancel does not use `cosmosTx`.

- [ ] **Step 3: Implement minimal behavior**

Fix `submitTrading` or the request factory only if the test exposes a gap.

- [ ] **Step 4: Verify green**

Run: `go test ./tyche -run TestDYDXRunner`

Expected: PASS.

### Task 4: Config Example Notes

**Files:**
- Modify: `tyche/cmd/tyche/tyche.dydx.example.toml`
- Modify: `tyche/cmd/tyche/app/config_test.go`

**Interfaces:**
- Consumes: existing TOML config parser
- Produces: clear placeholders for future RPC/sidecar wiring.

- [ ] **Step 1: Update example text**

Clarify these fields wait for official chain values:

```toml
dydxTradingURL
dydxHeightURL
dydxChainID
dydxMarkets
dydxEventSourceURL
```

- [ ] **Step 2: Verify config still parses**

Run: `go test ./cmd/tyche/app -run TestDYDXExampleConfigParsesCosmosTradingSettings`

Expected: PASS.

### Task 5: Server Verification

**Files:**
- Use: `scripts/ssh_deploy_tyche.py`

**Interfaces:**
- Consumes: prepared `/data/dydx` Linux test server
- Produces: Linux verification that Tyche still passes without deploying `protocol`.

- [ ] **Step 1: Run local full Tyche tests**

Run: `go test ./...` from `tyche`.

Expected: PASS.

- [ ] **Step 2: Deploy Tyche only and test on Linux**

Run: `python scripts/ssh_deploy_tyche.py --test` from workspace root.

Expected: server output includes `tyche_deployed=true` and `go test ./...` PASS.

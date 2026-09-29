# dYdX Mock Sidecar Funding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Tyche fully test dYdX trading submit and sync flows before official dYdX RPC is available.

**Architecture:** Add a Tyche-only mock dYdX sidecar HTTP handler that validates `params.cosmosTx`, returns configurable `submitted` or `rejected` submit results, and serves mock `fills`, `order-statuses`, and `positions`. Add status-aware submit handling so a synchronous `rejected` result refunds the order charge immediately. Add query endpoints for synced fills, order-status events, and positions.

**Tech Stack:** Go, `net/http`, existing Tyche in-memory stores and Redis stores, no `protocol` deployment.

## Global Constraints

- Do not upload or deploy `protocol`.
- Do not require a real dYdX RPC.
- Do not change NECO as the only external deposit/withdrawal path.
- Keep Ledger idempotency by `RefID` and TradingSync idempotency by `EventID`.

---

### Task 1: Mock Sidecar

**Files:**
- Create: `tyche/tyche/mock_dydx_sidecar.go`
- Create: `tyche/tyche/mock_dydx_sidecar_test.go`

**Interfaces:**
- Produces: `NewMockDYDXSidecar(conf MockDYDXSidecarConfig) *MockDYDXSidecar`
- Produces: `(*MockDYDXSidecar).Handler() http.Handler`

- [x] Write tests for trading submit validation and event-source reads.
- [x] Implement a lightweight HTTP handler for `/dydx/trading`, `/fills`, `/order-statuses`, and `/positions`.
- [x] Verify with `go test ./tyche -run TestMockDYDXSidecar`.

### Task 2: Status-Aware Submit Funding

**Files:**
- Modify: `tyche/tyche/trading.go`
- Modify: `tyche/tyche/dydx_runner.go`
- Modify: `tyche/tyche/trading_test.go`

**Interfaces:**
- Produces: optional `DYDXResultSubmitter` with `SubmitWithResult(context.Context, DYDXIntent) (DYDXTradingResult, error)`

- [x] Write tests proving synchronous `rejected` refunds charged balance and stores rejected order status.
- [x] Implement result-aware submit handling while preserving existing `DYDXSubmitter` behavior.
- [x] Verify with `go test ./tyche -run TestTradingService`.

### Task 3: Sync Query Endpoints

**Files:**
- Modify: `tyche/tyche/trading_sync.go`
- Modify: `tyche/model/trading/store.go`
- Modify: `tyche/cmd/tyche/app/api_trading.go`
- Modify: `tyche/cmd/tyche/app/router.go`
- Modify: `tyche/cmd/tyche/app/api_trading_test.go`

**Interfaces:**
- Produces: `TradingSyncService.GetFill`, `GetOrderStatus`, and `GetPosition`
- Produces routes `/tyche/api/dydx/sync/fill/get`, `/tyche/api/dydx/sync/order-status/get`, `/tyche/api/dydx/sync/position/get`

- [x] Write API tests for querying sync records.
- [x] Implement memory and Redis store reads.
- [x] Verify with `go test ./cmd/tyche/app -run TestDYDXSync`.

### Task 4: Full Verification

**Files:**
- Use: `scripts/ssh_deploy_tyche.py`

- [x] Run local `go test ./...` from `tyche`.
- [x] Deploy Tyche only to Linux and run `go test ./...`.

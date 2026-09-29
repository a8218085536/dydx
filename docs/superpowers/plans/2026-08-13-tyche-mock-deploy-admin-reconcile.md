# Tyche Mock Deploy Admin Reconcile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Before official dYdX RPC arrives, make Tyche runnable in a realistic mock trading environment with deploy scripts, isolated config, management queries, funding-flow tests, reconciliation, and run documentation.

**Architecture:** Tyche remains the only deployed service on the Linux test server. A standalone mock dYdX sidecar command uses the existing `tyche.MockDYDXSidecar` handler. Deployment scripts generate test config and systemd unit files for Tyche and the mock sidecar without embedding secrets in the repo. Management APIs expose read-only views plus controlled retry hooks over existing stores.

**Tech Stack:** Go 1.18+ for Tyche, Redis for real deployment state, PowerShell/Python Paramiko scripts for remote deployment, systemd on Ubuntu 20.04, no `protocol` deployment.

## Global Constraints

- Do not upload or deploy `protocol`.
- Do not require official dYdX RPC.
- NECO remains the only external deposit/withdrawal path.
- dYdX is trading-only and may be mock or real by configuration.
- All money-moving retries must stay idempotent by `clientOrderId`, `withdrawalId`, `eventId`, or Ledger `RefID`.
- Do not print secrets from `server-connection.local.env`.

---

### Task 1: Standalone Mock dYdX Sidecar Command

**Files:**
- Create: `tyche/cmd/mock-dydx-sidecar/main.go`
- Create: `tyche/cmd/mock-dydx-sidecar/main_test.go`
- Modify: `tyche/tyche/mock_dydx_sidecar.go`

**Interfaces:**
- Produces command `go run ./cmd/mock-dydx-sidecar -listen 127.0.0.1:8090`
- Produces env/config flags for submit status, tx hash, and optional seed events.

- [ ] Write tests for flag parsing and handler startup wiring.
- [ ] Implement command using `tyche.NewMockDYDXSidecar`.
- [ ] Verify with `go test ./cmd/mock-dydx-sidecar`.

### Task 2: Deployment Config and Service Scripts

**Files:**
- Create: `tyche/deploy/tyche.mock.toml`
- Create: `tyche/deploy/systemd/tyche-mock.service`
- Create: `tyche/deploy/systemd/mock-dydx-sidecar.service`
- Create: `tyche/deploy/start-tyche-mock.sh`
- Create: `scripts/ssh_install_tyche_services.py`

**Interfaces:**
- Produces test config with Redis DB, namespace, internal API key placeholder, NECO RPC placeholder, and mock/real dYdX mode notes.
- Produces remote installer that copies service files to `/data/dydx/services` without deploying `protocol`.

- [ ] Write script/unit files.
- [ ] Run static checks via shell syntax checks where possible.
- [ ] Verify no secrets are embedded.

### Task 3: Management Query and Retry APIs

**Files:**
- Modify: `tyche/tyche/wallet_binding.go`
- Modify: `tyche/model/walletbindings/store.go`
- Modify: `tyche/tyche/trading.go`
- Modify: `tyche/model/trading/store.go`
- Modify: `tyche/cmd/tyche/app/api_wallet_binding.go`
- Modify: `tyche/cmd/tyche/app/api_trading.go`
- Modify: `tyche/cmd/tyche/app/router.go`
- Modify tests in `tyche/cmd/tyche/app`

**Interfaces:**
- Produces `/tyche/api/wallet-binding/overview`
- Produces `/tyche/api/trading/order/list`
- Produces `/tyche/api/dydx/sync/failures`
- Produces `/tyche/api/dydx/sync/retry`

- [ ] Write API tests for overview, order list, failures list, and retry.
- [ ] Implement memory and Redis store methods needed by APIs.
- [ ] Verify with targeted app tests.

### Task 4: Mock Complete Funding Flow Tests

**Files:**
- Modify: `tyche/tyche/trading_sync_test.go`
- Modify: `tyche/tyche/mock_sidecar_funding_test.go`

**Interfaces:**
- Produces tested flows:
  - `submitted -> partial_filled`
  - `partial_filled -> cancelled` with remainder refund
  - `filled` with fee and realized PnL
  - duplicate fill/status no duplicate ledger update

- [ ] Write tests using mock sidecar event source and existing sync worker.
- [ ] Implement any missing small rules.
- [ ] Verify with `go test ./tyche -run "TestTradingWithMockSidecar|TestTradingSyncService"`.

### Task 5: Read-Only Reconciliation Worker

**Files:**
- Modify: `tyche/tyche/reconciliation.go`
- Modify: `tyche/tyche/reconciliation_test.go`

**Interfaces:**
- Produces checks comparing Ledger balances, Trading orders, and dYdX positions/sync snapshots.
- Produces mismatch records without changing money or order state.

- [ ] Write tests for missing position, rejected order with remaining charged balance, and healthy case.
- [ ] Implement read-only issue generation.
- [ ] Verify with `go test ./tyche -run TestReconciliation`.

### Task 6: Run Documentation

**Files:**
- Create: `tyche/docs/mock-dydx-runbook.md`

**Interfaces:**
- Explains how to run mock before official RPC.
- Explains which fields to replace after official RPC arrives.
- Explains which services remain Tyche-only and which `protocol` work is intentionally not deployed.

- [ ] Write runbook.
- [ ] Include local and server commands.
- [ ] Include config checklist.

### Task 7: Full Verification

**Files:**
- Use: `scripts/ssh_deploy_tyche.py`

- [ ] Run local `go test ./...` from `tyche`.
- [ ] Deploy Tyche only to Linux and run `go test ./...`.
- [ ] Confirm deployment scripts still package only `tyche`.

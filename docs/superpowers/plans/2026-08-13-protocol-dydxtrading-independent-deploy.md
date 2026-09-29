# Protocol dYdX Trading Independent Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `dydxtrading` deployable as an optional service independent from the `dydxprotocold` chain node.

**Architecture:** Keep the existing chain node Dockerfile, localnet Dockerfile, and compose service entrypoints dedicated to `dydxprotocold`/`cosmovisor`. Add separate container and compose material for `cmd/dydxtrading`, with environment-driven RPC/REST/Tyche settings and loopback-only default listen behavior. Document the operational boundary between chain node, Tyche trading proxy, and Tyche backend.

**Tech Stack:** Go 1.23.1, Docker multi-stage builds on Alpine, Docker Compose, existing `cmd/dydxtrading` environment variables.

## Global Constraints

- Do not modify the dYdX chain node core startup entrypoint.
- Do not make `dydxprotocold` depend on `dydxtrading`.
- `dydxtrading` may depend on chain RPC/REST, but must be independently startable, stoppable, restartable, and upgradable.
- `dydxtrading` must default to non-public listening; remote listen requires explicit configuration.
- Do not deploy or upload `protocol` to the Tyche test server as part of this task.

---

### Task 1: Static Deployment Boundary Test

**Files:**
- Create: `protocol/testing/deployment/deployment_boundary_test.go`

**Interfaces:**
- Produces tests asserting core chain Docker/compose files do not reference `dydxtrading`.
- Produces tests asserting sidecar Docker/compose files exist and avoid host port publishing.

- [ ] Write failing tests for missing sidecar deploy files.
- [ ] Run the test and confirm failure is caused by missing independent sidecar files.
- [ ] Implement independent deploy files.
- [ ] Re-run the test and confirm it passes.

### Task 2: Independent dYdX Trading Container

**Files:**
- Create: `protocol/Dockerfile.dydxtrading`
- Create: `protocol/docker-compose.dydxtrading.yml`
- Modify: `protocol/Makefile`

**Interfaces:**
- Produces image target `local:dydxtrading`.
- Produces compose service `dydxtrading` that uses `DYDX_REST_URL`, `DYDX_BROADCAST_URL`, `DYDX_TRADING_LISTEN`, `DYDX_TRADING_BEARER_TOKEN`, `DYDX_PROXY_WALLET_KEYSTORE_PATH`, `DYDX_PROXY_WALLET_KEYSTORE_SECRET`, optional `DYDX_SYSTEM_PRIVATE_KEY_HEX` for gas top-up, `TYCHE_URL`, and `TYCHE_INTERNAL_API_KEY`.

- [ ] Add Dockerfile building only `./cmd/dydxtrading`.
- [ ] Add compose override with no `ports:` section, only `expose: 8090`.
- [ ] Add Makefile targets for build/up/down/logs.

### Task 3: Deployment Documentation

**Files:**
- Create: `protocol/docs/dydxtrading-deployment.md`
- Modify: `protocol/README.md`

**Interfaces:**
- Documents `dydxprotocold`, `dydxtrading`, and Tyche boundaries.
- Documents startup order, maintenance model, and port exposure.

- [ ] Write docs explaining independent lifecycle.
- [ ] Link docs from README.

### Task 4: Verification

**Files:**
- Use existing Go packages and Dockerfiles.

**Interfaces:**
- Verifies sidecar tests, command build, chain node build path, and Docker build where available.

- [ ] Run `go test -mod=readonly -tags='test_ledger_mock netgo' ./sidecar/dydxtrading ./testing/deployment`.
- [ ] Run `go test -mod=readonly -tags='test_ledger_mock netgo' ./cmd/dydxtrading`.
- [ ] Run `go build -mod=readonly -tags='netgo' ./cmd/dydxtrading`.
- [ ] Run `go build -mod=readonly -tags='netgo' ./cmd/dydxprotocold` if the local platform allows it.
- [ ] Run Docker build for `Dockerfile.dydxtrading` and chain `Dockerfile` if Docker is available.

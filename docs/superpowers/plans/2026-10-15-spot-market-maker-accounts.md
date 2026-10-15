# Spot Market-Maker Accounts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create ten independently authenticated, trading-only spot accounts and fund each with 1,000,000 USDT, NETH, NBNB, and NBTC through the Assets Engine.

**Architecture:** Add a hybrid Tyche/API-key authentication provider while retaining the existing Tyche login path. Introduce explicit read, trade, and funding authorization actions so `MARKET_MAKER` keys can operate only their own orders and balances and can never deposit, withdraw, bridge, administer, or act as another user. Provision balances through the authenticated OMS-to-Assets-Engine deposit path with a temporary operator key, then remove that key.

**Tech Stack:** Java 21, Maven, Netty HTTP/WebSocket, gRPC, PostgreSQL read models, Aeron Assets Engine, Bash/curl/jq deployment tooling, Nginx, systemd

**Spec:** `docs/superpowers/specs/2026-10-15-spot-market-maker-accounts-design.md`

## Global Constraints

- Create exactly 10 accounts with exactly 1,000,000.00000000 of asset IDs 0, 1, 2, and 3 in each account.
- Assets Engine commands are the only permitted balance mutation; never edit PostgreSQL balance rows.
- Market-maker credentials may read and trade only their assigned user ID.
- Market-maker credentials must not deposit, internally withdraw, request NECO or bridge withdrawals, administer the OMS, or act as another user.
- Preserve existing Tyche wallet login, public market data, private WebSocket behavior, withdrawal extensions, unified fees, and production price protections.
- Never commit, log, or include full API keys in documentation.
- Do not deploy a clean local OMS JAR over the extension-bearing production JAR.
- Do not distribute credentials until all authorization and balance checks pass.

## Review Focus

- A request containing both a valid Tyche bearer and an invalid `X-API-Key` must fail rather than fall back to Tyche; Task 1 pins this behavior.
- A market-maker key using the WebSocket bearer subprotocol must authenticate as the API-key account rather than being sent to Tyche; Task 1 pins this behavior.
- A market-maker key must receive 403 on every internal, NECO, and bridge funding route, including production-only routes; Tasks 2 and 3 pin this behavior.
- Retrying interrupted provisioning must credit only the missing delta and must abort on locked, active, or over-target accounts; Task 4 pins this behavior.
- A production rollback must not silently erase or duplicate Assets Engine balances; Task 6 verifies state independently of the binary rollback.

---

### Task 1: Hybrid Tyche And API-Key Authentication

**Files:**
- Create: `openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/TycheApiKeyAuthenticationProvider.java`
- Create: `openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/TycheApiKeyAuthenticationProviderTest.java`
- Modify: `openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsApplication.java`
- Modify: `openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsConfig.java`
- Modify: `openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/OmsApplicationTycheAuthTest.java`
- Modify: `openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/OmsConfigTest.java`

**Interfaces:**
- Consumes: `AuthenticationProvider`, `ApiKeyAuthenticationProvider.parse(String, String)`, and `TycheAuthenticationProvider`.
- Produces: `TycheApiKeyAuthenticationProvider(AuthenticationProvider apiKeys, AuthenticationProvider tyche)`, `Principal authenticate(AuthenticationProvider.Headers headers)`, and auth mode `tyche-api-key`.

- [ ] **Step 1: Write failing hybrid-provider tests**

Add tests named `xApiKeySelectsApiKeyProvider`, `invalidExplicitApiKeyNeverFallsBackToTyche`, `bearerApiKeySupportsWebSocketAuthentication`, `unknownBearerFallsBackToTyche`, and `closeClosesBothProviders`. Assert exact selected principal IDs and that neither provider receives a secret through exception text.

- [ ] **Step 2: Run the focused tests and confirm failure**

Run: `mvn -pl oms-app -am -Dtest=TycheApiKeyAuthenticationProviderTest,OmsApplicationTycheAuthTest,OmsConfigTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: FAIL because `TycheApiKeyAuthenticationProvider` and `tyche-api-key` do not exist.

- [ ] **Step 3: Implement the hybrid provider**

Implement `TycheApiKeyAuthenticationProvider` so a nonblank `X-API-Key` exclusively selects the API-key provider. With no `X-API-Key`, try an API-key bearer first so `/ws/v1` works, then use Tyche only when the bearer is not a registered API key. Close both providers exactly once.

- [ ] **Step 4: Wire explicit `tyche-api-key` mode**

Update `OmsApplication.buildAuthProvider(...)` to require PostgreSQL, `TYCHE_REDIS_NAMESPACE`, and at least one configured API key; construct both providers and register the hybrid provider for shutdown. Update configuration comments and validation without changing existing `tyche`, `api-key`, `jwt`, `demo`, or `dev` modes.

- [ ] **Step 5: Run focused and module tests**

Run: `mvn -pl oms-app -am -Dtest=TycheApiKeyAuthenticationProviderTest,OmsApplicationTycheAuthTest,OmsConfigTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: PASS.

- [ ] **Step 6: Commit Task 1 only**

```bash
git add openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/TycheApiKeyAuthenticationProvider.java \
        openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsApplication.java \
        openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsConfig.java \
        openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/TycheApiKeyAuthenticationProviderTest.java \
        openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/OmsApplicationTycheAuthTest.java \
        openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/OmsConfigTest.java
git commit -m "feat: combine Tyche and market maker authentication"
```

### Task 2: Trading-Only Authorization

**Files:**
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/Principal.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/Authorizer.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/RoleBasedAuthorizer.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/rest/RestApiHandler.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc/GrpcAuth.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc/GrpcAccountService.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc/GrpcOrderService.java`
- Modify: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/auth/PrincipalTest.java`
- Modify: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/rest/RestApiHandlerAuthTest.java`
- Create: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/grpc/GrpcMarketMakerAuthorizationTest.java`

**Interfaces:**
- Consumes: authenticated `Principal` objects from Task 1.
- Produces: `Principal.ROLE_MARKET_MAKER`, `Principal.ROLE_FUNDING_ADMIN`, `Authorizer.ACTION_READ_AS_USER`, `Authorizer.ACTION_TRADE_AS_USER`, `Authorizer.ACTION_FUND_AS_USER`, and `GrpcAuth.resolveUserId(Authorizer, String, long, StreamObserver<?>)`.

- [ ] **Step 1: Write the permission-matrix tests**

Assert that a `MARKET_MAKER` principal may read and trade as itself, may not fund itself, may not perform any action as another user, and may not use admin actions. Assert that ordinary Tyche-style principals retain self-scoped read, trade, and funding behavior. For cross-user funding, require both `ANY_USER` and `FUNDING_ADMIN`; either role alone must fail, and `MARKET_MAKER` denial wins even if extra roles are accidentally assigned.

- [ ] **Step 2: Write failing REST and gRPC route tests**

For user 910001 with `MARKET_MAKER`, assert 2xx/normal delegation for self account reads and order creation, 403/no service call for `/accounts/910001/deposit` and `/accounts/910001/withdraw`, and 403 for user 910002. Assert gRPC read and trade resolution uses the matching action and cannot cross accounts.

- [ ] **Step 3: Run the focused tests and confirm failure**

Run: `mvn -pl oms-api -am -Dtest=PrincipalTest,RestApiHandlerAuthTest,GrpcMarketMakerAuthorizationTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: FAIL because the role and action-specific authorization do not exist.

- [ ] **Step 4: Implement action-specific authorization**

Keep `ACTION_ACT_AS_USER` for compatibility, add explicit read/trade/fund actions, and make funding return false for `MARKET_MAKER` regardless of self scope. Permit normal principals to fund themselves; require `ANY_USER|FUNDING_ADMIN` together for cross-user funding. Update REST handlers so GET/account/history/execution/position routes use read, order create/amend/cancel routes use trade, and internal deposit/withdraw routes use fund.

- [ ] **Step 5: Apply action-specific authorization to gRPC**

Change `GrpcAuth.resolveUserId(...)` and `canActAs(...)` to accept an action. Use read for balances/history/get/streams and trade for create/cancel. There is currently no gRPC funding method; add no new funding API.

- [ ] **Step 6: Run the focused tests and the full OMS API suite**

Run: `mvn -pl oms-api -am test`

Expected: PASS with no Tyche/self-scope regression.

- [ ] **Step 7: Commit Task 2 only**

```bash
git add openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth \
        openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/rest/RestApiHandler.java \
        openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/grpc \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/auth/PrincipalTest.java \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/rest/RestApiHandlerAuthTest.java \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/grpc/GrpcMarketMakerAuthorizationTest.java
git commit -m "feat: restrict market maker accounts to trading"
```

### Task 3: Production-Extension Compatibility Overlay

**Files:**
- Create: `openexch/production-patches/oms-market-maker-accounts/README.md`
- Create: `openexch/production-patches/oms-market-maker-accounts/src/main/java/com/openexchange/oms/api/rest/RestApiHandler.java`
- Create: `openexch/production-patches/oms-market-maker-accounts/src/test/java/com/openexchange/oms/api/rest/MarketMakerProductionRouteTest.java`
- Create: `scripts/build-spot-market-maker-accounts-patch.ps1`

**Interfaces:**
- Consumes: Task 1 hybrid authentication classes, Task 2 action constants, and the current production OMS JAR.
- Produces: a hash-recorded production JAR candidate that preserves internal deposit, direct withdrawal, bridge withdrawal, fee, deposit-address, WebSocket, and price-protection extensions.

- [ ] **Step 1: Snapshot and inspect the exact production baseline**

Download the active OMS JAR and record SHA-256. Compare its class inventory and decompiled signatures with the latest local production extraction. Stop if the running service changed after download or if the expected Tyche, withdrawal, fee, deposit-address, and WebSocket classes are absent.

- [ ] **Step 2: Write failing production-route authorization tests**

Compile a test against the production baseline that asserts `MARKET_MAKER` receives 403 and no service call for internal deposit, direct NECO withdrawal, and target-chain bridge withdrawal, while an ordinary self-scoped Tyche principal retains its existing behavior.

- [ ] **Step 3: Build the production-compatible REST overlay**

Create a source-controlled `RestApiHandler` overlay from the active production signature, preserving every extension and changing only authorization selection. Apply `ACTION_FUND_AS_USER` to all internal, NECO, and bridge money-movement routes.

- [ ] **Step 4: Implement the patch build script**

`scripts/build-spot-market-maker-accounts-patch.ps1` must accept `-BaseJar` and `-OutputJar`, compile only the reviewed overlay classes against the base JAR, update a copy, print both hashes, and refuse in-place modification. It must never package secret files.

- [ ] **Step 5: Run overlay tests and compare class inventory**

Run the overlay tests against the candidate JAR, then verify that all non-overlay class entries and resources are byte-identical to the base JAR.

Expected: PASS; only the declared class entries differ.

- [ ] **Step 6: Commit Task 3 only**

```bash
git add openexch/production-patches/oms-market-maker-accounts scripts/build-spot-market-maker-accounts-patch.ps1
git commit -m "build: package market maker OMS production overlay"
```

### Task 4: Retry-Safe Account Provisioning Tool

**Files:**
- Create: `scripts/spot/provision-market-maker-accounts.sh`
- Create: `scripts/spot/tests/provision-market-maker-accounts-test.sh`
- Create: `scripts/spot/market-maker-accounts.example.csv`

**Interfaces:**
- Consumes: localhost OMS REST, a temporary `ANY_USER|FUNDING_ADMIN` API key file, and an account CSV containing labels and selected user IDs.
- Produces: exact Assets Engine balances and a non-secret verification report; no database writes.

- [ ] **Step 1: Write failing script tests with stubbed HTTP responses**

Cover zero balances, partial retry, exact target, over-target balance, nonzero locked balance, active/open-order account, failed deposit, and redaction. Assert the script deposits only `target-current`, aborts unsafe accounts before any mutation, and never prints a key.

- [ ] **Step 2: Run the script tests and confirm failure**

Run: `bash scripts/spot/tests/provision-market-maker-accounts-test.sh`

Expected: FAIL because the provisioning script does not exist.

- [ ] **Step 3: Implement provisioning and verification**

Implement arguments `--base-url`, `--accounts-file`, `--seed-key-file`, and `--target`. Require exactly ten unique positive user IDs, assets 0-3, `jq`, HTTPS or loopback HTTP, zero locked balances, and no active orders. Use a mode-0600 curl config for the key, clean it with `trap`, and produce a report containing only labels, IDs, amounts, and pass/fail status.

- [ ] **Step 4: Run script tests and static checks**

Run: `bash scripts/spot/tests/provision-market-maker-accounts-test.sh`

Run when available: `shellcheck scripts/spot/provision-market-maker-accounts.sh scripts/spot/tests/provision-market-maker-accounts-test.sh`

Expected: PASS; no secret appears in captured output.

- [ ] **Step 5: Commit Task 4 only**

```bash
git add scripts/spot/provision-market-maker-accounts.sh \
        scripts/spot/tests/provision-market-maker-accounts-test.sh \
        scripts/spot/market-maker-accounts.example.csv
git commit -m "ops: add safe market maker account provisioning"
```

### Task 5: Colleague Trading API Guide And Verification Client

**Files:**
- Create: `docs/neco-spot-market-maker-api.zh-CN.md`
- Create: `scripts/spot/verify-market-maker-account.ps1`

**Interfaces:**
- Consumes: the deployed Nginx routes, one account ID, and one API key supplied at runtime.
- Produces: a Chinese handoff guide and a redacted read/trade/deny smoke-test report.

- [ ] **Step 1: Inspect active Nginx routes before documenting URLs**

Read the active `testapp.flydex.ai` server block and verify each REST and WebSocket upstream. Do not infer prefixes from local docs when the running route differs.

- [ ] **Step 2: Write the API guide**

Document `X-API-Key`, the WebSocket `bearer` subprotocol, account/asset/market mappings, order request fields, limit and market examples, order lifecycle, balances, executions, public order book/trades/candles/tickers, private orders/executions/balances, retries, client-order idempotency, and all forbidden funding/admin routes. Use placeholders, never real keys.

- [ ] **Step 3: Implement the verification client**

`verify-market-maker-account.ps1` accepts `-BaseUrl`, `-UserId`, and `-ApiKeyFile`; it verifies identity, four balances, self read, cross-account denial, funding denial, and withdrawal denial. An optional `-TradingSmoke` creates and cancels one small non-crossing limit order and confirms no fill or final balance change.

- [ ] **Step 4: Validate documentation routes and secret redaction**

Run: `rg -n "deposit|withdraw|admin|API Key|WebSocket|marketId|assetId" docs/neco-spot-market-maker-api.zh-CN.md`

Run the verifier against a local/mock OMS and assert no full key is printed.

- [ ] **Step 5: Commit Task 5 only**

```bash
git add docs/neco-spot-market-maker-api.zh-CN.md scripts/spot/verify-market-maker-account.ps1
git commit -m "docs: add spot market maker API handoff"
```

### Task 6: Production Deployment, Funding, And Handoff

**Files:**
- Create outside Git on server: `/home/ec2-user/flydex/neco-exchange/secrets/market-maker-api-keys.conf`
- Create outside Git on server: `/home/ec2-user/flydex/neco-exchange/secrets/market-maker-accounts.csv`
- Create outside Git locally: `F:\dydx-private\spot-market-maker-accounts-20261015.csv`

**Interfaces:**
- Consumes: Tasks 1-5 candidate JAR, scripts, current service configuration, and live Assets Engine.
- Produces: ten funded accounts, ten active trading-only keys, one restricted secret handoff file, and a complete verification report.

- [ ] **Step 1: Run all local verification**

Run: `mvn -pl oms-api,oms-app -am test`

Run: `bash scripts/spot/tests/provision-market-maker-accounts-test.sh`

Run the production-overlay tests and compare candidate/base JAR inventories.

Expected: all PASS before upload.

- [ ] **Step 2: Back up production and inspect account-ID candidates**

Back up the active OMS JAR, service environment, API-key file, and Nginx file with one timestamp. Read production Tyche mappings, orders, executions, Assets Engine projection, and existing key assignments to choose ten unused IDs outside 900000-900999 and below 1000000. This inspection is read-only.

- [ ] **Step 3: Generate and install credentials securely**

With `umask 077`, generate ten independent 256-bit keys plus one temporary operator key. Write `key:userId:MARKET_MAKER` entries to the server key file and keep the operator entry separate. Set owner to the OMS service account and mode 0600. Do not print keys.

- [ ] **Step 4: Deploy code before funding**

Install the candidate JAR atomically, set `OMS_AUTH_MODE=tyche-api-key` and `OMS_API_KEYS_FILE`, restart only OMS, then verify health, Assets Engine projection readiness, Tyche authentication, one market-maker self read, cross-account 403, internal deposit 403, internal withdrawal 403, NECO/bridge withdrawal 403, and private WebSocket authentication.

- [ ] **Step 5: Seed the ten accounts**

Temporarily add the operator key, restart OMS, and run `provision-market-maker-accounts.sh` against `127.0.0.1:28081`. Require forty exact `1,000,000.00000000` balances and zero locked amounts. On any failure, stop and do not distribute credentials.

- [ ] **Step 6: Remove the operator key and re-verify**

Remove the operator entry, restart OMS, prove the operator key returns 401, and rerun all ten account permission and balance checks. Confirm no NECO withdrawal record was created by denial tests.

- [ ] **Step 7: Run one non-crossing order smoke test per account**

Create and cancel a minimal non-crossing limit order through the normal matching path. Verify accepted/cancelled states, no fills, zero locked funds afterward, and unchanged final balances.

- [ ] **Step 8: Produce the secret and non-secret handoff artifacts**

Copy the ten account labels, user IDs, and keys to `F:\dydx-private\spot-market-maker-accounts-20261015.csv`, restrict its Windows ACL to Administrator, and provide the Chinese API guide separately. Include the candidate JAR hash, deployment timestamp, backup path, and verification report without secrets.

- [ ] **Step 9: Record rollback readiness**

Verify the backup can restore the prior JAR and auth environment. Record that binary rollback does not alter funded Assets Engine state; account funds may only be reversed through authenticated money commands after proving no open orders or locked balances.

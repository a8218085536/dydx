# Spot Cross-Chain Withdrawal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the spot withdrawal API so target chain `7260` performs the existing direct NECO ERC-20 transfer and target chain `56` performs an unlimited-approval-backed call to the deployed NECO bridge, without duplicate debit, signing, broadcast, or refund.

**Architecture:** Keep the production `services/neco-chain` binary in `watcher` mode for deposits only. Add a source-controlled Go withdrawal worker that is the sole signer, claims the existing OMS queue, serializes all direct/approval/bridge transactions under one signer lock, and persists every signed raw transaction before broadcast. Extend the production OMS JAR overlay and schema so the requested target chain survives validation, idempotency, claiming, status reads, and admin reads.

**Tech Stack:** Java 21 production-JAR overlay, PostgreSQL/Flyway-style SQL, Go 1.22, go-ethereum v1.14.11, `database/sql` with lib/pq v1.10.9, systemd, NECO EVM JSON-RPC.

**Spec:** `docs/superpowers/specs/2026-10-13-spot-cross-chain-withdrawal-design.md`

## Global Constraints

- Supported target chain IDs are exactly `7260` and `56`; omitted `targetChainId` means `7260`.
- Bridge proxy is `0x4b7a87972bf9b9dDfe771546037fA5313D035810` and the deployed method selector is `0x582bc2a8` for `transformAIChain(uint256,address,uint256,address)`.
- Never use the supplied ABI's incompatible `transformAIChain(uint256,string,uint256,address)` entry.
- Target chain validation and bridge-disabled rejection happen before OMS balance debit.
- OMS debits gross once, the worker sends net once, and definite terminal failure refunds gross once through the existing OMS path.
- The existing `services/neco-chain` process remains the deposit watcher; it must run with `NECO_CHAIN_MODE=watcher` before the new worker can become ready.
- The old and new withdrawal signers must never run concurrently.
- All direct transfers, allowance resets, unlimited approvals, and bridge calls use one serialized signer/nonce path.
- Every raw signed transaction is durably stored before the first RPC broadcast; restart recovery replays the identical bytes.
- RPC timeout or ambiguous broadcast is not refundable; keep it visible and reconcile/replay the identical transaction.
- Bridge success means the final NECO bridge-call receipt is successful at `NECO_CONFIRMATIONS`; external BSC payout is outside this service.
- Approval policy is unlimited approval (`MaxUint256`); non-zero insufficient allowance is reset to zero before approving max.
- Do not place private keys, internal keys, database credentials, RPC credentials, or real tokens in source, fixtures, logs, command output, or HTTP responses.
- Do not modify or revert unrelated dirty-worktree files. Commit only files named by the task being completed.
- A real-value direct or bridge withdrawal is a separately authorized production operation and is not part of automated deployment verification.

## Review Focus

- Duplicate `clientWithdrawalId` with a changed target chain must return a payload-conflict error and must not debit again; Task 1 pins this behavior.
- A bridge request received while bridge withdrawals are disabled must fail before `OrderService.withdraw`; Task 1 pins this behavior.
- A crash after signing but before or during broadcast must replay byte-identical raw transaction data with the original nonce; Tasks 4 and 6 pin this behavior.
- A race at cutover may leave a legacy V004 direct transaction in `signed` or `broadcasted`; Task 4 imports and completes it before new claims.
- A successful approval followed by a reverted bridge call must never be treated as token movement; Task 6 retries within the OMS attempt bound and refunds only after definite exhaustion.

---

### Task 1: Persist And Expose The Withdrawal Target Chain In OMS

**Files:**
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/api/WithdrawalTargetPolicy.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/api/NecoWithdrawalService.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/api/dto/NecoWithdrawalRequest.java`
- Modify: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/api/dto/NecoWithdrawalResponse.java`
- Modify: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/app/NecoWithdrawalService.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/persistence/NecoWithdrawalRepository.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/persistence/PostgresNecoWithdrawalRepository.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/java/com/openexchange/oms/api/rest/RestApiHandler.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/main/resources/db/migration/V012__withdrawal_target_chain.sql`
- Modify: `openexch/production-patches/oms-withdrawal-fee/src/test/java/com/openexchange/oms/app/WithdrawalFeePatchTest.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/test/java/com/openexchange/oms/app/WithdrawalTargetPatchTest.java`
- Create: `openexch/production-patches/oms-withdrawal-fee/src/test/java/com/openexchange/oms/persistence/NecoWithdrawalRepositoryPatchTest.java`

**Interfaces:**
- Consumes: the current production JAR contract, the existing `OrderService.withdraw/deposit` behavior, and the existing `neco_withdrawal_requests` V008-V011 schema.
- Produces: `NecoWithdrawalRequest.getTargetChainId()`, `NecoWithdrawalResponse.getTargetChainId()`, `NecoWithdrawalResponse.getRoute()`, `NecoWithdrawalResponse.getAttempt()`, and repository records containing `targetChainId` and `attempts` for the worker in Task 5.
- Produces: an optional `targetChainIds` filter on the internal claim endpoint so a paused bridge can leave chain `56` queued while direct chain `7260` withdrawals continue.
- Produces: `WithdrawalTargetPolicy.fromEnvironment(Map<String,String>)`, `validateBeforeDebit(long targetChainId)`, and `routeFor(long targetChainId)`.

- [ ] **Step 1: Add failing policy and service tests**

Add main-based tests named:

- `omittedTargetDefaultsToNecoDirect()` asserting `7260` and `NECO_DIRECT`.
- `acceptsConfiguredBscBridgeRoute()` asserting `56` and `NECO_BRIDGE`.
- `rejectsUnsupportedTargetBeforeDebit()` asserting target `1` throws and the withdraw-call counter remains zero.
- `rejectsBridgeTargetWhenBridgeDisabledBeforeDebit()` asserting target `56` throws and the withdraw-call counter remains zero.
- `duplicateWithDifferentTargetIsRejectedWithoutSecondDebit()` asserting the repository payload conflict is surfaced and withdraw is called once.
- `claimResponseIncludesTargetAndAttempt()` asserting the internal claim response carries the stored target chain and incremented attempt.
- `claimFilterLeavesDisallowedRouteReserved()` asserting a claim restricted to `[7260]` skips an older chain `56` row and claims the next direct row without incrementing the bridge row's attempt.

- [ ] **Step 2: Compile and run the new tests to verify they fail**

Run:

```powershell
$base = '.codex-tmp/withdrawal-fee/oms-app.current.jar'
$out = '.codex-tmp/spot-cross-chain-plan-red'
Remove-Item -Recurse -Force $out -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $out | Out-Null
$sources = Get-ChildItem 'openexch/production-patches/oms-withdrawal-fee/src/main/java','openexch/production-patches/oms-withdrawal-fee/src/test/java' -Recurse -Filter *.java
javac --release 21 -cp $base -d $out @($sources.FullName)
java -ea -cp "$out;$base" com.openexchange.oms.app.WithdrawalTargetPatchTest
java -ea -cp "$out;$base" com.openexchange.oms.persistence.NecoWithdrawalRepositoryPatchTest
```

Expected: compilation fails because target-chain fields, policy, repository signatures, and route parsing do not exist.

- [ ] **Step 3: Add the additive OMS migration**

`V012__withdrawal_target_chain.sql` must:

- add `target_chain_id BIGINT NOT NULL DEFAULT 7260`;
- add `CHECK (target_chain_id IN (7260, 56))`;
- document that old rows remain direct NECO withdrawals;
- avoid rewriting the existing amount, fee, status, or idempotency columns.

- [ ] **Step 4: Extend the request, response, service, and repository contracts**

Use these exact Java interfaces:

```java
public long NecoWithdrawalRequest.getTargetChainId()
public void NecoWithdrawalRequest.setTargetChainId(long targetChainId)
public long NecoWithdrawalResponse.getTargetChainId()
public String NecoWithdrawalResponse.getRoute()
public int NecoWithdrawalResponse.getAttempt()

NecoWithdrawalRepository.Record claim(
    String withdrawalId, long userId, String clientWithdrawalId,
    String destinationAddress, int assetId, long amountFixedPoint,
    long targetChainId)

NecoWithdrawalRepository.Record claimNextReserved(
    int maxAttempts, Set<Long> targetChainIds)
```

Keep the existing six-argument request constructor and make it assign `7260`; add a seven-argument constructor that accepts `targetChainId`. Extend `Record` with `long targetChainId` and `int attempts`, and update every SELECT/RETURNING/result-set mapping rather than supplying synthetic values.

- [ ] **Step 5: Implement pre-debit route policy and duplicate-payload protection**

`WithdrawalTargetPolicy` reads:

- `NECO_WITHDRAWAL_ALLOWED_CHAIN_IDS` with a safe missing-value default of direct-only `7260`;
- `NECO_BRIDGE_ENABLED` with a safe missing-value default of `false`.

It must reject `56` unless bridge support is enabled and `56` is allowed. `NecoWithdrawalService.requestWithdrawal` calls the policy before `withdrawals.claim` and before `orderService.withdraw`. `PostgresNecoWithdrawalRepository.ensureSamePayload` compares `target_chain_id` in addition to the existing fields.

- [ ] **Step 6: Parse and return `targetChainId` in every API path**

In `RestApiHandler.handleRequestNecoWithdrawal`, parse an optional integral JSON field; missing means `7260`, and non-integral/negative/out-of-range input is a 400 validation error. In `handleInternalNecoWithdrawalClaim`, accept an optional `targetChainIds` JSON array; omission preserves the old all-routes behavior, while an explicit array is validated against `7260` and `56` and is applied inside the `FOR UPDATE SKIP LOCKED` claim query. Ensure public POST/GET, internal claim, pending-confirmation listing, admin listing, and audit metadata use the extended response without exposing secrets.

- [ ] **Step 7: Run the focused overlay tests**

Repeat the Step 2 compile command, then run:

```powershell
java -ea -cp "$out;$base" com.openexchange.oms.app.WithdrawalFeePatchTest
java -ea -cp "$out;$base" com.openexchange.oms.app.WithdrawalTargetPatchTest
java -ea -cp "$out;$base" com.openexchange.oms.persistence.NecoWithdrawalRepositoryPatchTest
```

Expected: all fee, target policy, duplicate payload, repository mapping, claim-filter, and route parsing tests pass.

- [ ] **Step 8: Commit the OMS contract change**

```bash
git add openexch/production-patches/oms-withdrawal-fee/src
git commit -m "feat: add target chains to spot withdrawals"
```

### Task 2: Make The OMS Overlay Build Reproducible

**Files:**
- Create: `openexch/production-patches/oms-withdrawal-fee/scripts/build-overlay.ps1`
- Create: `openexch/production-patches/oms-withdrawal-fee/scripts/verify-overlay.ps1`
- Modify: `openexch/production-patches/oms-withdrawal-fee/README.md`

**Interfaces:**
- Consumes: the overlay Java and SQL sources from Task 1 plus an explicit path to the unmodified production OMS JAR.
- Produces: a patched JAR, a SHA-256 manifest, and deterministic verification commands without embedding production credentials.

- [ ] **Step 1: Add a failing verification script invocation**

Run:

```powershell
pwsh openexch/production-patches/oms-withdrawal-fee/scripts/verify-overlay.ps1 `
  -BaseJar .codex-tmp/withdrawal-fee/oms-app.current.jar
```

Expected: fail because the build and verification scripts do not exist.

- [ ] **Step 2: Implement `build-overlay.ps1`**

The script accepts `-BaseJar` and `-OutputJar`, checks the base JAR hash supplied by the operator, compiles with Java 21 against that JAR, updates only the declared overlay class/resource entries in a copied JAR, and writes `<output>.sha256`. It must not mutate the input JAR or package `.codex-tmp` content.

- [ ] **Step 3: Implement `verify-overlay.ps1`**

The script builds into a temporary directory, runs all main-based overlay tests, lists required class and migration entries from the resulting JAR, and fails if `V011` or `V012` is missing.

- [ ] **Step 4: Document the exact overlay inventory and rollback artifact**

Update the README with the added request/service/repository/REST classes, the baseline-hash requirement, the migration order, and the rule that production deployment keeps the original JAR beside the patched JAR.

- [ ] **Step 5: Run overlay verification**

Expected: the tests pass, the patched JAR contains V011 and V012, and a SHA-256 file is produced.

- [ ] **Step 6: Commit the reproducible overlay build**

```bash
git add openexch/production-patches/oms-withdrawal-fee/scripts openexch/production-patches/oms-withdrawal-fee/README.md
git commit -m "build: make OMS withdrawal overlay reproducible"
```

### Task 3: Build Worker Configuration, Amount Conversion, And Exact Calldata

**Files:**
- Create: `openexch/neco-withdrawal-main/go.mod`
- Create: `openexch/neco-withdrawal-main/go.sum`
- Create: `openexch/neco-withdrawal-main/internal/config/config.go`
- Create: `openexch/neco-withdrawal-main/internal/config/config_test.go`
- Create: `openexch/neco-withdrawal-main/internal/domain/types.go`
- Create: `openexch/neco-withdrawal-main/internal/amount/amount.go`
- Create: `openexch/neco-withdrawal-main/internal/amount/amount_test.go`
- Create: `openexch/neco-withdrawal-main/internal/evm/calldata.go`
- Create: `openexch/neco-withdrawal-main/internal/evm/calldata_test.go`

**Interfaces:**
- Consumes: the environment names already used by production `config/neco-exchange.env` plus the four bridge settings in the spec.
- Produces: `config.Load(getenv func(string) string) (Config, error)`, `amount.DecimalToUnits(string, uint8) (*big.Int, error)`, and exact calldata builders used by Task 5.

- [ ] **Step 1: Initialize the module and write failing configuration tests**

Use module path `github.com/a8218085536/dydx/openexch/neco-withdrawal-main`, Go `1.22`, go-ethereum `v1.14.11`, and lib/pq `v1.10.9`.

Tests must cover:

- RPC chain ID must be `7260`;
- allowed chains parse to exactly `7260` and `56` when bridge is enabled;
- missing/invalid bridge address fails when bridge is enabled;
- configured signer address must match the private key;
- all four token addresses and decimals are required;
- `NECO_CHAIN_MODE` must be `watcher` so the worker cannot start beside the legacy signer;
- log-safe config rendering redacts the private key, internal key, DSN, and credential-bearing RPC URL.

- [ ] **Step 2: Run configuration tests to verify they fail**

Run: `go test ./internal/config -run Test -v`

Expected: fail because `Config` and `Load` do not exist.

- [ ] **Step 3: Implement the configuration contract**

`Config` contains the existing `NECO_EVM_RPC_URL`, `NECO_CHAIN_ID`, `NECO_CONFIRMATIONS`, `NECO_OMS_INTERNAL_URL`, `NECO_OMS_INTERNAL_KEY`, `NECO_CHAIN_POSTGRES_DSN`, token address/decimal mappings, signer address/private key, poll/attempt/batch settings, and:

```text
NECO_WITHDRAWAL_ALLOWED_CHAIN_IDS=7260,56
NECO_BRIDGE_ENABLED=true
NECO_BRIDGE_ADDRESS=0x4b7a87972bf9b9dDfe771546037fA5313D035810
NECO_BRIDGE_UNLIMITED_APPROVAL=true
NECO_WITHDRAWAL_HTTP_ADDR=127.0.0.1:18087
```

Do not log raw configuration values. Startup RPC checks for chain ID, signer match, token code, bridge code, and paused state are implemented in Task 5, not in the environment parser.

- [ ] **Step 4: Write failing exact amount and calldata tests**

Assert:

- decimal strings convert without floating point and reject precision loss;
- direct transfer starts with `0xa9059cbb`;
- allowance starts with `0xdd62ed3e`;
- approve starts with `0x095ea7b3` and can encode zero and `2^256-1`;
- paused starts with `0x5c975abb`;
- bridge calldata starts with `0x582bc2a8` and ABI slots encode amount, EVM destination, chain `56`, and token address;
- no builder accepts or emits a string destination ABI.

- [ ] **Step 5: Implement amount conversion and calldata builders**

Use these signatures:

```go
func DecimalToUnits(value string, decimals uint8) (*big.Int, error)
func EncodeTransfer(to common.Address, amount *big.Int) ([]byte, error)
func EncodeAllowance(owner, spender common.Address) []byte
func EncodeApprove(spender common.Address, amount *big.Int) ([]byte, error)
func EncodePaused() []byte
func EncodeBridge(amount *big.Int, to common.Address, targetChainID uint64, token common.Address) ([]byte, error)
```

- [ ] **Step 6: Run focused and full module tests**

Run: `go test ./internal/config ./internal/amount ./internal/evm -v`

Expected: pass.

- [ ] **Step 7: Commit configuration and encoding**

```bash
git add openexch/neco-withdrawal-main
git commit -m "feat: define NECO withdrawal routes and calldata"
```

### Task 4: Add The Durable Signed-Transaction Outbox And Signer Lock

**Files:**
- Create: `openexch/neco-withdrawal-main/migrations/V005__unified_withdrawal_operations.sql`
- Create: `openexch/neco-withdrawal-main/migrations/embed.go`
- Create: `openexch/neco-withdrawal-main/internal/store/store.go`
- Create: `openexch/neco-withdrawal-main/internal/store/postgres.go`
- Create: `openexch/neco-withdrawal-main/internal/store/postgres_test.go`
- Create: `openexch/neco-withdrawal-main/internal/store/legacy.go`
- Create: `openexch/neco-withdrawal-main/internal/store/legacy_test.go`

**Interfaces:**
- Consumes: the production NECO-chain database and legacy `neco_chain_withdrawal_broadcast_outbox` V004 table.
- Produces: one lifetime signer lock and an operation outbox keyed by withdrawal, OMS claim attempt, and stage.

- [ ] **Step 1: Write failing migration and store tests**

Tests must assert:

- two processes cannot acquire the same signer advisory lock;
- `InsertSigned` is idempotent for identical bytes and rejects different bytes for the same key;
- `raw_tx` and deterministic `tx_hash` exist before status can become `broadcasted`;
- stage ordering is `approval_reset`, `approval_max`, `direct_transfer` or `bridge_call` as applicable;
- retry claim attempt `2` can create a new operation after a definite failure in attempt `1`;
- signed/broadcasted operations are returned for restart recovery;
- a legacy V004 `signed` or `broadcasted` row is imported once as attempt `0`, stage `direct_transfer`, target `7260`;
- legacy `oms_notified` rows are ignored;
- concurrent import cannot create two operation rows.

- [ ] **Step 2: Run store tests to verify they fail**

Run: `go test ./internal/store -v`

Expected: fail because the migration and store do not exist.

- [ ] **Step 3: Create the additive worker migration**

Create `neco_withdrawal_operations` with:

- `withdrawal_id`, `claim_attempt`, `stage`, and `stage_order`;
- `target_chain_id`, `asset_id`, `token_address`, destination, and raw amount;
- `nonce NUMERIC(20,0)`, raw signed transaction, and unique deterministic transaction hash;
- status limited to `signed`, `broadcasted`, `confirmed`, `reverted`, and `unknown`;
- broadcast/receipt/OMS attempt counts, errors, block number, and timestamps;
- primary key `(withdrawal_id, claim_attempt, stage)` and recovery indexes.

Do not alter or drop V004. Migration and import must be repeatable.

- [ ] **Step 4: Implement the store contract**

Use these core methods:

```go
type Store interface {
    AcquireSignerLock(ctx context.Context, signer common.Address) (io.Closer, error)
    MaxPersistedNonce(ctx context.Context, signer common.Address) (uint64, bool, error)
    GetOperation(ctx context.Context, key OperationKey) (*Operation, error)
    InsertSigned(ctx context.Context, op Operation) (*Operation, error)
    MarkBroadcasted(ctx context.Context, key OperationKey) error
    MarkReceipt(ctx context.Context, key OperationKey, receipt ReceiptResult) error
    MarkUnknown(ctx context.Context, key OperationKey, reason string) error
    ListRecoverable(ctx context.Context, limit int) ([]Operation, error)
    ImportLegacy(ctx context.Context) (int, error)
}
```

Hold the PostgreSQL advisory lock on a dedicated connection for the process lifetime. Derive a new nonce as `max(RPC pending nonce, max persisted nonce + 1)`; an existing signed operation always reuses its stored nonce and raw bytes.

- [ ] **Step 5: Implement legacy V004 adoption**

Import only direct NECO rows. Verify the stored raw transaction hashes to the stored `tx_hash`; a mismatch fails readiness and requires operator reconciliation. Recovery completes all imported rows before the worker claims a new OMS withdrawal.

- [ ] **Step 6: Run store tests**

Run: `go test ./internal/store -v`

Expected: pass against the disposable PostgreSQL test database; skip with an explicit message only when the test DSN is absent.

- [ ] **Step 7: Commit durable operation storage**

```bash
git add openexch/neco-withdrawal-main/migrations openexch/neco-withdrawal-main/internal/store
git commit -m "feat: persist NECO withdrawal transactions"
```

### Task 5: Implement The Serialized EVM Transaction Engine

**Files:**
- Create: `openexch/neco-withdrawal-main/internal/evm/backend.go`
- Create: `openexch/neco-withdrawal-main/internal/evm/transactor.go`
- Create: `openexch/neco-withdrawal-main/internal/evm/transactor_test.go`
- Create: `openexch/neco-withdrawal-main/internal/evm/preflight.go`
- Create: `openexch/neco-withdrawal-main/internal/evm/preflight_test.go`

**Interfaces:**
- Consumes: configuration and calldata from Task 3, and the durable operation store from Task 4.
- Produces: startup/live preflight checks, read-only allowance/paused calls, byte-stable signing, broadcast classification, and confirmed receipt results for Task 6.

- [ ] **Step 1: Write failing preflight tests**

Use a fake backend to cover wrong chain ID, signer mismatch, no bytecode at a required token or bridge, paused bridge, and successful preflight. A paused bridge makes bridge readiness false without disabling direct NECO withdrawals.

- [ ] **Step 2: Write failing transaction tests**

Cover:

- direct transfer destination is the user and value is zero;
- bridge transaction destination is the bridge proxy and value is zero;
- signed bytes are persisted through `InsertSigned` before `SendTransaction` is called;
- restart uses identical raw bytes and hash;
- `already known` is treated as broadcasted;
- timeout checks hash visibility and otherwise records unknown without requesting a refund;
- successful receipt waits for configured confirmation depth;
- reverted receipt is definite failure;
- nonces remain strictly increasing across direct, reset, max approval, and bridge stages.

- [ ] **Step 3: Run EVM tests to verify they fail**

Run: `go test ./internal/evm -run 'TestPreflight|TestTransactor' -v`

Expected: fail because the backend and transactor do not exist.

- [ ] **Step 4: Define the backend and transaction interfaces**

```go
type Backend interface {
    ChainID(context.Context) (*big.Int, error)
    CodeAt(context.Context, common.Address, *big.Int) ([]byte, error)
    CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
    PendingNonceAt(context.Context, common.Address) (uint64, error)
    SuggestGasPrice(context.Context) (*big.Int, error)
    EstimateGas(context.Context, ethereum.CallMsg) (uint64, error)
    SendTransaction(context.Context, *types.Transaction) error
    TransactionByHash(context.Context, common.Hash) (*types.Transaction, bool, error)
    TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
    BlockNumber(context.Context) (uint64, error)
}
```

`Transactor.PrepareAndPersist(ctx, OperationSpec)`, `Broadcast(ctx, Operation)`, and `Receipt(ctx, Operation)` must keep signing, persistence, and classification separate enough for tests to prove ordering.

- [ ] **Step 5: Implement startup and per-bridge preflight**

Startup verifies chain `7260`, configured signer, token bytecode, bridge bytecode, and bridge paused state. Re-check `paused()` immediately before signing each bridge call. Direct withdrawals remain available if the bridge is paused, but the worker must not consume a bridge attempt merely because it is paused.

- [ ] **Step 6: Implement signing, persistence, broadcast, and receipt depth**

Use EIP-155 signing for chain `7260`, RPC-estimated gas with a bounded safety margin, and the configured gas price source. Never sign a replacement transaction for an existing operation key. Never mark OMS broadcasted for approval transactions.

- [ ] **Step 7: Run EVM tests**

Run: `go test ./internal/evm -v`

Expected: pass.

- [ ] **Step 8: Commit the EVM engine**

```bash
git add openexch/neco-withdrawal-main/internal/evm
git commit -m "feat: serialize NECO withdrawal transactions"
```

### Task 6: Orchestrate OMS Claims, Approvals, Direct Transfers, Bridge Calls, And Refunds

**Files:**
- Create: `openexch/neco-withdrawal-main/internal/oms/client.go`
- Create: `openexch/neco-withdrawal-main/internal/oms/client_test.go`
- Create: `openexch/neco-withdrawal-main/internal/worker/processor.go`
- Create: `openexch/neco-withdrawal-main/internal/worker/processor_test.go`
- Create: `openexch/neco-withdrawal-main/internal/worker/recovery.go`
- Create: `openexch/neco-withdrawal-main/internal/worker/recovery_test.go`

**Interfaces:**
- Consumes: the extended OMS contract from Task 1, worker config/calldata from Task 3, store from Task 4, and EVM engine from Task 5.
- Produces: `OMSClient`, `Processor.Recover`, and `Processor.RunOnce` for the executable in Task 7.
- Produces: `OMSClient.Claim(ctx context.Context, targetChainIDs []uint64)`, where `[7260]` keeps bridge rows untouched when bridge readiness is false.

- [ ] **Step 1: Write failing OMS client tests**

Use `httptest.Server` to verify the internal key is sent only as a header, is never present in logged errors, and these calls decode target chain and attempt correctly:

```text
POST /api/v1/internal/withdrawals/neco/claim
POST /api/v1/internal/withdrawals/neco/{id}/broadcasted
POST /api/v1/internal/withdrawals/neco/{id}/retry
POST /api/v1/internal/withdrawals/neco/{id}/failed
POST /api/v1/internal/withdrawals/neco/{id}/refund
POST /api/v1/internal/withdrawals/neco/{id}/confirmed
GET  /api/v1/internal/withdrawals/neco/broadcasted
```

- [ ] **Step 2: Implement the OMS client and run its tests**

Run: `go test ./internal/oms -v`

Expected: pass, including redaction assertions and bounded request timeouts.

- [ ] **Step 3: Write failing processor route tests**

Cover:

- target `7260` creates only `direct_transfer`, then broadcasts and confirms it;
- target `56` with sufficient allowance skips approvals;
- zero allowance creates and confirms `approval_max`, re-reads allowance, then creates bridge call;
- non-zero insufficient allowance creates and confirms `approval_reset`, then `approval_max`, then bridge call;
- when the bridge is already paused, claims use `[7260]` and leave chain `56` rows reserved without incrementing attempts;
- if the bridge pauses after a chain `56` row is claimed but before final signing, the processor releases that row once to `reserved`, switches subsequent claims to `[7260]`, and consumes no EVM nonce;
- approval hashes remain only in the operation store and final bridge hash is sent to OMS;
- successful final receipt calls OMS confirmed once;
- definite revert calls OMS retry while attempts remain;
- definite failure at the attempt limit calls failed then refund once;
- ambiguous broadcast never calls retry, failed, refund, or confirmed;
- invalid amount precision fails before signing;
- a second `RunOnce` cannot duplicate debit, operation, broadcast notification, or refund.

- [ ] **Step 4: Write failing recovery tests**

Cover recovery ordering:

1. import legacy V004 operations;
2. replay signed operations byte-for-byte;
3. reconcile broadcasted/unknown operation hashes;
4. reconcile OMS rows already marked broadcasted;
5. only then claim a new reserved withdrawal.

Assert a legacy direct transaction completes as target `7260`, and an unresolved hash blocks refund but remains visible in health details.

- [ ] **Step 5: Implement `Processor` as a single-threaded state machine**

Use these entry points:

```go
func (p *Processor) Recover(ctx context.Context) error
func (p *Processor) RunOnce(ctx context.Context) error
func (p *Processor) Process(ctx context.Context, w oms.Withdrawal) error
```

For bridge withdrawals, always re-read allowance after approval confirmation and before bridge signing. If an approval succeeded but the bridge later definitely fails, retry the bridge route under the next OMS claim attempt; approvals are not reported as user withdrawal success.

- [ ] **Step 6: Implement bounded terminal failure and exact-once refund coordination**

Only a definite preparation/signing error before persistence or a reverted receipt may enter retry/exhaustion. On exhaustion call `failed`, then `refund`; rely on the existing OMS `failed -> refunding -> refunded` idempotency. An unknown broadcast remains in the operation table and OMS `broadcasting` until reconciled.

- [ ] **Step 7: Run worker tests including race detection**

Run:

```bash
go test ./internal/oms ./internal/worker -v
go test -race ./internal/...
```

Expected: pass with no race reports.

- [ ] **Step 8: Commit orchestration**

```bash
git add openexch/neco-withdrawal-main/internal/oms openexch/neco-withdrawal-main/internal/worker
git commit -m "feat: process direct and bridge withdrawals"
```

### Task 7: Add The Executable, Health Checks, Deployment Artifacts, And Cutover Runbook

**Files:**
- Create: `openexch/neco-withdrawal-main/cmd/neco-withdrawal/main.go`
- Create: `openexch/neco-withdrawal-main/internal/health/server.go`
- Create: `openexch/neco-withdrawal-main/internal/health/server_test.go`
- Create: `openexch/neco-withdrawal-main/deploy/neco-withdrawal.service`
- Create: `openexch/neco-withdrawal-main/deploy/install.sh`
- Create: `openexch/neco-withdrawal-main/deploy/neco-exchange.env.example`
- Create: `openexch/neco-withdrawal-main/README.md`
- Create: `openexch/neco-withdrawal-main/docs/production-cutover.md`

**Interfaces:**
- Consumes: all worker packages and the production root `/home/ec2-user/flydex/neco-exchange`.
- Produces: Linux `services/neco-withdrawal`, loopback health/readiness on `127.0.0.1:18087`, a systemd unit, an additive installer, and a no-double-signer cutover/rollback procedure.

- [ ] **Step 1: Write failing health tests**

Assert `/health` reports process liveness and `/ready` is false unless all core conditions are true: chain/signer/token preflight passed, signer advisory lock is held, recovery completed, OMS is reachable, and `NECO_CHAIN_MODE=watcher`. A paused bridge is reported as route `56` unavailable while route `7260` and overall worker readiness remain available. Readiness details may include withdrawal IDs and transaction hashes but never secrets or raw signed transactions.

- [ ] **Step 2: Implement command lifecycle and health endpoints**

`main` loads config, connects to PostgreSQL and RPC, runs migrations, acquires the signer lock, performs preflight and recovery, starts the loopback health server, then polls `RunOnce`. SIGTERM stops claiming new work and allows the current persisted operation to reach a recoverable state before exit.

- [ ] **Step 3: Run module tests and build Linux artifact**

Run:

```bash
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o dist/neco-withdrawal ./cmd/neco-withdrawal
```

Expected: tests and vet pass; `dist/neco-withdrawal` is a Linux amd64 static executable.

- [ ] **Step 4: Add systemd and installer artifacts**

The unit must:

- run as `ec2-user` from `/home/ec2-user/flydex/neco-exchange`;
- read `/home/ec2-user/flydex/neco-exchange/config/neco-exchange.env`;
- execute `/home/ec2-user/flydex/neco-exchange/services/neco-withdrawal`;
- restart on failure with a bounded delay;
- order after PostgreSQL/network and `neco-exchange.service`;
- avoid command-line secrets.

The installer copies only the binary, V005 migration, unit, and non-secret documentation. It must not edit the live env automatically.

- [ ] **Step 5: Write the production cutover runbook**

The runbook must use this order:

1. record current hashes/statuses and back up OMS JAR, both withdrawal binaries, env, V004/V005 and V008-V012 migrations, OMS PostgreSQL, and NECO-chain PostgreSQL;
2. verify the current signer address, chain `7260`, bridge code, `paused=false`, and per-token allowance without printing secrets;
3. apply additive OMS V012 and worker V005 migrations;
4. deploy/restart the patched OMS and verify omitted target defaults to `7260`, unsupported targets reject before debit, and target `56` is accepted only when configured;
5. set `NECO_CHAIN_MODE=watcher` and restart the managed `chain` process;
6. verify the legacy process reports watcher readiness and deposits still advance;
7. inspect V004 plus OMS `broadcasting`/`broadcasted` rows, then start the new worker so recovery completes before readiness;
8. verify exactly one withdrawal signer by process list, worker advisory lock, health, and a nonce sample;
9. run a zero-value/read-only API validation and deposit regression; do not initiate real-value withdrawal without separate approval.

Rollback order must stop and disable `neco-withdrawal` first, verify its signer lock is released and no unknown operations remain, restore the previous OMS JAR/config if needed, then return `services/neco-chain` to `full`. Never reverse those two signer steps.

- [ ] **Step 6: Run deployment artifact checks**

Run shell syntax checks, `systemd-analyze verify` where available, confirm all example values are placeholders, and search the module for key/secret patterns before packaging.

- [ ] **Step 7: Commit executable and operations artifacts**

```bash
git add openexch/neco-withdrawal-main
git commit -m "ops: package unified NECO withdrawal worker"
```

### Task 8: Final Integration Review And Staging Gate

**Files:**
- Modify: `openexch/neco-withdrawal-main/README.md`
- Modify: `openexch/production-patches/oms-withdrawal-fee/README.md`
- Test: all files introduced in Tasks 1-7

**Interfaces:**
- Consumes: the complete OMS overlay, worker, migrations, and runbook.
- Produces: a reviewed release candidate and evidence bundle; it does not authorize a real-value production withdrawal.

- [ ] **Step 1: Run all automated checks from a clean output directory**

Run overlay verification, `go test ./...`, `go test -race ./internal/...`, `go vet ./...`, SQL migration tests, and the Linux build. Expected: all pass.

- [ ] **Step 2: Run mock-RPC integration scenarios**

Exercise direct success, each approval branch, bridge success, approval revert, bridge revert, RPC timeout, restart after signed persistence, restart after broadcast, and legacy V004 import. Assert exact OMS status transitions and operation counts.

- [ ] **Step 3: Review the immutable bridge assumptions**

Verify against live NECO read-only RPC immediately before release: proxy address has code, implementation slot is readable, selector `0x582bc2a8` remains callable by simulation, bridge is not paused, chain ID is `7260`, and token contracts have code. Record hashes/addresses, not credentials.

- [ ] **Step 4: Perform a security and money-movement review**

Review authorization headers, secret redaction, destination validation, amount precision, fee/gross/net handling, duplicate request behavior, nonce serialization, approval scope, bridge pause behavior, ambiguous broadcast handling, exact-once refund, and rollback ordering.

- [ ] **Step 5: Produce a release manifest**

Record source commit, OMS base/patched JAR hashes, worker binary hash, migration hashes, bridge/token addresses, configuration variable names, and test results. Do not include configuration values classified as secrets.

- [ ] **Step 6: Commit final documentation corrections**

```bash
git add openexch/neco-withdrawal-main/README.md openexch/production-patches/oms-withdrawal-fee/README.md
git commit -m "docs: finalize cross-chain withdrawal release gate"
```

## Self-Review Result

- Spec coverage: public API, allowed chains, backward compatibility, unlimited approval, bridge selector, fee preservation, durable raw transactions, retries, unknown outcomes, watcher mode, deployment, and rollback each map to a task.
- Step scan: each task has an explicit failing test, implementation boundary, verification command, and commit.
- Type consistency: OMS returns `targetChainId` and `attempt`; worker operation keys use the same withdrawal ID plus claim attempt and stage throughout storage, EVM, and processor tasks.
- Review focus coverage: payload conflict and disabled bridge are in Task 1; crash replay and legacy adoption are in Task 4; post-approval bridge failure is in Task 6.
- Proportion: exact signatures and safety decisions are specified; implementation bodies remain for the executor.

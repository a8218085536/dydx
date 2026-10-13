# Spot Cross-Chain Withdrawal Design

Date: 2026-10-13

## Goal

Extend the existing spot withdrawal API so a user can withdraw either directly
on Neco or through the deployed Neco bridge contract. The spot exchange remains
responsible only for the transaction submitted on Neco. External-chain event
listeners, manual review, and external-chain payout are outside this system.

## Confirmed Production Facts

- Neco EVM chain ID: `7260`.
- BSC chain ID: `56`.
- Bridge proxy: `0x4b7a87972bf9b9dDfe771546037fA5313D035810`.
- The deployed proxy implementation exposes
  `transformAIChain(uint256,address,uint256,address)` with selector
  `0x582bc2a8`.
- The supplied ABI's `string to` overload does not match the deployed
  implementation and must not be used.
- The bridge is a UUPS upgradeable contract and is currently not paused.
- Spot assets are ERC-20 tokens configured as USDT, NETH, NBNB, and NBTC.
- The existing production `services/neco-chain` binary supports deposit watcher
  and full withdrawal modes, but its source directory was not retained in the
  current repository or release archive.

## Architecture

Keep the deployed `services/neco-chain` binary, but run it in `watcher` mode so
it continues deposit scanning and OMS deposit crediting without signing
withdrawals.

Add a source-controlled spot withdrawal worker to this repository. It becomes
the only process allowed to claim and sign spot withdrawals. It reuses the
existing OMS withdrawal queue, signer configuration, token mapping, Neco RPC,
confirmation depth, retry policy, and durable PostgreSQL state.

The new worker handles both routes through one serialized signer pipeline:

- Target chain `7260`: direct ERC-20 transfer on Neco.
- Target chain `56`: ERC-20 approval when needed, followed by the bridge call.

This prevents nonce races between direct withdrawals, approval transactions,
and bridge transactions.

## Public API

The existing endpoint remains:

```text
POST /api/v1/neco/withdrawals
GET  /api/v1/neco/withdrawals/{withdrawalId}
```

The POST request gains an optional field:

```json
{
  "userId": 1000446,
  "clientWithdrawalId": "client-generated-id",
  "destinationAddress": "0x1111111111111111111111111111111111111111",
  "assetId": 0,
  "amount": "100.00000000",
  "targetChainId": 56
}
```

Rules:

- An omitted `targetChainId` defaults to `7260` for backward compatibility.
- Allowed target chain IDs are exactly `7260` and `56`.
- Any other target chain is rejected before the user balance is debited.
- `destinationAddress` must be a valid EVM address for both routes.
- The response includes `targetChainId` and a route value of `NECO_DIRECT` or
  `NECO_BRIDGE`.
- `txHash` is the direct transfer hash for `7260` and the final bridge-call hash
  for `56`. Approval hashes remain available in the worker audit database.

## Configuration

Add and validate these settings:

```text
NECO_WITHDRAWAL_ALLOWED_CHAIN_IDS=7260,56
NECO_BRIDGE_ENABLED=true
NECO_BRIDGE_ADDRESS=0x4b7a87972bf9b9dDfe771546037fA5313D035810
NECO_BRIDGE_UNLIMITED_APPROVAL=true
```

Existing signer, RPC, asset address, token decimal, confirmation, PostgreSQL,
and OMS internal-auth settings remain authoritative. Secrets must not be copied
into source files, logs, HTTP responses, or test fixtures.

When bridge withdrawals are enabled, startup must fail if:

- chain ID `56` is absent from the allowed target chains;
- the bridge address is invalid or has no deployed code;
- the local RPC chain ID is not `7260`;
- the signer address does not match the configured private key; or
- a required token address or decimal configuration is missing.

The worker checks the bridge paused state at startup and immediately before
signing the final bridge transaction. A paused bridge leaves a claimed request
retryable and does not produce a bridge transaction.

## Persistence

OMS withdrawal storage gains a non-null `target_chain_id` column with default
`7260`. Existing rows therefore retain direct-withdrawal behavior. The value is
captured when the withdrawal is created and returned to the worker when it
claims the row.

The worker stores a durable transaction record for every signed operation. A
bridge withdrawal may contain these ordered operations:

1. optional allowance reset to zero;
2. optional unlimited approval;
3. bridge call.

Each operation records its stage, nonce, raw signed transaction, deterministic
transaction hash, broadcast state, receipt state, and last error before the raw
transaction is sent to RPC. A restart replays the same raw transaction instead
of signing another transaction for the same stage.

The public OMS status stays `broadcasting` while approval stages are running.
It changes to `broadcasted` only after the direct transfer or final bridge call
is broadcast. It changes to `confirmed` only after the final transaction has a
successful receipt at the configured confirmation depth.

## Amounts And Fees

The existing unified withdrawal fee remains unchanged:

- OMS debits the requested gross amount once.
- The worker sends the calculated net amount.
- A failed withdrawal refunds the original gross amount once.

For bridge withdrawals, the net fixed-point amount is converted using the
configured token decimals and passed to both allowance comparison and
`transformAIChain`.

## Allowance And Bridge Flow

For target chain `56`:

1. Read `allowance(withdrawer, bridge)` for the selected asset token.
2. If the allowance covers the net raw amount, skip approval.
3. If allowance is zero, approve `MaxUint256` and wait for a successful,
   sufficiently confirmed receipt.
4. If allowance is non-zero but insufficient, first approve zero and confirm
   it, then approve `MaxUint256` and confirm it.
5. Re-read allowance before the bridge call. It must cover the net raw amount.
6. Call `transformAIChain(netRawAmount, destinationAddress, 56, tokenAddress)`
   with zero native value.
7. Persist and broadcast the bridge transaction through the same serialized
   signer path used for direct withdrawals.
8. Mark the withdrawal confirmed after the Neco receipt succeeds at the
   configured confirmation depth.

The system does not wait for or infer an external BSC transfer. A successful
Neco bridge receipt is the terminal on-chain success condition for spot.

Unlimited approval is an explicit operational choice. Because the bridge is
upgradeable, its owner and future implementations can exercise the allowance.
`NECO_BRIDGE_ENABLED=false` provides an emergency stop for new bridge
withdrawals, but it does not revoke an existing allowance. An operator runbook
must include an explicit per-token allowance revocation procedure.

## Failure And Recovery

- Validation and configuration failures happen before ledger debit whenever
  they can be known synchronously.
- Preparation or signing failures before a transaction hash is persisted return
  the withdrawal to the bounded retry flow.
- RPC timeout after persistence or broadcast is an unknown outcome. The worker
  checks the recorded transaction hash and replays only the identical raw
  transaction.
- A reverted approval or final transaction is a definite failure.
- A successful approval followed by a failed bridge call does not move tokens;
  the withdrawal can be retried within the configured attempt limit.
- Once attempts are exhausted after a definite failure, OMS uses the existing
  idempotent refund path to return the gross amount exactly once.
- An unresolved broadcast outcome is not refunded automatically. It remains
  visible for reconciliation to avoid paying both on chain and internally.
- A successful bridge receipt is never automatically refunded because the
  external payout process is outside spot ownership.

## Compatibility

- Existing clients that omit `targetChainId` continue to withdraw directly on
  Neco.
- Existing rows default to chain `7260`.
- Deposit scanning and crediting remain in the existing binary's watcher mode.
- Existing direct-withdrawal API authentication, fee calculation, idempotency,
  balance reservation, admin listing, and refund behavior remain unchanged.

## Testing

Automated coverage must include:

- old requests defaulting to chain `7260`;
- accepted `7260` and `56` requests;
- unsupported chain rejection before balance debit;
- duplicate client withdrawal IDs preserving the original target chain;
- direct ERC-20 transfer encoding and amount conversion;
- bridge selector and ABI encoding matching `0x582bc2a8`;
- sufficient allowance skipping approval;
- zero allowance producing one unlimited approval;
- non-zero insufficient allowance producing reset then unlimited approval;
- receipt failure, RPC timeout, retry exhaustion, and restart replay;
- no duplicate debit, transaction signing, broadcast, or refund;
- serialized nonces across direct, approval, and bridge transactions;
- legacy watcher continuing deposit crediting with withdrawal signing disabled.

Before production deployment, run unit tests, database migration tests, a local
mock-RPC integration test, package doctor checks, and a staging withdrawal test.
A real-value bridge test requires separate operator authorization and must use a
small amount.

## Deployment

1. Back up the production OMS JAR, `services/neco-chain`, configuration,
   migrations, and both relevant PostgreSQL databases.
2. Deploy the OMS migration and API changes.
3. Deploy the new withdrawal worker and its migration.
4. Set the existing chain adapter to `watcher` mode.
5. Configure the allowed chains and bridge settings without printing secrets.
6. Restart the spot stack and verify that exactly one withdrawal signer is
   active.
7. Verify deposit scanning, public withdrawal validation, worker health,
   database state, and RPC connectivity.
8. Perform a direct-withdrawal regression test in staging.
9. Perform a small bridge withdrawal only after explicit operator approval.

Rollback restores the prior OMS JAR, migrations-compatible configuration, and
`services/neco-chain` full mode only after the new withdrawal worker is stopped.
At no point may both withdrawal signers run concurrently.

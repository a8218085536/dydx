# Close Position NECO Settlement Design

## Context

FlyDEX uses NECO as the only user-facing deposit and withdrawal chain. Users pay DEDX or another configured NECO asset before trading. Tyche prices that asset in USD, funds the user's dYdX proxy wallet subaccount with quote collateral, and submits native dYdX perpetual orders through `dydxtrading`.

The current close-position flow can submit a reduce-only close order and reconcile dYdX state, but it does not yet complete the business settlement that sends DEDX back to the user's NECO wallet after a position is closed.

## Decision

When a user closes a dYdX position, Tyche should pay the user on NECO with DEDX after the close is confirmed. The dYdX collateral does not have to be withdrawn from the subaccount immediately in the first production version. Instead, Tyche must mark the corresponding closed-position value as already settled and no longer user-available.

This avoids repeated dYdX withdraw transactions while preserving the core safety rule:

> Once Tyche pays DEDX to the user's NECO wallet for a closed position, the corresponding dYdX USD value becomes system-reserved and cannot be reused by the user for future orders.

Future orders still require a fresh NECO-side funding step. A user's remaining dYdX subaccount collateral is not treated as free user balance by Tyche.

## Recommended Shape

Use a two-phase implementation.

Phase 1 keeps the existing single configured dYdX subaccount number and adds a business settlement lock:

1. User calls `POST /tyche/api/trading/close-position`.
2. Tyche submits a reduce-only dYdX close order.
3. Reconcile confirms the close order tx has `code = 0` and the target position is closed.
4. Tyche computes the settlement USD amount for the closed position.
5. Tyche prices DEDX through the configured NECO price route.
6. Tyche creates a unique settlement record keyed by close order ID.
7. Tyche credits an internal DEDX settlement amount with an idempotent ledger ref.
8. Tyche freezes a NECO DEDX withdrawal and asks the NECO hot wallet worker to broadcast it.
9. The settlement status advances through `pending`, `dedx_credited`, `neco_payout_broadcasted`, and `neco_payout_paid`.
10. The dYdX USD value represented by the settlement is recorded as `system_reserved`, not user-available.

Phase 2 adds position buckets using dYdX subaccount numbers:

1. Each independent open-position workflow receives a `positionBucketId`.
2. Each bucket maps to one user dYdX address plus one subaccount number.
3. Funding, orders, close orders, reconcile, and settlement all carry that subaccount number.
4. Closing one bucket only settles that bucket and does not inspect or affect other buckets.
5. Idle bucket collateral can later be swept back to the system wallet in batch.

Phase 2 is the cleanest way to support "two positions and two open orders, close only one and settle only that one" without collateral ambiguity.

## Data Model

Add a `TradingSettlement` record:

- `id`
- `userId`
- `necoAddress`
- `dydxAccount`
- `subaccountNumber`
- `market`
- `closeOrderId`
- `closeClientOrderId`
- `sourcePositionId`
- `settlementUsd`
- `settlementAsset`, initially `DEDX`
- `settlementAmount`
- `assetUsdPrice`
- `priceSnapshotId`
- `dydxReservedQuantums`
- `ledgerCreditRefId`
- `ledgerWithdrawalId`
- `necoTxHash`
- `status`
- `reason`
- `createdAt`
- `updatedAt`

The settlement idempotency key is:

```text
dydx-close-settlement:<userId>:<closeOrderId>
```

Ledger credit ref:

```text
dydx-close-settlement:<userId>:<closeOrderId>:credit
```

NECO withdrawal is keyed from the settlement ID so retries do not create another payout.

## Settlement Amount Rule

Tyche must support an explicit close settlement mode:

- `margin_only`: settle the recorded margin value for the closed Tyche position. This is the first production-test mode because the current flow already records the user's NECO-funded margin amount.
- `full_accounting`: settle `releasedMargin + realizedPnl - fees` after dYdX fills, fees, and realized PnL are reliably synced.

The initial implementation uses `margin_only` unless config sets `full_accounting`.

For `full_accounting`, settlement uses the closed position accounting known by Tyche:

```text
settlementUsd = releasedMargin + realizedPnl - fees
```

If `full_accounting` is configured but fills, fees, or realized PnL are missing, Tyche must mark the settlement `manual_review_required` and must not pay automatically.

DEDX payout calculation:

```text
dedxAmountRaw = floor(settlementUsd / DEDX_USD_price * 10^DEDX_decimals)
```

Rounding must always favor the system by flooring the payout, never rounding up.

## Safety Rules

Tyche must not pay if:

- the close order is not confirmed on dYdX,
- the target position is still open,
- the settlement record already reached a terminal payout state,
- the NECO address is missing or does not match the user's active login/binding,
- DEDX/USD price is missing or stale,
- settlement USD is zero or negative,
- the same close order already has a payout,
- the subaccount state is ambiguous and the bucket model is not available.

For Phase 1 with one shared subaccount, Tyche should be conservative:

- if the closed position can be uniquely mapped to a Tyche position record, settle that position amount only;
- do not expose leftover dYdX collateral as user balance;
- multiple orders in the same market inside the same subaccount are treated as one net dYdX position;
- if the user needs independent same-market positions, Phase 2 bucketed subaccounts are required.

For Phase 2, settlement is per bucket and can be automatic because the subaccount boundary isolates funds.

## Required Code Changes

Tyche:

- add `TradingSettlement` domain model and Redis store;
- add close settlement service;
- wire settlement into `TradingReconcileService` after close confirmation;
- add DEDX payout conversion using the configured asset mapping and price route;
- reuse ledger credit and NECO withdrawal worker for payout;
- fix NECO withdrawal worker wiring to use `necoBackend` in dYdX mode;
- add admin/query endpoints for settlement status and manual retry;
- extend order/position records with optional `subaccountNumber` and `positionBucketId`.

dydxtrading:

- no mandatory Phase 1 chain withdraw is required;
- later add `/subaccount/withdraw` using `MsgWithdrawFromSubaccount` for batch sweeping system-reserved collateral.

Frontend/API:

- close-position response should include settlement status when available;
- add a settlement query endpoint so the UI can show `closing`, `closed`, `payout_broadcasted`, `paid`, or `manual_review`.

## Tests

Add tests for:

- close order confirmed and position closed creates exactly one settlement;
- duplicate reconcile does not pay twice;
- DEDX payout floors decimals;
- stale/missing DEDX price blocks payout;
- missing NECO address blocks payout;
- NECO worker uses the NECO backend in dYdX mode;
- Phase 1 does not treat dYdX leftover collateral as user-available;
- Phase 2 bucket close only settles the matching subaccount.

## Deployment Notes

This feature does not require changing the dYdX chain node. It affects Tyche first. `dydxtrading` only needs changes when enabling optional batch withdrawal from subaccounts back to the system dYdX wallet.

The first production-test version should leave automatic dYdX collateral sweeping disabled and rely on the Tyche settlement ledger to prevent user reuse.

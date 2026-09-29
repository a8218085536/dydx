# Admin Settlement And Reconciliation APIs

These APIs are protected by `X-Tyche-Internal-Key` and should only be called by the admin backend/BFF, not directly from browser code.

## List Manual Review Settlements

```http
POST /tyche/api/trading/settlement/manual-review/list
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

Request:

```json
{
  "userId": "optional-user-id"
}
```

If `userId` is empty, Tyche lists all settlements currently in `manual_review_required`.

## Manual Payout

Use this after an operator has checked the dYdX fill/accounting result and decided how much DEDX should be paid back on NECO.

```http
POST /tyche/api/trading/settlement/manual-payout
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

Request:

```json
{
  "userId": "user-id",
  "settlementId": "settlement-id",
  "settlementUsd": "20.5",
  "settlementAmount": "5125000000000000000",
  "reason": "admin confirmed fills and fees"
}
```

Notes:

- `settlementAmount` is the raw settlement asset amount, for example DEDX with 18 decimals.
- The operation is idempotent. A repeated call for the same settlement will not duplicate the ledger credit/freeze.
- Tyche writes the confirmed values back to the settlement and then reuses the normal payout path: ledger credit, ledger withdrawal freeze, and optional NECO withdrawal worker.
- If the settlement was already `rejected`, Tyche returns an error and will not pay it.

## Reject Settlement

Use this when the operator decides that no payout should be made for a manual review settlement.

```http
POST /tyche/api/trading/settlement/reject
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

Request:

```json
{
  "userId": "user-id",
  "settlementId": "settlement-id",
  "reason": "duplicate fill / invalid accounting"
}
```

Rejected settlements are terminal for automatic retry and manual payout.

## Reconciliation Report

```http
POST /tyche/api/trading/reconciliation/report
Content-Type: application/json
X-Tyche-Internal-Key: <internalAPIKey>
```

Request examples:

```json
{
  "userId": "user-id",
  "settlementId": "settlement-id"
}
```

```json
{
  "status": "manual_review_required"
}
```

The report checks the local Tyche settlement record against ledger withdrawals, NECO deposits, and synced dYdX fill accounting. It can flag:

- `manual_review_required`
- `settlement_failed`
- `settlement_rejected`
- `invalid_settlement_usd`
- `invalid_settlement_amount`
- `ledger_withdrawal_missing`
- `ledger_withdrawal_mismatch`
- `ledger_withdrawal_not_paid`
- `missing_neco_tx_hash`
- `missing_dydx_reserved_quantums`
- `fill_lookup_failed`
- `fill_event_id_missing`
- `duplicate_fill`
- `invalid_fill_accounting`
- `negative_realized_pnl`
- `neco_deposit_pending`
- `neco_deposit_invalid_amount`
- `neco_deposit_missing_ledger_ref`
- `neco_deposit_lookup_failed`
- `ledger_ref_lookup_failed`

Severity values are `info`, `warning`, and `critical`.

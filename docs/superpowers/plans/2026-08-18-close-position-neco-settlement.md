# Close Position NECO Settlement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Phase 1 close-position settlement so a confirmed dYdX close order pays DEDX to the user's NECO wallet exactly once while marking the corresponding dYdX value as system-reserved.

**Architecture:** Tyche owns the business settlement. dYdX collateral can remain on the subaccount in Phase 1, but Tyche creates an idempotent settlement record, credits and freezes a DEDX payout in Ledger, then broadcasts the NECO withdrawal through the existing hot-wallet worker. Reconcile is the trigger because it already confirms dYdX tx and subaccount state.

**Tech Stack:** Go, Gin, Redigo Redis stores, existing Tyche Ledger, existing NECO ERC20 withdrawal worker, existing dYdX reconcile worker, existing NECO V2 price oracle.

## Global Constraints

- NECO remains the only user-facing deposit and withdrawal chain.
- dYdX does not become a user withdrawal surface.
- Phase 1 does not require immediate `MsgWithdrawFromSubaccount`.
- Once a close settlement pays DEDX on NECO, the corresponding dYdX USD value is system-reserved and not user-available.
- The first implementation uses `margin_only` close settlement mode unless config sets `full_accounting`.
- In `full_accounting`, missing fills, fees, or realized PnL must produce `manual_review_required`, not an automatic payout.
- DEDX payout calculation floors the raw amount and never rounds up.
- Redis structs must include both `json` and `redis` tags.
- This workspace has no `.git` directory, so commit steps are replaced with local checkpoint notes unless a repository is restored.

---

## Scope

This plan implements Phase 1 only:

- close settlement records;
- idempotent NECO DEDX payout after close confirmation;
- business reservation of the dYdX value;
- query and retry endpoints;
- NECO withdrawal worker wiring fix.

This plan does not implement bucketed subaccounts or dYdX subaccount sweeping. Those remain separate follow-up work.

## File Structure

- Create `tyche/tyche/trading_settlement.go`
  - Domain constants, errors, structs, store interface, memory store, calculator helpers.
- Create `tyche/tyche/trading_close_settlement.go`
  - `CloseSettlementService` that turns a closed dYdX order into a single NECO DEDX payout.
- Create `tyche/model/trading/settlement_store.go`
  - Redis implementation of `TradingSettlementStore`.
- Modify `tyche/model/trading/keys.go`
  - Add settlement Redis keys and indexes.
- Modify `tyche/tyche/config.go`
  - Add close settlement config fields and validate defaults in dYdX mode.
- Modify `tyche/tyche/trading.go`
  - Add `SubaccountNumber` and `PositionBucketID` to `TradingOrder`.
  - Persist the configured subaccount number on open and close orders.
- Modify `tyche/model/trading/store.go`
  - Persist `subaccountNumber` and `positionBucketId`.
- Modify `tyche/tyche/trading_reconcile.go`
  - Trigger close settlement after a close order is confirmed filled/closed.
- Modify `tyche/cmd/tyche/app/app.go`
  - Wire settlement store/service and fix NECO withdrawal worker to use `necoBackend`.
- Modify `tyche/cmd/tyche/app/api_trading.go`
  - Add settlement query and retry handlers.
- Modify `tyche/cmd/tyche/app/router.go`
  - Add protected settlement endpoints.
- Modify `tyche/deploy/tyche.dydx.single-node.toml`, `tyche/deploy/tyche.mock.toml`, and `tyche/cmd/tyche/tyche.dydx.example.toml`
  - Add close settlement config.
- Modify `tyche/docs/frontend-trading-api.md`
  - Document settlement statuses and frontend polling.

### Task 1: Settlement Domain, Config, and Store

**Files:**
- Create: `tyche/tyche/trading_settlement.go`
- Create: `tyche/model/trading/settlement_store.go`
- Modify: `tyche/model/trading/keys.go`
- Modify: `tyche/tyche/config.go`
- Test: `tyche/tyche/trading_settlement_test.go`
- Test: `tyche/model/trading/store_test.go`

**Interfaces:**
- Produces:
  - `type TradingSettlement struct`
  - `type TradingSettlementStore interface`
  - `func NewMemoryTradingSettlementStore() *MemoryTradingSettlementStore`
  - `func NormalizeCloseSettlementMode(string) string`
- Consumes:
  - Existing `parseLedgerAmount`, `parseDecimalRat`, `ratToDecimalString`, `AssetMapping`, `AssetMapper`.

- [ ] **Step 1: Write failing domain tests**

Add to `tyche/tyche/trading_settlement_test.go`:

```go
func TestMemoryTradingSettlementStoreCreateIfAbsentIsIdempotent(t *testing.T) {
	store := NewMemoryTradingSettlementStore()
	request := TradingSettlement{
		ID:                 "settlement-1",
		UserID:             "user-1",
		NECOAddress:        "0xabc",
		DYDXAccount:        "dydx1abc",
		SubaccountNumber:   0,
		Market:             "ETH-USD",
		CloseOrderID:       "close-order-1",
		CloseClientOrderID: "client-close-1",
		SettlementAsset:    "DEDX",
		SettlementUSD:      "20",
		SettlementAmount:   "5000000000000000000",
		Status:             TradingSettlementStatusPending,
	}

	first, created, err := store.CreateIfAbsent(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("first CreateIfAbsent() = (%+v, %v, %v), want created", first, created, err)
	}
	second, created, err := store.CreateIfAbsent(context.Background(), request)
	if err != nil || created {
		t.Fatalf("second CreateIfAbsent() = (%+v, %v, %v), want existing", second, created, err)
	}
	if second.ID != first.ID {
		t.Fatalf("second ID = %q, want %q", second.ID, first.ID)
	}
}

func TestCloseSettlementConfigDefaultsToMarginOnly(t *testing.T) {
	conf := Config{
		ChainType:                 ChainTypeDYDX,
		InternalAPIKey:            strings.Repeat("a", 32),
		DYDXTradingURL:            "http://127.0.0.1:8090/dydx/trading",
		DYDXChainID:               "flydex-mainnet-1",
		DYDXHeightURL:             "http://127.0.0.1:26657/status",
		DYDXMarkets:               []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1}},
		ExpectedMD5Password:       "x",
		AllowedIP:                 "*",
		CloseSettlementAsset:      "",
		CloseSettlementMode:       "",
		CloseSettlementAssetDecimals: 0,
	}
	if err := conf.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if conf.CloseSettlementMode != CloseSettlementModeMarginOnly {
		t.Fatalf("CloseSettlementMode = %q", conf.CloseSettlementMode)
	}
	if conf.CloseSettlementAsset != "DEDX" {
		t.Fatalf("CloseSettlementAsset = %q", conf.CloseSettlementAsset)
	}
	if conf.CloseSettlementAssetDecimals != 18 {
		t.Fatalf("CloseSettlementAssetDecimals = %d", conf.CloseSettlementAssetDecimals)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche -run "TestMemoryTradingSettlementStoreCreateIfAbsentIsIdempotent|TestCloseSettlementConfigDefaultsToMarginOnly"
```

Expected: fail because the settlement types and config fields do not exist.

- [ ] **Step 3: Add settlement domain code**

Create `tyche/tyche/trading_settlement.go` with:

```go
package tyche

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	CloseSettlementModeMarginOnly    = "margin_only"
	CloseSettlementModeFullAccounting = "full_accounting"

	TradingSettlementStatusPending              = "pending"
	TradingSettlementStatusDEDXCredited          = "dedx_credited"
	TradingSettlementStatusNECOPayoutBroadcasted = "neco_payout_broadcasted"
	TradingSettlementStatusNECOPayoutPaid        = "neco_payout_paid"
	TradingSettlementStatusManualReviewRequired  = "manual_review_required"
	TradingSettlementStatusFailed                = "failed"

	defaultCloseSettlementAsset         = "DEDX"
	defaultCloseSettlementAssetDecimals = 18
)

var (
	ErrTradingSettlementNotFound        = errors.New("trading settlement not found")
	ErrTradingSettlementDuplicatePayout = errors.New("trading settlement duplicate payout")
	ErrTradingSettlementManualReview    = errors.New("trading settlement manual review required")
	ErrTradingSettlementInvalidAmount   = errors.New("trading settlement invalid amount")
)

type TradingSettlement struct {
	ID                    string `json:"id" redis:"id"`
	UserID                string `json:"userId" redis:"userId"`
	NECOAddress           string `json:"necoAddress" redis:"necoAddress"`
	DYDXAccount           string `json:"dydxAccount" redis:"dydxAccount"`
	SubaccountNumber      uint32 `json:"subaccountNumber" redis:"subaccountNumber"`
	Market                string `json:"market" redis:"market"`
	CloseOrderID          string `json:"closeOrderId" redis:"closeOrderId"`
	CloseClientOrderID    string `json:"closeClientOrderId" redis:"closeClientOrderId"`
	SourcePositionID      string `json:"sourcePositionId" redis:"sourcePositionId"`
	SettlementUSD         string `json:"settlementUsd" redis:"settlementUsd"`
	SettlementAsset       string `json:"settlementAsset" redis:"settlementAsset"`
	SettlementAmount      string `json:"settlementAmount" redis:"settlementAmount"`
	AssetUSDPrice         string `json:"assetUsdPrice" redis:"assetUsdPrice"`
	PriceSnapshotID       string `json:"priceSnapshotId" redis:"priceSnapshotId"`
	DYDXReservedQuantums   string `json:"dydxReservedQuantums" redis:"dydxReservedQuantums"`
	LedgerCreditRefID      string `json:"ledgerCreditRefId" redis:"ledgerCreditRefId"`
	LedgerWithdrawalID     string `json:"ledgerWithdrawalId" redis:"ledgerWithdrawalId"`
	NECOTxHash            string `json:"necoTxHash" redis:"necoTxHash"`
	Status                string `json:"status" redis:"status"`
	Reason                string `json:"reason" redis:"reason"`
	CreatedAt             int64  `json:"createdAt" redis:"createdAt"`
	UpdatedAt             int64  `json:"updatedAt" redis:"updatedAt"`
}

type TradingSettlementStore interface {
	CreateIfAbsent(context.Context, TradingSettlement) (TradingSettlement, bool, error)
	Save(context.Context, TradingSettlement) error
	GetByID(context.Context, string) (TradingSettlement, bool, error)
	GetByCloseOrderID(context.Context, string, string) (TradingSettlement, bool, error)
	ListByUser(context.Context, string) ([]TradingSettlement, error)
}

func NormalizeCloseSettlementMode(mode string) string {
	switch normalizeAsset(mode) {
	case "FULL_ACCOUNTING":
		return CloseSettlementModeFullAccounting
	default:
		return CloseSettlementModeMarginOnly
	}
}
```

Add the memory store in the same file using maps:

```go
type MemoryTradingSettlementStore struct {
	mu           sync.Mutex
	byID         map[string]TradingSettlement
	byCloseOrder map[string]string
	byUser       map[string]map[string]bool
}

func NewMemoryTradingSettlementStore() *MemoryTradingSettlementStore {
	return &MemoryTradingSettlementStore{
		byID:         map[string]TradingSettlement{},
		byCloseOrder: map[string]string{},
		byUser:       map[string]map[string]bool{},
	}
}

func closeSettlementKey(userID, closeOrderID string) string {
	return userID + ":" + closeOrderID
}
```

The methods must assign `uuid.NewString()` when `settlement.ID` is empty, set `CreatedAt` and `UpdatedAt` with `time.Now().Unix()`, and index by `userID:closeOrderID`.

- [ ] **Step 4: Add config fields**

Modify `tyche/tyche/config.go`:

```go
CloseSettlementMode          string `toml:"closeSettlementMode" json:"closeSettlementMode"`
CloseSettlementAsset         string `toml:"closeSettlementAsset" json:"closeSettlementAsset"`
CloseSettlementAssetDecimals uint8  `toml:"closeSettlementAssetDecimals" json:"closeSettlementAssetDecimals"`
```

In dYdX config init:

```go
c.CloseSettlementMode = NormalizeCloseSettlementMode(c.CloseSettlementMode)
if c.CloseSettlementAsset == "" {
	c.CloseSettlementAsset = defaultCloseSettlementAsset
}
if c.CloseSettlementAssetDecimals == 0 {
	c.CloseSettlementAssetDecimals = defaultCloseSettlementAssetDecimals
}
```

- [ ] **Step 5: Add Redis settlement keys and store**

Modify `tyche/model/trading/keys.go`:

```go
func (k keygen) Settlement(id string) string {
	return fmt.Sprintf("%s:settlement:%s", k.Namespace(), id)
}

func (k keygen) CloseOrderSettlement(userID, closeOrderID string) string {
	return fmt.Sprintf("%s:close-settlement:%s:%s", k.Namespace(), userID, closeOrderID)
}

func (k keygen) UserSettlements(userID string) string {
	return fmt.Sprintf("%s:user-settlements:%s", k.Namespace(), userID)
}
```

Create `tyche/model/trading/settlement_store.go` with `CreateIfAbsent`, `Save`, `GetByID`, `GetByCloseOrderID`, and `ListByUser`. Use `WATCH` on `keys.CloseOrderSettlement(userID, closeOrderID)`, `HSET` all fields, `SET` the close-order index, and `SADD` the user set.

- [ ] **Step 6: Run Task 1 tests**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche ./model/trading -run "Settlement|CloseSettlement"
```

Expected: pass.

- [ ] **Step 7: Checkpoint**

Run:

```powershell
cd F:\dydx
git status --short
```

Expected in this workspace: `fatal: not a git repository`. Record changed files in the final task summary.

### Task 2: Close Settlement Service

**Files:**
- Create: `tyche/tyche/trading_close_settlement.go`
- Test: `tyche/tyche/trading_close_settlement_test.go`

**Interfaces:**
- Consumes:
  - `TradingSettlementStore`
  - `LedgerService`
  - `NECOWithdrawalWorker`
  - `AssetUSDOracle`
  - `WalletBindingStore.GetByUserID(context.Context, string) (WalletBinding, error)`
- Produces:
  - `type CloseSettlementService struct`
  - `type CloseSettlementConfig struct`
  - `type CloseSettlementRequest struct`
  - `func NewCloseSettlementService(...) *CloseSettlementService`
  - `func (s *CloseSettlementService) SettleClosedPosition(context.Context, CloseSettlementRequest) (TradingSettlement, error)`

- [ ] **Step 1: Write failing service tests**

Add to `tyche/tyche/trading_close_settlement_test.go`:

```go
type fixedCloseSettlementAssetOracle struct {
	asset      string
	price      string
	snapshotID string
}

func (o fixedCloseSettlementAssetOracle) AssetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if normalizeAsset(asset) != normalizeAsset(o.asset) {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	return AssetPriceSnapshot{
		ID:       o.snapshotID,
		Asset:    normalizeAsset(o.asset),
		USDAsset: "USD",
		USDPrice: o.price,
	}, nil
}

func closeSettlementOrderFixture() TradingOrder {
	return TradingOrder{
		ID:                  "close-1",
		UserID:              "user-1",
		NECOAddress:         "0x0000000000000000000000000000000000000001",
		DYDXAccount:         "dydx1abc",
		ClientOrderID:       "close-client-1",
		Market:             "ETH-USD",
		MarginAsset:        "DEDX",
		MarginUSD:          "20",
		DYDXFundingQuantums: "20000000",
		ClosePosition:      true,
		Status:             TradingOrderStatusClosed,
	}
}

func closeSettlementReconcileFixture() DYDXOrderReconcileResult {
	return DYDXOrderReconcileResult{
		Tx: DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "0xtx"},
		Subaccount: DYDXSubaccountReconcileSnapshot{
			Found: true,
			PerpetualPositions: nil,
		},
	}
}

func TestCloseSettlementServicePaysDEDXOnceForMarginOnlyClose(t *testing.T) {
	ledger := NewLedgerService(NewMemoryLedgerStore())
	settlements := NewMemoryTradingSettlementStore()
	withdrawals := NewMemoryNECOWithdrawalStore()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	worker, err := NewNECOWithdrawalWorker(ledger, withdrawals, &fakeNECOWithdrawalBackend{
		chainID:  big.NewInt(7260),
		gasPrice: big.NewInt(2),
		nonce:    1,
	}, NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		},
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	service := NewCloseSettlementService(settlements, ledger, worker, fixedCloseSettlementAssetOracle{asset: "DEDX", price: "4", snapshotID: "price-1"}, nil, CloseSettlementConfig{
		Mode:                         CloseSettlementModeMarginOnly,
		SettlementAsset:              "DEDX",
		SettlementAssetDecimals:      18,
		CollateralQuantumsPerUSD:     "1000000",
	})

	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: closeSettlementOrderFixture(),
		Reconcile: closeSettlementReconcileFixture(),
	})
	if err != nil {
		t.Fatalf("SettleClosedPosition() error = %v", err)
	}
	if settlement.Status != TradingSettlementStatusNECOPayoutBroadcasted {
		t.Fatalf("status = %q", settlement.Status)
	}
	if settlement.SettlementAmount != "5000000000000000000" {
		t.Fatalf("settlement amount = %q", settlement.SettlementAmount)
	}

	again, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{CloseOrder: closeSettlementOrderFixture(), Reconcile: closeSettlementReconcileFixture()})
	if err != nil {
		t.Fatalf("duplicate SettleClosedPosition() error = %v", err)
	}
	if again.ID != settlement.ID {
		t.Fatalf("duplicate settlement ID = %q, want %q", again.ID, settlement.ID)
	}
}
```

Add a second test:

```go
func TestCloseSettlementServiceFullAccountingWithoutFillRequiresManualReview(t *testing.T) {
	service := NewCloseSettlementService(NewMemoryTradingSettlementStore(), NewLedgerService(NewMemoryLedgerStore()), nil, fixedCloseSettlementAssetOracle{asset: "DEDX", price: "4", snapshotID: "price-1"}, nil, CloseSettlementConfig{
		Mode:                    CloseSettlementModeFullAccounting,
		SettlementAsset:         "DEDX",
		SettlementAssetDecimals: 18,
	})
	settlement, err := service.SettleClosedPosition(context.Background(), CloseSettlementRequest{
		CloseOrder: TradingOrder{
			ID:             "close-1",
			UserID:         "user-1",
			NECOAddress:    "0x0000000000000000000000000000000000000001",
			ClientOrderID:  "close-client-1",
			Market:        "ETH-USD",
			MarginUSD:     "20",
			ClosePosition: true,
			Status:        TradingOrderStatusClosed,
		},
		Reconcile: DYDXOrderReconcileResult{Tx: DYDXTxReconcileSnapshot{Found: true, Code: 0}},
	})
	if !errors.Is(err, ErrTradingSettlementManualReview) {
		t.Fatalf("error = %v, want manual review", err)
	}
	if settlement.Status != TradingSettlementStatusManualReviewRequired {
		t.Fatalf("status = %q", settlement.Status)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche -run "TestCloseSettlementService"
```

Expected: fail because `CloseSettlementService` does not exist.

- [ ] **Step 3: Implement the service**

Create `tyche/tyche/trading_close_settlement.go`:

```go
type CloseSettlementConfig struct {
	Mode                     string
	SettlementAsset          string
	SettlementAssetDecimals  uint8
	CollateralQuantumsPerUSD string
}

type CloseSettlementRequest struct {
	CloseOrder TradingOrder
	Position   TradingPosition
	Fill       TradingFill
	Reconcile  DYDXOrderReconcileResult
}

type CloseSettlementService struct {
	store      TradingSettlementStore
	ledger     *LedgerService
	withdrawer *NECOWithdrawalWorker
	oracle     AssetUSDOracle
	bindings   interface {
		GetByUserID(context.Context, string) (WalletBinding, error)
	}
	conf CloseSettlementConfig
}
```

Implement:

```go
func NewCloseSettlementService(store TradingSettlementStore, ledger *LedgerService, withdrawer *NECOWithdrawalWorker, oracle AssetUSDOracle, bindings interface {
	GetByUserID(context.Context, string) (WalletBinding, error)
}, conf CloseSettlementConfig) *CloseSettlementService
```

`SettleClosedPosition` must:

1. return existing settlement if `GetByCloseOrderID` finds a terminal or in-progress record;
2. require `CloseOrder.ClosePosition == true`;
3. require `CloseOrder.Status == TradingOrderStatusClosed` or `TradingOrderStatusFilled`;
4. require reconcile tx found with `Code == 0`;
5. require no matching open perpetual position for the close order market when reconcile subaccount data is present;
6. resolve NECO address from `CloseOrder.NECOAddress`, falling back to active wallet binding by user ID;
7. compute settlement USD:
   - `margin_only`: use `CloseOrder.MarginUSD`, fallback to `CloseOrder.MarginAmount` only if it is already a USD decimal value;
   - `full_accounting`: require non-empty `Fill.ReleasedMargin`, `Fill.Fee`, and `Fill.RealizedPNL`;
8. price `SettlementAsset` with `oracle.AssetUSDPrice(ctx, settlementAsset)`;
9. compute raw DEDX amount with floor rounding;
10. create settlement with status `pending`;
11. credit ledger with `CreditMockDeposit` using ref `dydx-close-settlement:<userId>:<closeOrderId>:credit`;
12. update settlement to `dedx_credited`;
13. freeze ledger withdrawal to the NECO address;
14. call `withdrawer.ProcessWithdrawal(ctx, withdrawal.ID)` when withdrawer is configured;
15. save `neco_payout_broadcasted` plus tx hash if broadcast succeeds;
16. save `manual_review_required` when automatic payout is blocked by configured accounting mode.

Add helper:

```go
func settlementAssetAmountRaw(settlementUSD, assetUSDPrice string, decimals uint8) (string, error)
```

Use `big.Rat` and floor by integer division:

```go
scaled := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
num := new(big.Rat).Mul(usd, new(big.Rat).SetInt(scaled))
num.Quo(num, price)
amount := new(big.Int).Quo(num.Num(), num.Denom())
```

- [ ] **Step 4: Run Task 2 tests**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche -run "TestCloseSettlementService|TestSettlementAssetAmountRaw"
```

Expected: pass.

- [ ] **Step 5: Checkpoint**

Run:

```powershell
cd F:\dydx
git status --short
```

Expected in this workspace: `fatal: not a git repository`.

### Task 3: Reconcile Integration and NECO Backend Wiring

**Files:**
- Modify: `tyche/tyche/trading_reconcile.go`
- Modify: `tyche/tyche/trading_reconcile_test.go`
- Modify: `tyche/cmd/tyche/app/app.go`
- Modify: `tyche/cmd/tyche/app/ledger_store_test.go` or create `tyche/cmd/tyche/app/app_withdrawal_test.go`

**Interfaces:**
- Consumes:
  - `CloseSettlementService.SettleClosedPosition`
  - `TradingSettlementStore`
- Produces:
  - `func (s *TradingReconcileService) SetCloseSettlementService(*CloseSettlementService)`
  - Reconcile calls settlement exactly once after close order reaches `closed`.

- [ ] **Step 1: Write failing reconcile settlement test**

Add to `tyche/tyche/trading_reconcile_test.go`:

```go
func TestTradingReconcileSettlesClosedPositionOnce(t *testing.T) {
	orderStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	sync := NewTradingSyncService(syncStore, orderStore, ledger)
	source := &recordingDYDXOrderReconcileSource{
		result: DYDXOrderReconcileResult{
			Tx:            DYDXTxReconcileSnapshot{Found: true, Code: 0, TxHash: "0xclose"},
			StatefulOrder: DYDXStatefulOrderReconcileSnapshot{Checked: true, Found: false},
			Subaccount:   DYDXSubaccountReconcileSnapshot{Found: true},
		},
	}
	settler := &recordingCloseSettlementService{}
	reconcile := NewTradingReconcileService(source, sync, TradingReconcileConfig{
		SubaccountNumber: 0,
		OrderFlags:       DYDXOrderFlagsLongTerm,
		Markets:          []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000"}},
	})
	reconcile.SetCloseSettlementService(settler)
	order := TradingOrder{
		ID:            "close-1",
		UserID:        "user-1",
		NECOAddress:   "0x0000000000000000000000000000000000000001",
		DYDXAccount:   "dydx1abc",
		ClientOrderID: "close-client-1",
		Market:        "ETH-USD",
		MarginUSD:     "20",
		ClosePosition: true,
		Status:        TradingOrderStatusCloseBroadcasted,
		DYDXOrderTxHash: "0xclose",
	}

	if _, err := reconcile.ReconcileOrder(context.Background(), order); err != nil {
		t.Fatalf("ReconcileOrder() error = %v", err)
	}
	if settler.calls != 1 {
		t.Fatalf("settlement calls = %d, want 1", settler.calls)
	}
	if settler.request.CloseOrder.Status != TradingOrderStatusClosed {
		t.Fatalf("settled order status = %q", settler.request.CloseOrder.Status)
	}
}
```

- [ ] **Step 2: Write failing app wiring test for NECO backend**

Add a test that constructs `App{backend:nil, necoBackend:fakeBackend, conf:...}` and asserts `newNECOWithdrawalWorker()` returns non-nil. The test fails with current code because it checks `app.backend`.

```go
func TestAppNewNECOWithdrawalWorkerUsesNECOBackendInDYDXMode(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	app := &App{
		ledger:      tyche.NewLedgerService(tyche.NewMemoryLedgerStore()),
		necoBackend: &appFakeNECOWithdrawalBackend{
			chainID:  big.NewInt(7260),
			gasPrice: big.NewInt(2),
			nonce:    1,
		},
		conf: &Config{Tyche: &tyche.Config{NECOWallet: tyche.NECOWalletConfig{
			HotWalletAddress: hotWallet.Hex(),
			HotWalletKey:     crypto.FromECDSA(key),
			TokenContracts: map[string]string{
				"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
			},
		}}},
	}
	if worker := app.newNECOWithdrawalWorker(); worker == nil {
		t.Fatalf("newNECOWithdrawalWorker() returned nil")
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche ./cmd/tyche/app -run "TestTradingReconcileSettlesClosedPositionOnce|TestAppNewNECOWithdrawalWorkerUsesNECOBackendInDYDXMode"
```

Expected: fail because reconcile has no settlement hook and app uses `backend`.

- [ ] **Step 4: Implement reconcile hook**

Modify `TradingReconcileService`:

```go
type closeSettlementHandler interface {
	SettleClosedPosition(context.Context, CloseSettlementRequest) (TradingSettlement, error)
}

type TradingReconcileService struct {
	source           DYDXOrderReconcileSource
	sync             *TradingSyncService
	closeSettlement  closeSettlementHandler
	subaccountNumber uint32
	orderFlags       uint32
	markets          map[string]DYDXMarketConfig
}

func (s *TradingReconcileService) SetCloseSettlementService(service closeSettlementHandler) {
	s.closeSettlement = service
}
```

After `syncStatus(... TradingOrderStatusFilled ...)` for close orders, set the local order status using `orderStatusForDYDXResult(order, TradingOrderStatusFilled)`, then call:

```go
if s.closeSettlement != nil {
	_, err := s.closeSettlement.SettleClosedPosition(ctx, CloseSettlementRequest{
		CloseOrder: order,
		Reconcile:  result,
	})
	if err != nil && !errors.Is(err, ErrTradingSettlementDuplicatePayout) && !errors.Is(err, ErrTradingSettlementManualReview) {
		return result, err
	}
}
```

- [ ] **Step 5: Fix NECO backend wiring and app construction**

Change `newNECOWithdrawalWorker` in `tyche/cmd/tyche/app/app.go` from:

```go
if app.backend == nil || app.conf == nil || app.conf.Tyche == nil || len(app.conf.Tyche.NECOWallet.HotWalletKey) == 0 {
	return nil
}
worker, err := tyche.NewNECOWithdrawalWorker(app.ledger, necowithdrawals.NewRedisStore(), app.backend, app.conf.Tyche.NECOWallet)
```

to:

```go
if app.necoBackend == nil || app.conf == nil || app.conf.Tyche == nil || len(app.conf.Tyche.NECOWallet.HotWalletKey) == 0 {
	return nil
}
worker, err := tyche.NewNECOWithdrawalWorker(app.ledger, necowithdrawals.NewRedisStore(), app.necoBackend, app.conf.Tyche.NECOWallet)
```

In `App.Init`, construct the settlement service after `app.necoWithdrawal` is created, then pass it to reconcile:

```go
app.necoWithdrawal = app.newNECOWithdrawalWorker()
app.closeSettlement = app.newCloseSettlementService()
if app.tradingReconcileRun != nil {
	app.tradingReconcileRun.SetCloseSettlementService(app.closeSettlement)
}
```

If the worker does not expose a setter, add:

```go
func (w *TradingReconcileWorker) SetCloseSettlementService(service closeSettlementHandler) {
	if w != nil && w.service != nil {
		w.service.SetCloseSettlementService(service)
	}
}
```

- [ ] **Step 6: Run Task 3 tests**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche ./cmd/tyche/app -run "TestTradingReconcileSettlesClosedPositionOnce|TestAppNewNECOWithdrawalWorkerUsesNECOBackendInDYDXMode"
```

Expected: pass.

- [ ] **Step 7: Checkpoint**

Run:

```powershell
cd F:\dydx
git status --short
```

Expected in this workspace: `fatal: not a git repository`.

### Task 4: Settlement API, Config Templates, and Frontend Docs

**Files:**
- Modify: `tyche/cmd/tyche/app/api_trading.go`
- Modify: `tyche/cmd/tyche/app/router.go`
- Modify: `tyche/cmd/tyche/app/api_trading_test.go`
- Modify: `tyche/deploy/tyche.dydx.single-node.toml`
- Modify: `tyche/deploy/tyche.mock.toml`
- Modify: `tyche/cmd/tyche/tyche.dydx.example.toml`
- Modify: `tyche/docs/frontend-trading-api.md`

**Interfaces:**
- Consumes:
  - `TradingSettlementStore.GetByID`
  - `TradingSettlementStore.GetByCloseOrderID`
  - `TradingSettlementStore.ListByUser`
  - `CloseSettlementService.SettleClosedPosition`
- Produces:
  - `POST /tyche/api/trading/settlement/get`
  - `POST /tyche/api/trading/settlement/list`
  - `POST /tyche/api/trading/settlement/retry`

- [ ] **Step 1: Write failing API tests**

Add to `tyche/cmd/tyche/app/api_trading_test.go`:

```go
func TestTradingSettlementGetAPI(t *testing.T) {
	app := newTestApp(t)
	app.tradingSettlements = tyche.NewMemoryTradingSettlementStore()
	_, _, err := app.tradingSettlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		ID:              "settlement-1",
		UserID:          "user-1",
		CloseOrderID:    "close-1",
		SettlementAsset: "DEDX",
		Status:          tyche.TradingSettlementStatusNECOPayoutBroadcasted,
	})
	if err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}

	rec := postJSON(t, app, "/tyche/api/trading/settlement/get", map[string]interface{}{
		"userId":      "user-1",
		"settlementId":"settlement-1",
	}, withInternalKey())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"neco_payout_broadcasted"`)) {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```powershell
cd F:\dydx\tyche
go test ./cmd/tyche/app -run "TestTradingSettlement"
```

Expected: fail because routes and fields do not exist.

- [ ] **Step 3: Add app fields and handlers**

In `App` add:

```go
tradingSettlements tyche.TradingSettlementStore
closeSettlement    *tyche.CloseSettlementService
```

Add request structs:

```go
type tradingSettlementGetRequest struct {
	UserID        string `json:"userId"`
	SettlementID  string `json:"settlementId"`
	CloseOrderID  string `json:"closeOrderId"`
}

type tradingSettlementListRequest struct {
	UserID string `json:"userId"`
}

type tradingSettlementRetryRequest struct {
	UserID       string `json:"userId"`
	SettlementID string `json:"settlementId"`
}
```

Add handlers:

```go
func (app *App) TradingSettlementGet(c *ginx.Context)
func (app *App) TradingSettlementList(c *ginx.Context)
func (app *App) TradingSettlementRetry(c *ginx.Context)
```

`get` should accept either `settlementId` or `closeOrderId`. `retry` should load the settlement, load the close order from trading store, and call `CloseSettlementService` again.

- [ ] **Step 4: Add routes**

Modify `tyche/cmd/tyche/app/router.go`:

```go
g.POST("/tyche/api/trading/settlement/get", ginx.WrapHandler(app.protected(app.TradingSettlementGet)))
g.POST("/tyche/api/trading/settlement/list", ginx.WrapHandler(app.protected(app.TradingSettlementList)))
g.POST("/tyche/api/trading/settlement/retry", ginx.WrapHandler(app.protected(app.TradingSettlementRetry)))
```

- [ ] **Step 5: Add config templates**

Add to all dYdX Tyche toml templates:

```toml
closeSettlementMode = "margin_only"
closeSettlementAsset = "DEDX"
closeSettlementAssetDecimals = 18
```

- [ ] **Step 6: Update frontend API docs**

In `tyche/docs/frontend-trading-api.md`, add a section:

```markdown
### Query Close Settlement

POST `/tyche/api/trading/settlement/get`

Request:

```json
{
  "userId": "user-uuid",
  "closeOrderId": "close-order-id"
}
```

Statuses:

| Status | Meaning |
| --- | --- |
| `pending` | Close confirmed, payout not prepared yet |
| `dedx_credited` | Internal DEDX payout balance created |
| `neco_payout_broadcasted` | NECO DEDX transfer broadcasted |
| `neco_payout_paid` | NECO receipt confirmed |
| `manual_review_required` | Missing accounting data or ambiguous state |
| `failed` | Payout failed and needs retry or manual handling |
```
```

- [ ] **Step 7: Run Task 4 tests**

Run:

```powershell
cd F:\dydx\tyche
go test ./cmd/tyche/app -run "TestTradingSettlement|TestTradingClosePositionAPI"
```

Expected: pass.

- [ ] **Step 8: Checkpoint**

Run:

```powershell
cd F:\dydx
git status --short
```

Expected in this workspace: `fatal: not a git repository`.

### Task 5: Full Verification and Deployment Artifact

**Files:**
- Modify only if tests expose missing wiring:
  - `tyche/deploy/update-tyche-dydx-from-tmp.sh`
  - `tyche/README.md`

**Interfaces:**
- Consumes:
  - Tasks 1-4.
- Produces:
  - Verified Tyche test suite.
  - Updated deployment zip with replacement-friendly layout.

- [ ] **Step 1: Run focused settlement tests**

Run:

```powershell
cd F:\dydx\tyche
go test ./tyche ./model/trading ./cmd/tyche/app -run "Settlement|ClosePosition|Reconcile|NECOWithdrawal"
```

Expected: pass.

- [ ] **Step 2: Run full Tyche tests**

Run:

```powershell
cd F:\dydx\tyche
go test ./...
```

Expected: pass.

- [ ] **Step 3: Build Tyche**

Run:

```powershell
cd F:\dydx\tyche
go build -o build/tyche.exe ./cmd/tyche
```

Expected: binary builds.

- [ ] **Step 4: Confirm no accidental dYdX chain-node changes**

Run:

```powershell
cd F:\dydx
git diff -- protocol
```

Expected in this workspace: if git is unavailable, manually confirm this plan's file list does not include `protocol`.

- [ ] **Step 5: Package Tyche**

Use the existing packaging approach for `tyche-dydx-deploy-20260815-final.zip`. The archive must contain one top-level `tyche/` folder and must not create timestamped source folders during extraction.

Expected: server update script can replace `/home/ec2-user/flydex/dydx/tyche` and preserve `/home/ec2-user/flydex/dydx/tyche/tyche.toml`.

- [ ] **Step 6: Server smoke commands after deployment**

On server:

```bash
sudo systemctl restart tyche-dydx.service
sudo systemctl status tyche-dydx.service --no-pager -l
curl -s -X POST http://127.0.0.1:8081/tyche/api/trading/settlement/list \
  -H 'Content-Type: application/json' \
  -H "X-Tyche-Internal-Key: $KEY" \
  -d '{"userId":"USER_ID"}' | jq .
```

Expected: service active, settlement endpoint returns `{"code":200,...}`.

## Self-Review Checklist

- Spec coverage:
  - NECO-only user withdrawal is covered by using `NECOWithdrawalWorker`.
  - dYdX collateral not immediately withdrawn is covered by settlement status and no user balance credit from dYdX leftover collateral.
  - Idempotency is covered by `CreateIfAbsent`, ledger credit ref, and withdrawal ID linkage.
  - `margin_only` and `full_accounting` behavior is covered by config and tests.
  - Redis tag risk is covered by explicit `redis` tags in `TradingSettlement`.
  - NECO backend wiring fix is covered in Task 3.
- Marker scan:
  - No open markers or undefined helper functions remain in the plan.
- Type consistency:
  - `TradingSettlementStore` is created in Task 1 and consumed in Tasks 2-4.
  - `CloseSettlementService` is created in Task 2 and consumed in Tasks 3-4.
  - Route names in Task 4 match docs and smoke commands.

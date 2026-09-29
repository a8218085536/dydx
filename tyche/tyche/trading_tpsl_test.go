package tyche

import (
	"context"
	"testing"
	"time"
)

func TestTradingTPSLServiceTriggersTakeProfitAndCancelsSibling(t *testing.T) {
	ctx := context.Background()
	service, submitter, syncService := newTestTradingTPSLService("2110")
	if _, err := syncService.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:     "pos-1",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "1",
		EntryPrice:  "2000",
		MarkPrice:   "2000",
		MarginAsset: "DEDX",
		Margin:      "2000",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}

	group, err := service.Create(ctx, TradingTPSLCreateRequest{
		UserID:          "user-1",
		Market:          "ETH-USD",
		Size:            "0.5",
		TakeProfitPrice: "2100",
		StopLossPrice:   "1900",
		SlippageBps:     150,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := service.PollOnce(ctx, 100)
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if len(updated) != 2 {
		t.Fatalf("updated entries = %d, want 2", len(updated))
	}
	tp := tpslByKind(t, updated, TradingTPSLKindTakeProfit)
	sl := tpslByKind(t, updated, TradingTPSLKindStopLoss)
	if tp.Status != TradingTPSLStatusTriggered {
		t.Fatalf("take profit status = %q, want %q", tp.Status, TradingTPSLStatusTriggered)
	}
	if sl.Status != TradingTPSLStatusCancelled {
		t.Fatalf("stop loss status = %q, want %q", sl.Status, TradingTPSLStatusCancelled)
	}
	if submitter.calls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitter.calls)
	}
	if submitter.intent.Params["reduceOnly"] != true {
		t.Fatalf("reduceOnly param = %v, want true", submitter.intent.Params["reduceOnly"])
	}
	if submitter.intent.Params["orderType"] != TradingOrderTypeIOC {
		t.Fatalf("orderType param = %v, want %q", submitter.intent.Params["orderType"], TradingOrderTypeIOC)
	}
	if submitter.intent.Params["side"] != "sell" {
		t.Fatalf("side param = %v, want sell", submitter.intent.Params["side"])
	}
	if submitter.intent.Params["size"] != "0.5" {
		t.Fatalf("size param = %v, want 0.5", submitter.intent.Params["size"])
	}
	if group.GroupID == "" || tp.GroupID != group.GroupID || sl.GroupID != group.GroupID {
		t.Fatalf("group linkage = %+v tp=%+v sl=%+v", group, tp, sl)
	}
}

func TestTradingTPSLServiceTriggersStopLossForLongPosition(t *testing.T) {
	ctx := context.Background()
	service, submitter, syncService := newTestTradingTPSLService("1880")
	if _, err := syncService.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:     "pos-1",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "1",
		EntryPrice:  "2000",
		MarkPrice:   "2000",
		MarginAsset: "DEDX",
		Margin:      "2000",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	if _, err := service.Create(ctx, TradingTPSLCreateRequest{
		UserID:          "user-1",
		Market:          "ETH-USD",
		TakeProfitPrice: "2100",
		StopLossPrice:   "1900",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := service.PollOnce(ctx, 100)
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	tp := tpslByKind(t, updated, TradingTPSLKindTakeProfit)
	sl := tpslByKind(t, updated, TradingTPSLKindStopLoss)
	if sl.Status != TradingTPSLStatusTriggered {
		t.Fatalf("stop loss status = %q, want %q", sl.Status, TradingTPSLStatusTriggered)
	}
	if tp.Status != TradingTPSLStatusCancelled {
		t.Fatalf("take profit status = %q, want %q", tp.Status, TradingTPSLStatusCancelled)
	}
	if submitter.intent.Params["side"] != "sell" {
		t.Fatalf("side param = %v, want sell", submitter.intent.Params["side"])
	}
}

func TestTradingTPSLServiceUsesDEDXUSDIsolatedSubaccountPosition(t *testing.T) {
	ctx := context.Background()
	tradingStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{Status: TradingOrderStatusBroadcasted, TxHash: "close-tx-dedx"},
	}
	trading := NewTradingService(tradingStore, ledger, submitter)
	trading.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	trading.SetMarketOrderSlippageBps(100)
	trading.SetDYDXMarkets([]DYDXMarketConfig{{
		Market:           "FDEX-USD",
		ClobPairID:       302,
		QuantumsPerUnit:  "1000000",
		SubticksPerUnit:  "1000000",
		SubticksPerTick:  "1000000",
		StepBaseQuantums: "1000000",
	}})
	marketPrices := staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"FDEX-USD": {Market: "FDEX-USD", Price: "5.5", Source: "test", SnapshotID: "snapshot-dedx"},
		},
	}
	trading.SetMarketPriceSource(marketPrices)
	syncService := NewTradingSyncService(syncStore, tradingStore, ledger)
	trading.SetPositionProvider(syncService)
	service := NewTradingTPSLService(NewMemoryTradingTPSLStore(), trading, marketPrices, syncService)
	service.now = func() time.Time { return time.Unix(1000, 0) }
	if _, err := syncService.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:          "pos-dedx-127",
		UserID:           "user-1",
		Market:           "FDEX-USD",
		SubaccountNumber: 127,
		Size:             "2",
		EntryPrice:       "5",
		MarkPrice:        "5",
		MarginAsset:      "DEDX",
		Margin:           "40",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}

	if _, err := service.Create(ctx, TradingTPSLCreateRequest{
		UserID:          "user-1",
		Market:          "FDEX-USD",
		TakeProfitPrice: "5.4",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := service.PollOnce(ctx, 100)
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("updated entries = %d, want 1", len(updated))
	}
	if submitter.intent.Params["subaccountNumber"] != uint32(127) {
		t.Fatalf("close subaccountNumber = %v, want 127", submitter.intent.Params["subaccountNumber"])
	}
}

func TestTradingTPSLServiceDoesNotTriggerInsideRange(t *testing.T) {
	ctx := context.Background()
	service, submitter, syncService := newTestTradingTPSLService("2000")
	if _, err := syncService.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:     "pos-1",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "1",
		EntryPrice:  "2000",
		MarkPrice:   "2000",
		MarginAsset: "DEDX",
		Margin:      "2000",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	if _, err := service.Create(ctx, TradingTPSLCreateRequest{
		UserID:          "user-1",
		Market:          "ETH-USD",
		TakeProfitPrice: "2100",
		StopLossPrice:   "1900",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := service.PollOnce(ctx, 100)
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if len(updated) != 0 {
		t.Fatalf("updated entries = %+v, want none", updated)
	}
	if submitter.calls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitter.calls)
	}
}

func TestTradingTPSLServiceCancelsGroupWhenPositionSideChanged(t *testing.T) {
	ctx := context.Background()
	service, submitter, syncService := newTestTradingTPSLService("2110")
	if _, err := syncService.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:     "pos-1",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "1",
		EntryPrice:  "2000",
		MarkPrice:   "2000",
		MarginAsset: "DEDX",
		Margin:      "2000",
	}); err != nil {
		t.Fatalf("SyncPosition() error = %v", err)
	}
	if _, err := service.Create(ctx, TradingTPSLCreateRequest{
		UserID:          "user-1",
		Market:          "ETH-USD",
		TakeProfitPrice: "2100",
		StopLossPrice:   "1900",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := syncService.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:     "pos-2",
		UserID:      "user-1",
		Market:      "ETH-USD",
		Size:        "-1",
		EntryPrice:  "2050",
		MarkPrice:   "2110",
		MarginAsset: "DEDX",
		Margin:      "2000",
	}); err != nil {
		t.Fatalf("SyncPosition() replacement error = %v", err)
	}

	updated, err := service.PollOnce(ctx, 100)
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if len(updated) != 2 {
		t.Fatalf("updated entries = %d, want 2 cancelled entries", len(updated))
	}
	for _, entry := range updated {
		if entry.Status != TradingTPSLStatusCancelled || entry.Reason != ErrTradingTPSLPositionChanged.Error() {
			t.Fatalf("entry = %+v, want cancelled by position changed", entry)
		}
	}
	if submitter.calls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitter.calls)
	}
}

func newTestTradingTPSLService(price string) (*TradingTPSLService, *resultTradingSubmitter, *TradingSyncService) {
	tradingStore := NewMemoryTradingStore()
	syncStore := NewMemoryTradingSyncStore()
	ledger := NewLedgerService(NewMemoryLedgerStore())
	submitter := &resultTradingSubmitter{
		result: DYDXTradingResult{Status: TradingOrderStatusBroadcasted, TxHash: "close-tx-1"},
	}
	trading := NewTradingService(tradingStore, ledger, submitter)
	trading.SetDYDXAccountResolver(fixedDYDXAccountResolver{account: "dydx1userwallet"})
	marketPrices := staticTradingMarketPriceSource{
		snapshots: map[string]TradingMarketPriceSnapshot{
			"ETH-USD": {Market: "ETH-USD", Price: price, Source: "test", SnapshotID: "snapshot-1"},
		},
	}
	trading.SetMarketPriceSource(marketPrices)
	trading.SetMarketOrderSlippageBps(100)
	trading.SetDYDXMarkets([]DYDXMarketConfig{{
		Market:           "ETH-USD",
		ClobPairID:       1,
		QuantumsPerUnit:  "1000000000",
		SubticksPerUnit:  "1000000",
		SubticksPerTick:  "100000",
		StepBaseQuantums: "1000000",
	}})
	syncService := NewTradingSyncService(syncStore, tradingStore, ledger)
	trading.SetPositionProvider(syncService)
	tpsl := NewTradingTPSLService(NewMemoryTradingTPSLStore(), trading, marketPrices, syncService)
	tpsl.now = func() time.Time { return time.Unix(1000, 0) }
	return tpsl, submitter, syncService
}

func tpslByKind(t *testing.T, entries []TradingTPSL, kind string) TradingTPSL {
	t.Helper()
	for _, entry := range entries {
		if entry.Kind == kind {
			return entry
		}
	}
	t.Fatalf("missing tpsl kind %q in %+v", kind, entries)
	return TradingTPSL{}
}

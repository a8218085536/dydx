package trading

import (
	"ethgo/tyche"
	"testing"

	"github.com/garyburd/redigo/redis"
)

func requireTradingStore(tyche.TradingStore)         {}
func requireTradingSyncStore(tyche.TradingSyncStore) {}
func requireTradingSyncCursorStore(tyche.TradingSyncCursorStore) {
}
func requireTradingSettlementStore(tyche.TradingSettlementStore) {}

func TestRedisStoreImplementsTradingStores(t *testing.T) {
	requireTradingStore(NewRedisStore())
	requireTradingSyncStore(NewRedisSyncStore())
	requireTradingSyncCursorStore(NewRedisSyncStore())
	requireTradingSettlementStore(NewRedisSettlementStore())
}

func TestTradingOrderScanStructRestoresMarketOrderFields(t *testing.T) {
	values := []interface{}{
		[]byte("id"), []byte("order-1"),
		[]byte("userId"), []byte("user-1"),
		[]byte("subaccountNumber"), []byte("302"),
		[]byte("clientOrderId"), []byte("client-1"),
		[]byte("market"), []byte("ETH-USD"),
		[]byte("side"), []byte("buy"),
		[]byte("orderType"), []byte("market"),
		[]byte("executionOrderType"), []byte("ioc"),
		[]byte("price"), []byte("2525"),
		[]byte("referencePrice"), []byte("2500"),
		[]byte("slippageBps"), []byte("100"),
		[]byte("marketPriceSource"), []byte("dydx-rest"),
		[]byte("marketPriceSnapshotId"), []byte("snapshot-1"),
		[]byte("size"), []byte("1"),
		[]byte("marginAsset"), []byte("DEDX"),
		[]byte("marginAmount"), []byte("2525"),
		[]byte("status"), []byte("funding_required"),
	}
	var order tyche.TradingOrder

	if err := redis.ScanStruct(values, &order); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if order.ClientOrderID != "client-1" || order.OrderType != tyche.TradingOrderTypeMarket || order.ExecutionOrderType != tyche.TradingOrderTypeIOC {
		t.Fatalf("order after scan = %+v", order)
	}
	if order.ReferencePrice != "2500" || order.SlippageBps != 100 || order.MarketPriceSnapshotID != "snapshot-1" {
		t.Fatalf("market fields after scan = %+v", order)
	}
	if order.SubaccountNumber != 302 {
		t.Fatalf("subaccountNumber after scan = %d, want 302", order.SubaccountNumber)
	}
}

func TestShouldListOrderForReconcileIncludesPartialFilledOrders(t *testing.T) {
	order := tyche.TradingOrder{
		ID:              "order-1",
		UserID:          "user-1",
		ClientOrderID:   "client-1",
		DYDXOrderTxHash: "tx-1",
		Status:          tyche.TradingOrderStatusPartialFilled,
	}

	if !shouldListOrderForReconcile(order) {
		t.Fatalf("partial filled order was not queued for reconcile")
	}
}

func TestTradingFillScanStructRestoresTradeDetails(t *testing.T) {
	values := []interface{}{
		[]byte("eventId"), []byte("fill-1"),
		[]byte("tradeHash"), []byte("height-10-tx-1-event-2"),
		[]byte("userId"), []byte("user-1"),
		[]byte("orderId"), []byte("dydx1taker:0:33:1"),
		[]byte("market"), []byte("ETH-USD"),
		[]byte("asset"), []byte("USDT"),
		[]byte("liquidityRole"), []byte("taker"),
		[]byte("makerAddress"), []byte("dydx1maker"),
		[]byte("makerNecoAddress"), []byte("0x00000000000000000000000000000000000000aa"),
		[]byte("takerAddress"), []byte("dydx1taker"),
		[]byte("takerNecoAddress"), []byte("0x00000000000000000000000000000000000000bb"),
		[]byte("makerOrderId"), []byte("dydx1maker:0:22:1"),
		[]byte("takerOrderId"), []byte("dydx1taker:0:33:1"),
		[]byte("clobPairId"), []byte("1"),
		[]byte("fillAmount"), []byte("1000000000"),
		[]byte("makerFee"), []byte("-10"),
		[]byte("takerFee"), []byte("45"),
		[]byte("makerOrderSide"), []byte("sell"),
		[]byte("takerOrderSide"), []byte("buy"),
		[]byte("makerOrderQuantums"), []byte("1000000000"),
		[]byte("makerOrderSubticks"), []byte("1818000000"),
		[]byte("takerOrderQuantums"), []byte("2000000000"),
		[]byte("takerOrderSubticks"), []byte("1819000000"),
		[]byte("createdAt"), []byte("1787898670"),
	}
	var fill tyche.TradingFill

	if err := redis.ScanStruct(values, &fill); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if fill.TradeHash != "height-10-tx-1-event-2" ||
		fill.LiquidityRole != "taker" ||
		fill.MakerAddress != "dydx1maker" ||
		fill.MakerNECOAddress != "0x00000000000000000000000000000000000000aa" ||
		fill.TakerAddress != "dydx1taker" ||
		fill.TakerNECOAddress != "0x00000000000000000000000000000000000000bb" ||
		fill.ClobPairID != 1 ||
		fill.FillAmount != "1000000000" ||
		fill.MakerFee != "-10" ||
		fill.TakerFee != "45" ||
		fill.MakerOrderSubticks != "1818000000" ||
		fill.TakerOrderSubticks != "1819000000" ||
		fill.CreatedAt != 1787898670 {
		t.Fatalf("fill after scan = %+v", fill)
	}
}

func TestTradingPositionScanStructRestoresSubaccountNumber(t *testing.T) {
	values := []interface{}{
		[]byte("eventId"), []byte("position-1"),
		[]byte("userId"), []byte("user-1"),
		[]byte("market"), []byte("FDEX-USD"),
		[]byte("subaccountNumber"), []byte("127"),
		[]byte("size"), []byte("2"),
		[]byte("marginAsset"), []byte("DEDX"),
		[]byte("margin"), []byte("40"),
	}
	var position tyche.TradingPosition

	if err := redis.ScanStruct(values, &position); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if position.SubaccountNumber != 127 || position.Market != "FDEX-USD" || position.Size != "2" {
		t.Fatalf("position after scan = %+v, want subaccount 127 FDEX-USD size 2", position)
	}
}

func TestTradingSettlementScanStructRestoresSettlementFields(t *testing.T) {
	values := []interface{}{
		[]byte("id"), []byte("settlement-1"),
		[]byte("kind"), []byte(tyche.TradingSettlementKindCancelOrder),
		[]byte("userId"), []byte("user-1"),
		[]byte("necoAddress"), []byte("0xabc"),
		[]byte("dydxAccount"), []byte("dydx1abc"),
		[]byte("subaccountNumber"), []byte("0"),
		[]byte("market"), []byte("ETH-USD"),
		[]byte("closeOrderId"), []byte("close-1"),
		[]byte("closeClientOrderId"), []byte("client-close-1"),
		[]byte("settlementUsd"), []byte("20"),
		[]byte("settlementAsset"), []byte("DEDX"),
		[]byte("settlementAmount"), []byte("5000000000000000000"),
		[]byte("assetUsdPrice"), []byte("4"),
		[]byte("priceSnapshotId"), []byte("price-1"),
		[]byte("systemReserved"), []byte("true"),
		[]byte("dydxReservedQuantums"), []byte("20000000"),
		[]byte("ledgerCreditRefId"), []byte("credit-ref"),
		[]byte("ledgerWithdrawalId"), []byte("withdrawal-1"),
		[]byte("necoTxHash"), []byte("0xtx"),
		[]byte("status"), []byte(tyche.TradingSettlementStatusNECOPayoutBroadcasted),
	}
	var settlement tyche.TradingSettlement

	if err := redis.ScanStruct(values, &settlement); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if settlement.ID != "settlement-1" ||
		settlement.Kind != tyche.TradingSettlementKindCancelOrder ||
		settlement.CloseOrderID != "close-1" ||
		settlement.SettlementAmount != "5000000000000000000" ||
		!settlement.SystemReserved ||
		settlement.DYDXReservedQuantums != "20000000" ||
		settlement.Status != tyche.TradingSettlementStatusNECOPayoutBroadcasted {
		t.Fatalf("settlement after scan = %+v", settlement)
	}
}

func TestTradingOrderScanStructRestoresDYDXFundingFields(t *testing.T) {
	values := []interface{}{
		[]byte("id"), []byte("order-1"),
		[]byte("userId"), []byte("user-1"),
		[]byte("clientOrderId"), []byte("client-1"),
		[]byte("dydxFundingTxHash"), []byte("funding-tx-1"),
		[]byte("dydxFundingStatus"), []byte(tyche.TradingOrderStatusBroadcasted),
		[]byte("dydxFundingAssetId"), []byte("0"),
		[]byte("dydxFundingQuantums"), []byte("1892000"),
		[]byte("dydxOrderMarginQuantums"), []byte("1892000"),
		[]byte("dydxRetainedCollateralQuantums"), []byte("1000000"),
		[]byte("dydxOrderTxHash"), []byte("order-tx-1"),
		[]byte("dydxCancelTxHash"), []byte("cancel-tx-1"),
		[]byte("status"), []byte(tyche.TradingOrderStatusBroadcasted),
	}
	var order tyche.TradingOrder

	if err := redis.ScanStruct(values, &order); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}
	if order.DYDXFundingTxHash != "funding-tx-1" ||
		order.DYDXFundingStatus != tyche.TradingOrderStatusBroadcasted ||
		order.DYDXFundingAssetID != 0 ||
		order.DYDXFundingQuantums != "1892000" ||
		order.DYDXOrderMarginQuantums != "1892000" ||
		order.DYDXRetainedCollateralQuantums != "1000000" ||
		order.DYDXOrderTxHash != "order-tx-1" ||
		order.DYDXCancelTxHash != "cancel-tx-1" {
		t.Fatalf("funding fields after scan = %+v", order)
	}
}

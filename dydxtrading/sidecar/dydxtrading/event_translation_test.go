package dydxtrading

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexerEventTranslatorConvertsOrderFillForKnownTaker(t *testing.T) {
	translator := NewIndexerEventTranslator(IndexerEventTranslatorConfig{
		UserResolver: staticUserResolver{"dydx1taker": "user-1"},
		Markets: map[uint32]TradingMarketMapping{
			7: {Market: "ETH-USD", Asset: "USDT"},
		},
	})
	fill := NormalizedOrderFillEvent{
		MakerOrder: NormalizedOrder{OrderID: normalizedOrderID("dydx1maker", 22, 7)},
		TakerOrder: &NormalizedOrder{OrderID: normalizedOrderID("dydx1taker", 33, 7)},
		FillAmount: 55,
		TakerFee:   2,
	}

	events, err := translator.TranslateOrderFill(context.Background(), "height-10-event-2", fill)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "height-10-event-2:taker", events[0].EventID)
	require.Equal(t, "user-1", events[0].UserID)
	require.Equal(t, "dydx1taker:0:33:7", events[0].OrderID)
	require.Equal(t, "ETH-USD", events[0].Market)
	require.Equal(t, "USDT", events[0].Asset)
	require.Equal(t, "0", events[0].ReleasedMargin)
	require.Equal(t, "2", events[0].Fee)
	require.Equal(t, "partial_filled", events[0].OrderStatus)
}

func TestIndexerEventTranslatorPreservesOrderFillAccountingFields(t *testing.T) {
	translator := NewIndexerEventTranslator(IndexerEventTranslatorConfig{
		UserResolver: staticUserResolver{"dydx1taker": "user-1"},
		Markets: map[uint32]TradingMarketMapping{
			7: {Market: "ETH-USD", Asset: "USD"},
		},
	})
	fill := NormalizedOrderFillEvent{
		MakerOrder:          NormalizedOrder{OrderID: normalizedOrderID("dydx1maker", 22, 7)},
		TakerOrder:          &NormalizedOrder{OrderID: normalizedOrderID("dydx1taker", 33, 7)},
		FillAmount:          55,
		TakerReleasedMargin: "12.5",
		TakerFee:            1,
		TakerRealizedPNL:    "3",
	}

	events, err := translator.TranslateOrderFill(context.Background(), "height-10-event-3", fill)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "12.5", events[0].ReleasedMargin)
	require.Equal(t, "1", events[0].Fee)
	require.Equal(t, "3", events[0].RealizedPNL)
}

func TestIndexerEventTranslatorPreservesOrderFillTradeDetails(t *testing.T) {
	translator := NewIndexerEventTranslator(IndexerEventTranslatorConfig{
		UserResolver: staticUserResolver{
			"dydx1maker": "maker-user",
			"dydx1taker": "taker-user",
		},
		Markets: map[uint32]TradingMarketMapping{
			7: {Market: "ETH-USD", Asset: "USD"},
		},
	})
	fill := NormalizedOrderFillEvent{
		MakerOrder: NormalizedOrder{
			OrderID:  normalizedOrderID("dydx1maker", 22, 7),
			Side:     "sell",
			Quantums: "1000000000",
			Subticks: "1818000000",
		},
		TakerOrder: &NormalizedOrder{
			OrderID:  normalizedOrderID("dydx1taker", 33, 7),
			Side:     "buy",
			Quantums: "2000000000",
			Subticks: "1819000000",
		},
		FillAmount: 1000000000,
		MakerFee:   -10,
		TakerFee:   45,
	}

	events, err := translator.TranslateOrderFill(context.Background(), "height-10-tx-1-event-2", fill)

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "height-10-tx-1-event-2", events[0].TradeHash)
	require.Equal(t, "maker", events[0].LiquidityRole)
	require.Equal(t, "dydx1maker", events[0].MakerAddress)
	require.Equal(t, "dydx1taker", events[0].TakerAddress)
	require.Equal(t, "dydx1maker:0:22:7", events[0].MakerOrderID)
	require.Equal(t, "dydx1taker:0:33:7", events[0].TakerOrderID)
	require.Equal(t, uint32(7), events[0].ClobPairID)
	require.Equal(t, "1000000000", events[0].FillAmount)
	require.Equal(t, "-10", events[0].MakerFee)
	require.Equal(t, "45", events[0].TakerFee)
	require.Equal(t, "sell", events[0].MakerOrderSide)
	require.Equal(t, "buy", events[0].TakerOrderSide)
	require.Equal(t, "1000000000", events[0].MakerOrderQuantums)
	require.Equal(t, "1818000000", events[0].MakerOrderSubticks)
	require.Equal(t, "2000000000", events[0].TakerOrderQuantums)
	require.Equal(t, "1819000000", events[0].TakerOrderSubticks)
	require.Equal(t, "height-10-tx-1-event-2", events[1].TradeHash)
	require.Equal(t, "taker", events[1].LiquidityRole)
}

func TestIndexerEventTranslatorConvertsStatefulRemovalAndPositions(t *testing.T) {
	translator := NewIndexerEventTranslator(IndexerEventTranslatorConfig{
		UserResolver: staticUserResolver{"dydx1owner": "user-1"},
		Markets: map[uint32]TradingMarketMapping{
			7: {Market: "ETH-USD", Asset: "USDT"},
		},
		Perpetuals: map[uint32]TradingMarketMapping{
			2: {Market: "ETH-USD", Asset: "USDT"},
		},
	})
	removal := NormalizedStatefulOrderRemovalEvent{
		OrderID: normalizedOrderID("dydx1owner", 44, 7),
		Reason:  NormalizedOrderRemovalReasonUserCanceled,
	}
	status, err := translator.TranslateStatefulOrderRemoval(context.Background(), "height-11-event-1", removal)

	require.NoError(t, err)
	require.Equal(t, "height-11-event-1", status.EventID)
	require.Equal(t, "user-1", status.UserID)
	require.Equal(t, "dydx1owner:0:44:7", status.OrderID)
	require.Equal(t, "cancelled", status.Status)

	positionEvents, err := translator.TranslateSubaccountUpdate(context.Background(), "height-12-event-1", NormalizedSubaccountUpdateEvent{
		SubaccountID: NormalizedSubaccountID{Owner: "dydx1owner", Number: 127},
		UpdatedPerpetualPositions: []NormalizedPerpetualPosition{
			{
				PerpetualId: 2,
				Quantums:    "123",
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, positionEvents, 1)
	require.Equal(t, "height-12-event-1:2", positionEvents[0].EventID)
	require.Equal(t, "user-1", positionEvents[0].UserID)
	require.Equal(t, uint32(127), positionEvents[0].SubaccountNumber)
	require.Equal(t, "ETH-USD", positionEvents[0].Market)
	require.Equal(t, "123", positionEvents[0].Size)
	require.Equal(t, "USDT", positionEvents[0].MarginAsset)
}

type staticUserResolver map[string]string

func (r staticUserResolver) UserIDByDYDXAddress(_ context.Context, address string) (string, bool, error) {
	userID, ok := r[address]
	return userID, ok, nil
}

func normalizedOrderID(owner string, clientID uint32, clobPairID uint32) NormalizedOrderID {
	return NormalizedOrderID{
		SubaccountID: NormalizedSubaccountID{Owner: owner, Number: 0},
		ClientID:     clientID,
		ClobPairID:   clobPairID,
	}
}

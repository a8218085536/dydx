package dydxtrading

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRawIndexerEventAdapterConvertsOrderFill(t *testing.T) {
	raw := []byte(`{
		"eventId": "height-10-event-1",
		"subtype": "order_fill",
		"data": {
			"makerOrder": {
				"orderId": {
					"subaccountId": {"owner": "dydx1maker", "number": 0},
					"clientId": 22,
					"orderFlags": 0,
					"clobPairId": 7
				},
				"side": "SIDE_SELL",
				"quantums": "1000000000",
				"subticks": "1818000000"
			},
			"order": {
				"orderId": {
					"subaccountId": {"owner": "dydx1taker", "number": 0},
					"clientId": 33,
					"orderFlags": 0,
					"clobPairId": 7
				},
				"side": "SIDE_BUY",
				"quantums": "2000000000",
				"subticks": "1819000000"
			},
			"fillAmount": 55,
			"makerReleasedMargin": "10",
			"makerFee": 1,
			"makerRealizedPnl": "2",
			"takerReleasedMargin": "12",
			"takerFee": 2,
			"takerRealizedPnl": "-1"
		}
	}`)

	event, err := NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.NoError(t, err)
	require.Equal(t, "height-10-event-1", event.EventID)
	require.Equal(t, NormalizedIndexerEventTypeOrderFill, event.Type)
	require.Equal(t, uint64(55), event.OrderFill.FillAmount)
	require.Equal(t, "10", event.OrderFill.MakerReleasedMargin)
	require.Equal(t, int64(1), event.OrderFill.MakerFee)
	require.Equal(t, "2", event.OrderFill.MakerRealizedPNL)
	require.Equal(t, "12", event.OrderFill.TakerReleasedMargin)
	require.Equal(t, int64(2), event.OrderFill.TakerFee)
	require.Equal(t, "-1", event.OrderFill.TakerRealizedPNL)
	require.Equal(t, "dydx1maker", event.OrderFill.MakerOrder.OrderID.SubaccountID.Owner)
	require.Equal(t, "sell", event.OrderFill.MakerOrder.Side)
	require.Equal(t, "1000000000", event.OrderFill.MakerOrder.Quantums)
	require.Equal(t, "1818000000", event.OrderFill.MakerOrder.Subticks)
	require.Equal(t, uint32(33), event.OrderFill.TakerOrder.OrderID.ClientID)
	require.Equal(t, "buy", event.OrderFill.TakerOrder.Side)
	require.Equal(t, "2000000000", event.OrderFill.TakerOrder.Quantums)
	require.Equal(t, "1819000000", event.OrderFill.TakerOrder.Subticks)
}

func TestRawIndexerEventAdapterConvertsProtocolSnakeCaseOrderFill(t *testing.T) {
	raw := []byte(`{
		"eventId": "height-10-event-2",
		"subtype": "order_fill",
		"data": {
			"maker_order": {
				"order_id": {
					"subaccount_id": {"owner": "dydx1maker", "number": 1},
					"client_id": 22,
					"order_flags": 64,
					"clob_pair_id": 7
				},
				"side": 2,
				"quantums": 3000000000,
				"subticks": 1820000000
			},
			"order": {
				"order_id": {
					"subaccount_id": {"owner": "dydx1taker", "number": 2},
					"client_id": 33,
					"order_flags": 0,
					"clob_pair_id": 8
				},
				"side": 1,
				"quantums": 4000000000,
				"subticks": 1821000000
			},
			"fill_amount": 55,
			"maker_released_margin": "11",
			"maker_fee": -1,
			"maker_realized_pnl": "3",
			"taker_released_margin": "13",
			"taker_fee": 2,
			"taker_realized_pnl": "-2"
		}
	}`)

	event, err := NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.NoError(t, err)
	require.Equal(t, NormalizedIndexerEventTypeOrderFill, event.Type)
	require.Equal(t, uint32(1), event.OrderFill.MakerOrder.OrderID.SubaccountID.Number)
	require.Equal(t, uint32(64), event.OrderFill.MakerOrder.OrderID.OrderFlags)
	require.Equal(t, uint32(8), event.OrderFill.TakerOrder.OrderID.ClobPairID)
	require.Equal(t, uint64(55), event.OrderFill.FillAmount)
	require.Equal(t, "11", event.OrderFill.MakerReleasedMargin)
	require.Equal(t, int64(-1), event.OrderFill.MakerFee)
	require.Equal(t, "3", event.OrderFill.MakerRealizedPNL)
	require.Equal(t, "sell", event.OrderFill.MakerOrder.Side)
	require.Equal(t, "3000000000", event.OrderFill.MakerOrder.Quantums)
	require.Equal(t, "1820000000", event.OrderFill.MakerOrder.Subticks)
	require.Equal(t, "13", event.OrderFill.TakerReleasedMargin)
	require.Equal(t, "-2", event.OrderFill.TakerRealizedPNL)
	require.Equal(t, "buy", event.OrderFill.TakerOrder.Side)
	require.Equal(t, "4000000000", event.OrderFill.TakerOrder.Quantums)
	require.Equal(t, "1821000000", event.OrderFill.TakerOrder.Subticks)
}

func TestRawIndexerEventAdapterConvertsStatefulOrderRemoval(t *testing.T) {
	raw := []byte(`{
		"eventId": "height-11-event-1",
		"subtype": "stateful_order",
		"data": {
			"orderRemoval": {
				"removedOrderId": {
					"subaccountId": {"owner": "dydx1owner", "number": 0},
					"clientId": 44,
					"orderFlags": 64,
					"clobPairId": 7
				},
				"reason": "ORDER_REMOVAL_REASON_USER_CANCELED"
			}
		}
	}`)

	event, err := NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.NoError(t, err)
	require.Equal(t, NormalizedIndexerEventTypeStatefulOrderRemoval, event.Type)
	require.Equal(t, "user_canceled", event.StatefulOrderRemoval.Reason)
	require.Equal(t, uint32(44), event.StatefulOrderRemoval.OrderID.ClientID)
	require.Equal(t, uint32(64), event.StatefulOrderRemoval.OrderID.OrderFlags)
}

func TestRawIndexerEventAdapterConvertsProtocolSnakeCaseStatefulOrderRemoval(t *testing.T) {
	raw := []byte(`{
		"eventId": "height-11-event-2",
		"subtype": "stateful_order",
		"data": {
			"order_removal": {
				"removed_order_id": {
					"subaccount_id": {"owner": "dydx1owner", "number": 0},
					"client_id": 44,
					"order_flags": 64,
					"clob_pair_id": 7
				},
				"reason": "ORDER_REMOVAL_REASON_FULLY_FILLED"
			}
		}
	}`)

	event, err := NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.NoError(t, err)
	require.Equal(t, NormalizedIndexerEventTypeStatefulOrderRemoval, event.Type)
	require.Equal(t, "fully_filled", event.StatefulOrderRemoval.Reason)
	require.Equal(t, uint32(44), event.StatefulOrderRemoval.OrderID.ClientID)
}

func TestRawIndexerEventAdapterConvertsSubaccountUpdate(t *testing.T) {
	raw := []byte(`{
		"eventId": "height-12-event-1",
		"subtype": "subaccount_update",
		"data": {
			"subaccountId": {"owner": "dydx1owner", "number": 0},
			"updatedPerpetualPositions": [
				{"perpetualId": 2, "quantums": "123"}
			]
		}
	}`)

	event, err := NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.NoError(t, err)
	require.Equal(t, NormalizedIndexerEventTypeSubaccountUpdate, event.Type)
	require.Equal(t, "dydx1owner", event.SubaccountUpdate.SubaccountID.Owner)
	require.Len(t, event.SubaccountUpdate.UpdatedPerpetualPositions, 1)
	require.Equal(t, "123", event.SubaccountUpdate.UpdatedPerpetualPositions[0].Quantums)
}

func TestRawIndexerEventAdapterConvertsProtocolSnakeCaseSubaccountUpdate(t *testing.T) {
	raw := []byte(`{
		"eventId": "height-12-event-2",
		"subtype": "subaccount_update",
		"data": {
			"subaccount_id": {"owner": "dydx1owner", "number": 0},
			"updated_perpetual_positions": [
				{"perpetual_id": 2, "quantums": "123"}
			]
		}
	}`)

	event, err := NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.NoError(t, err)
	require.Equal(t, NormalizedIndexerEventTypeSubaccountUpdate, event.Type)
	require.Equal(t, "dydx1owner", event.SubaccountUpdate.SubaccountID.Owner)
	require.Len(t, event.SubaccountUpdate.UpdatedPerpetualPositions, 1)
	require.Equal(t, uint32(2), event.SubaccountUpdate.UpdatedPerpetualPositions[0].PerpetualId)
	require.Equal(t, "123", event.SubaccountUpdate.UpdatedPerpetualPositions[0].Quantums)
}

func TestRawIndexerEventAdapterRejectsUnsupportedSubtype(t *testing.T) {
	raw, err := json.Marshal(RawIndexerEvent{EventID: "height-1-event-1", Subtype: "market"})
	require.NoError(t, err)

	_, err = NewRawIndexerEventAdapter().ConvertJSON(raw)

	require.ErrorContains(t, err, "unsupported indexer event subtype")
}

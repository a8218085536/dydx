package dydxtrading

import "context"

type TradingEventPage[T any] struct {
	Data       []T
	NextCursor string
}

type TradingEventProvider interface {
	Fills(context.Context, string) (TradingEventPage[TradingFillEvent], error)
	OrderStatuses(context.Context, string) (TradingEventPage[TradingOrderStatusEvent], error)
	Positions(context.Context, string) (TradingEventPage[TradingPositionEvent], error)
}

type TradingFillEvent struct {
	EventID            string `json:"eventId"`
	TradeHash          string `json:"tradeHash,omitempty"`
	UserID             string `json:"userId"`
	OrderID            string `json:"orderId"`
	Market             string `json:"market"`
	Asset              string `json:"asset"`
	ReleasedMargin     string `json:"releasedMargin"`
	Fee                string `json:"fee"`
	RealizedPNL        string `json:"realizedPnl"`
	OrderStatus        string `json:"orderStatus"`
	LiquidityRole      string `json:"liquidityRole,omitempty"`
	MakerAddress       string `json:"makerAddress,omitempty"`
	TakerAddress       string `json:"takerAddress,omitempty"`
	MakerOrderID       string `json:"makerOrderId,omitempty"`
	TakerOrderID       string `json:"takerOrderId,omitempty"`
	ClobPairID         uint32 `json:"clobPairId,omitempty"`
	FillAmount         string `json:"fillAmount,omitempty"`
	MakerFee           string `json:"makerFee,omitempty"`
	TakerFee           string `json:"takerFee,omitempty"`
	MakerOrderSide     string `json:"makerOrderSide,omitempty"`
	TakerOrderSide     string `json:"takerOrderSide,omitempty"`
	MakerOrderQuantums string `json:"makerOrderQuantums,omitempty"`
	MakerOrderSubticks string `json:"makerOrderSubticks,omitempty"`
	TakerOrderQuantums string `json:"takerOrderQuantums,omitempty"`
	TakerOrderSubticks string `json:"takerOrderSubticks,omitempty"`
}

type TradingOrderStatusEvent struct {
	EventID      string `json:"eventId"`
	UserID       string `json:"userId"`
	OrderID      string `json:"orderId"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	RefundAsset  string `json:"refundAsset"`
	RefundAmount string `json:"refundAmount"`
}

type TradingPositionEvent struct {
	EventID          string `json:"eventId"`
	UserID           string `json:"userId"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
	Size             string `json:"size"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnrealizedPNL    string `json:"unrealizedPnl"`
	MarginAsset      string `json:"marginAsset"`
	Margin           string `json:"margin"`
}

type EmptyTradingEventProvider struct{}

func (EmptyTradingEventProvider) Fills(context.Context, string) (TradingEventPage[TradingFillEvent], error) {
	return TradingEventPage[TradingFillEvent]{Data: []TradingFillEvent{}}, nil
}

func (EmptyTradingEventProvider) OrderStatuses(context.Context, string) (TradingEventPage[TradingOrderStatusEvent], error) {
	return TradingEventPage[TradingOrderStatusEvent]{Data: []TradingOrderStatusEvent{}}, nil
}

func (EmptyTradingEventProvider) Positions(context.Context, string) (TradingEventPage[TradingPositionEvent], error) {
	return TradingEventPage[TradingPositionEvent]{Data: []TradingPositionEvent{}}, nil
}

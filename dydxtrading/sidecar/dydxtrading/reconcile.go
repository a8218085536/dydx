package dydxtrading

import "context"

type OrderReconcileRequest struct {
	TxHash           string `json:"txHash"`
	UserID           string `json:"userId"`
	OrderID          string `json:"orderId"`
	DYDXAccount      string `json:"dydxAccount"`
	ClientOrderID    string `json:"clientOrderId"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber"`
	ClobPairID       uint32 `json:"clobPairId"`
	OrderFlags       uint32 `json:"orderFlags"`
}

type OrderReconcileResult struct {
	Tx            TxReconcileSnapshot            `json:"tx"`
	StatefulOrder StatefulOrderReconcileSnapshot `json:"statefulOrder"`
	Subaccount    SubaccountReconcileSnapshot    `json:"subaccount"`
}

type TxReconcileSnapshot struct {
	Found  bool   `json:"found"`
	TxHash string `json:"txHash"`
	Height string `json:"height"`
	Code   uint32 `json:"code"`
	RawLog string `json:"rawLog"`
}

type StatefulOrderReconcileSnapshot struct {
	Checked    bool   `json:"checked"`
	Found      bool   `json:"found"`
	FillAmount string `json:"fillAmount"`
}

type SubaccountReconcileSnapshot struct {
	Found              bool                                 `json:"found"`
	AssetPositions     []AssetPositionReconcileSnapshot     `json:"assetPositions"`
	PerpetualPositions []PerpetualPositionReconcileSnapshot `json:"perpetualPositions"`
}

type AssetPositionReconcileSnapshot struct {
	AssetID  uint32 `json:"assetId"`
	Quantums string `json:"quantums"`
}

type PerpetualPositionReconcileSnapshot struct {
	PerpetualID  uint32 `json:"perpetualId"`
	Quantums     string `json:"quantums"`
	FundingIndex string `json:"fundingIndex,omitempty"`
	QuoteBalance string `json:"quoteBalance,omitempty"`
}

type OrderReconcileProvider interface {
	ReconcileOrder(context.Context, OrderReconcileRequest) (OrderReconcileResult, error)
}

type EmptyOrderReconcileProvider struct{}

func (EmptyOrderReconcileProvider) ReconcileOrder(context.Context, OrderReconcileRequest) (OrderReconcileResult, error) {
	return OrderReconcileResult{}, nil
}

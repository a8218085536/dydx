package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	TradingCloseCallbackStatusSent   = "sent"
	TradingCloseCallbackStatusFailed = "failed"
)

var ErrTradingCloseCallbackFillMissing = errors.New("trading close callback fill is missing")

type TradingCloseCallbackPayload struct {
	TradeHash          string     `json:"trade_hash"`
	Symbol             string     `json:"symbol"`
	MarketType         string     `json:"market_type"`
	TradedAt           *time.Time `json:"traded_at,omitempty"`
	MakerAddress       string     `json:"maker_address"`
	MakerNECOAddress   string     `json:"maker_neco_address"`
	TakerAddress       string     `json:"taker_address"`
	TakerNECOAddress   string     `json:"taker_neco_address"`
	Price              string     `json:"price"`
	Quantity           string     `json:"quantity"`
	Turnover           string     `json:"turnover"`
	MakerFeeRate       string     `json:"maker_fee_rate"`
	TakerFeeRate       string     `json:"taker_fee_rate"`
	MakerFee           string     `json:"maker_fee"`
	TakerFee           string     `json:"taker_fee"`
	NetFee             string     `json:"net_fee"`
	UserID             string     `json:"user_id"`
	NECOAddress        string     `json:"neco_address"`
	DYDXAccount        string     `json:"dydx_account"`
	SubaccountNumber   uint32     `json:"subaccount_number"`
	CloseOrderID       string     `json:"close_order_id"`
	CloseClientOrderID string     `json:"close_client_order_id"`
	SettlementID       string     `json:"settlement_id"`
	SettlementUSD      string     `json:"settlement_usd"`
	SettlementAsset    string     `json:"settlement_asset"`
	SettlementAmount   string     `json:"settlement_amount"`
	NECOTxHash         string     `json:"neco_tx_hash,omitempty"`
	FillEventID        string     `json:"fill_event_id,omitempty"`
	LiquidityRole      string     `json:"liquidity_role,omitempty"`
}

type TradingCloseCallbackSender interface {
	SendTradeClose(context.Context, TradingCloseCallbackPayload) error
}

type HTTPTradingCloseCallbackSender struct {
	url    string
	client *http.Client
}

func NewHTTPTradingCloseCallbackSender(url string) *HTTPTradingCloseCallbackSender {
	return &HTTPTradingCloseCallbackSender{
		url: strings.TrimSpace(url),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *HTTPTradingCloseCallbackSender) SendTradeClose(ctx context.Context, payload TradingCloseCallbackPayload) error {
	if s == nil || strings.TrimSpace(s.url) == "" {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return errors.New("trading close callback returned non-2xx status")
	}
	return nil
}

func NewTradingCloseCallbackPayload(settlement TradingSettlement, order TradingOrder, fill TradingFill, markets map[string]DYDXMarketConfig) TradingCloseCallbackPayload {
	market := firstNonEmptyTradingString(fill.Market, order.Market, settlement.Market)
	profile := TradingFeeProfileForMarket(market)
	price, quantity, turnover := tradingFillPriceQuantityTurnover(fill, markets, market)
	return TradingCloseCallbackPayload{
		TradeHash:          fill.TradeHash,
		Symbol:             market,
		MarketType:         profile.MarketType,
		TradedAt:           tradingFillTradedAt(fill),
		MakerAddress:       fill.MakerAddress,
		MakerNECOAddress:   fill.MakerNECOAddress,
		TakerAddress:       fill.TakerAddress,
		TakerNECOAddress:   fill.TakerNECOAddress,
		Price:              price,
		Quantity:           quantity,
		Turnover:           turnover,
		MakerFeeRate:       profile.MakerFeeRate,
		TakerFeeRate:       profile.TakerFeeRate,
		MakerFee:           defaultDecimalString(fill.MakerFee),
		TakerFee:           defaultDecimalString(fill.TakerFee),
		NetFee:             tradingNetFeeString(fill),
		UserID:             settlement.UserID,
		NECOAddress:        settlement.NECOAddress,
		DYDXAccount:        settlement.DYDXAccount,
		SubaccountNumber:   settlement.SubaccountNumber,
		CloseOrderID:       settlement.CloseOrderID,
		CloseClientOrderID: settlement.CloseClientOrderID,
		SettlementID:       settlement.ID,
		SettlementUSD:      settlement.SettlementUSD,
		SettlementAsset:    settlement.SettlementAsset,
		SettlementAmount:   settlement.SettlementAmount,
		NECOTxHash:         settlement.NECOTxHash,
		FillEventID:        fill.EventID,
		LiquidityRole:      fill.LiquidityRole,
	}
}

func tradingFillTradedAt(fill TradingFill) *time.Time {
	if fill.CreatedAt <= 0 {
		return nil
	}
	tradedAt := time.Unix(fill.CreatedAt, 0).UTC()
	return &tradedAt
}

func tradingFillPriceQuantityTurnover(fill TradingFill, markets map[string]DYDXMarketConfig, fallbackMarket string) (string, string, string) {
	marketName := firstNonEmptyTradingString(fill.Market, fallbackMarket, marketNameForClobPair(markets, fill.ClobPairID))
	market, ok := markets[normalizeDYDXMarket(marketName)]
	if !ok {
		return "", "", ""
	}
	quantity, ok := scaledIntegerStringToRat(fill.FillAmount, market.QuantumsPerUnit)
	if !ok {
		return "", "", ""
	}
	priceSubticks := firstNonEmptyTradingString(fill.MakerOrderSubticks, fill.TakerOrderSubticks)
	price, ok := scaledIntegerStringToRat(priceSubticks, market.SubticksPerUnit)
	if !ok {
		return "", ratToDecimalString(quantity, 18), ""
	}
	turnover := new(big.Rat).Mul(quantity, price)
	return ratToDecimalString(price, 18), ratToDecimalString(quantity, 18), ratToDecimalString(turnover, 18)
}

func tradingNetFeeString(fill TradingFill) string {
	makerFee, makerOK := parseDecimalRat(fill.MakerFee)
	takerFee, takerOK := parseDecimalRat(fill.TakerFee)
	if makerOK || takerOK {
		total := big.NewRat(0, 1)
		if makerOK {
			total.Add(total, makerFee)
		}
		if takerOK {
			total.Add(total, takerFee)
		}
		return ratToDecimalString(total, 18)
	}
	return defaultDecimalString(fill.Fee)
}

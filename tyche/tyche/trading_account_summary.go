package tyche

import (
	"context"
	"math/big"
	"sort"
	"strings"
)

type TradingAccountSummaryRequest struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress,omitempty"`
}

type TradingAccountSummary struct {
	UserID     string                          `json:"userId"`
	Stats      TradingAccountStats             `json:"stats"`
	Positions  []TradingAccountPositionSummary `json:"positions"`
	OpenOrders []TradingAccountOpenOrder       `json:"openOrders"`
}

type TradingAccountStats struct {
	CumulativeTurnoverUSD  string `json:"cumulativeTurnoverUsd"`
	RealizedPNLUSD         string `json:"realizedPnlUsd"`
	UnrealizedPNLUSD       string `json:"unrealizedPnlUsd"`
	TradeCount             int    `json:"tradeCount"`
	WinRate                string `json:"winRate"`
	MaxDrawdownUSD         string `json:"maxDrawdownUsd"`
	CurrentMarketType      string `json:"currentMarketType"`
	MakerFeeRate           string `json:"makerFeeRate"`
	TakerFeeRate           string `json:"takerFeeRate"`
	CumulativeFeeRebateUSD string `json:"cumulativeFeeRebateUsd"`
	CumulativeMakerFeeUSD  string `json:"cumulativeMakerFeeUsd"`
	CumulativeTakerFeeUSD  string `json:"cumulativeTakerFeeUsd"`
	NetFeeUSD              string `json:"netFeeUsd"`
}

type TradingAccountPositionSummary struct {
	EventID          string `json:"eventId,omitempty"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber"`
	Size             string `json:"size"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnrealizedPNLUSD string `json:"unrealizedPnlUsd"`
	MarginAsset      string `json:"marginAsset"`
	Margin           string `json:"margin"`
	MarginUSD        string `json:"marginUsd"`
}

type TradingAccountOpenOrder struct {
	ID               string `json:"id"`
	ClientOrderID    string `json:"clientOrderId"`
	Market           string `json:"market"`
	SubaccountNumber uint32 `json:"subaccountNumber"`
	Side             string `json:"side"`
	OrderType        string `json:"orderType"`
	Price            string `json:"price"`
	Size             string `json:"size"`
	MarginAsset      string `json:"marginAsset"`
	MarginAmount     string `json:"marginAmount"`
	MarginUSD        string `json:"marginUsd"`
	Status           string `json:"status"`
	OpenedAt         int64  `json:"openedAt"`
}

type TradingAccountOrderLister interface {
	ListOrders(context.Context, string) ([]TradingOrder, error)
}

type TradingAccountSummaryService struct {
	orders TradingAccountOrderLister
	sync   interface {
		TradingFillLookup
		TradingPositionLookup
	}
	markets map[string]DYDXMarketConfig
}

func NewTradingAccountSummaryService(orders TradingAccountOrderLister, syncSource interface {
	TradingFillLookup
	TradingPositionLookup
}, markets []DYDXMarketConfig) *TradingAccountSummaryService {
	service := &TradingAccountSummaryService{
		orders:  orders,
		sync:    syncSource,
		markets: map[string]DYDXMarketConfig{},
	}
	for _, market := range markets {
		service.markets[normalizeDYDXMarket(market.Market)] = market
	}
	return service
}

func (s *TradingAccountSummaryService) Summary(ctx context.Context, request TradingAccountSummaryRequest) (TradingAccountSummary, error) {
	summary := TradingAccountSummary{UserID: request.UserID}
	orders, err := s.listOrders(ctx, request.UserID)
	if err != nil {
		return TradingAccountSummary{}, err
	}
	positions, err := s.listPositions(ctx, request.UserID)
	if err != nil {
		return TradingAccountSummary{}, err
	}
	fills, err := s.listFills(ctx, request.UserID, orders)
	if err != nil {
		return TradingAccountSummary{}, err
	}
	summary.Positions = s.positionSummaries(positions)
	summary.OpenOrders = s.openOrderSummaries(orders)
	summary.Stats = s.stats(orders, positions, fills)
	return summary, nil
}

func (s *TradingAccountSummaryService) listOrders(ctx context.Context, userID string) ([]TradingOrder, error) {
	if s == nil || s.orders == nil || strings.TrimSpace(userID) == "" {
		return []TradingOrder{}, nil
	}
	return s.orders.ListOrders(ctx, userID)
}

func (s *TradingAccountSummaryService) listPositions(ctx context.Context, userID string) ([]TradingPosition, error) {
	if s == nil || s.sync == nil || strings.TrimSpace(userID) == "" {
		return []TradingPosition{}, nil
	}
	return s.sync.ListPositionsByUser(ctx, userID)
}

func (s *TradingAccountSummaryService) listFills(ctx context.Context, userID string, orders []TradingOrder) ([]TradingFill, error) {
	if s == nil || s.sync == nil || strings.TrimSpace(userID) == "" {
		return []TradingFill{}, nil
	}
	fills := make([]TradingFill, 0)
	seen := map[string]bool{}
	for _, order := range orders {
		candidates := []string{order.ID}
		if dydxOrderID := s.dydxFillOrderID(order); dydxOrderID != "" && dydxOrderID != order.ID {
			candidates = append(candidates, dydxOrderID)
		}
		for _, orderID := range candidates {
			if strings.TrimSpace(orderID) == "" {
				continue
			}
			list, err := s.sync.ListFillsByOrder(ctx, userID, orderID)
			if err != nil {
				return nil, err
			}
			for _, fill := range list {
				key := fill.EventID
				if key == "" {
					key = fill.TradeHash + ":" + fill.OrderID
				}
				if key != "" && seen[key] {
					continue
				}
				if key != "" {
					seen[key] = true
				}
				fills = append(fills, fill)
			}
		}
	}
	sort.Slice(fills, func(i, j int) bool {
		return fills[i].EventID < fills[j].EventID
	})
	return fills, nil
}

func (s *TradingAccountSummaryService) dydxFillOrderID(order TradingOrder) string {
	market, ok := s.markets[normalizeDYDXMarket(order.Market)]
	if !ok || strings.TrimSpace(order.DYDXAccount) == "" || strings.TrimSpace(order.ClientOrderID) == "" {
		return ""
	}
	subaccount := explicitOrBuiltInDYDXSubaccountNumber(order.SubaccountNumber, order.Market, market.ClobPairID, order.SubaccountNumber)
	return order.DYDXAccount + ":" + uint32String(subaccount) + ":" + order.ClientOrderID + ":" + uint32String(market.ClobPairID)
}

func (s *TradingAccountSummaryService) positionSummaries(positions []TradingPosition) []TradingAccountPositionSummary {
	out := make([]TradingAccountPositionSummary, 0, len(positions))
	for _, position := range positions {
		if isZeroDecimalString(position.Size) {
			continue
		}
		out = append(out, TradingAccountPositionSummary{
			EventID:          position.EventID,
			Market:           position.Market,
			SubaccountNumber: position.SubaccountNumber,
			Size:             position.Size,
			EntryPrice:       position.EntryPrice,
			MarkPrice:        position.MarkPrice,
			UnrealizedPNLUSD: defaultDecimalString(position.UnrealizedPNL),
			MarginAsset:      position.MarginAsset,
			Margin:           position.Margin,
			MarginUSD:        defaultDecimalString(position.Margin),
		})
	}
	return out
}

func (s *TradingAccountSummaryService) openOrderSummaries(orders []TradingOrder) []TradingAccountOpenOrder {
	out := make([]TradingAccountOpenOrder, 0)
	for _, order := range orders {
		if !isTradingRestingOpenOrder(order) {
			continue
		}
		out = append(out, TradingAccountOpenOrder{
			ID:               order.ID,
			ClientOrderID:    order.ClientOrderID,
			Market:           order.Market,
			SubaccountNumber: order.SubaccountNumber,
			Side:             order.Side,
			OrderType:        order.OrderType,
			Price:            order.Price,
			Size:             order.Size,
			MarginAsset:      order.MarginAsset,
			MarginAmount:     order.MarginAmount,
			MarginUSD:        order.MarginUSD,
			Status:           order.Status,
			OpenedAt:         firstNonZeroInt64(order.CreatedAt, order.UpdatedAt),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OpenedAt < out[j].OpenedAt
	})
	return out
}

func isTradingRestingOpenOrder(order TradingOrder) bool {
	if order.ClosePosition || !isTradingOpenOrderStatus(order.Status) {
		return false
	}
	return !isTradingShortTermOrder(order)
}

func isTradingShortTermOrder(order TradingOrder) bool {
	return strings.EqualFold(order.ExecutionOrderType, TradingOrderTypeIOC) ||
		strings.EqualFold(order.OrderType, TradingOrderTypeIOC) ||
		strings.EqualFold(order.OrderType, TradingOrderTypeMarket)
}

func (s *TradingAccountSummaryService) stats(orders []TradingOrder, positions []TradingPosition, fills []TradingFill) TradingAccountStats {
	turnover := big.NewRat(0, 1)
	realizedPNL := big.NewRat(0, 1)
	unrealizedPNL := big.NewRat(0, 1)
	makerFees := big.NewRat(0, 1)
	takerFees := big.NewRat(0, 1)
	feeRebate := big.NewRat(0, 1)
	netFee := big.NewRat(0, 1)
	cumulativePNL := big.NewRat(0, 1)
	peakPNL := big.NewRat(0, 1)
	maxDrawdown := big.NewRat(0, 1)
	wins := 0
	pnlCount := 0

	for _, position := range positions {
		addDecimalInto(unrealizedPNL, position.UnrealizedPNL)
	}
	for _, fill := range fills {
		if value, ok := s.fillTurnoverUSD(fill); ok {
			turnover.Add(turnover, value)
		}
		if pnl, ok := parseDecimalRat(fill.RealizedPNL); ok {
			realizedPNL.Add(realizedPNL, pnl)
			cumulativePNL.Add(cumulativePNL, pnl)
			if pnl.Sign() > 0 {
				wins++
			}
			pnlCount++
			if cumulativePNL.Cmp(peakPNL) > 0 {
				peakPNL.Set(cumulativePNL)
			}
			drawdown := new(big.Rat).Sub(peakPNL, cumulativePNL)
			if drawdown.Cmp(maxDrawdown) > 0 {
				maxDrawdown.Set(drawdown)
			}
		}
		makerFee, makerOK := parseDecimalRat(fill.MakerFee)
		takerFee, takerOK := parseDecimalRat(fill.TakerFee)
		if makerOK {
			makerFees.Add(makerFees, makerFee)
			netFee.Add(netFee, makerFee)
			if makerFee.Sign() < 0 {
				feeRebate.Add(feeRebate, new(big.Rat).Neg(makerFee))
			}
		}
		if takerOK {
			takerFees.Add(takerFees, takerFee)
			netFee.Add(netFee, takerFee)
		}
		if !makerOK && !takerOK {
			addDecimalInto(netFee, fill.Fee)
		}
	}

	profile := TradingFeeProfileForMarket(accountSummaryReferenceMarket(orders, positions, fills))
	return TradingAccountStats{
		CumulativeTurnoverUSD:  ratToDecimalString(turnover, 18),
		RealizedPNLUSD:         ratToDecimalString(realizedPNL, 18),
		UnrealizedPNLUSD:       ratToDecimalString(unrealizedPNL, 18),
		TradeCount:             len(fills),
		WinRate:                winRateString(wins, pnlCount),
		MaxDrawdownUSD:         ratToDecimalString(maxDrawdown, 18),
		CurrentMarketType:      profile.MarketType,
		MakerFeeRate:           profile.MakerFeeRate,
		TakerFeeRate:           profile.TakerFeeRate,
		CumulativeFeeRebateUSD: ratToDecimalString(feeRebate, 18),
		CumulativeMakerFeeUSD:  ratToDecimalString(makerFees, 18),
		CumulativeTakerFeeUSD:  ratToDecimalString(takerFees, 18),
		NetFeeUSD:              ratToDecimalString(netFee, 18),
	}
}

func (s *TradingAccountSummaryService) fillTurnoverUSD(fill TradingFill) (*big.Rat, bool) {
	marketName := firstNonEmptyTradingString(fill.Market, marketNameForClobPair(s.markets, fill.ClobPairID))
	market, ok := s.markets[normalizeDYDXMarket(marketName)]
	if !ok {
		return nil, false
	}
	quantity, ok := scaledIntegerStringToRat(fill.FillAmount, market.QuantumsPerUnit)
	if !ok {
		return nil, false
	}
	priceSubticks := firstNonEmptyTradingString(fill.MakerOrderSubticks, fill.TakerOrderSubticks)
	price, ok := scaledIntegerStringToRat(priceSubticks, market.SubticksPerUnit)
	if !ok {
		return nil, false
	}
	return new(big.Rat).Mul(quantity, price), true
}

func accountSummaryReferenceMarket(orders []TradingOrder, positions []TradingPosition, fills []TradingFill) string {
	for _, order := range orders {
		if TradingFeeProfileForMarket(order.Market).MarketType == TradingMarketTypeMain {
			return order.Market
		}
	}
	for _, position := range positions {
		if TradingFeeProfileForMarket(position.Market).MarketType == TradingMarketTypeMain {
			return position.Market
		}
	}
	for _, fill := range fills {
		if TradingFeeProfileForMarket(fill.Market).MarketType == TradingMarketTypeMain {
			return fill.Market
		}
	}
	for _, order := range orders {
		if order.Market != "" {
			return order.Market
		}
	}
	for _, position := range positions {
		if position.Market != "" {
			return position.Market
		}
	}
	for _, fill := range fills {
		if fill.Market != "" {
			return fill.Market
		}
	}
	return ""
}

func marketNameForClobPair(markets map[string]DYDXMarketConfig, clobPairID uint32) string {
	if clobPairID == 0 {
		return ""
	}
	for name, market := range markets {
		if market.ClobPairID == clobPairID {
			return name
		}
	}
	return ""
}

func scaledIntegerStringToRat(value, scale string) (*big.Rat, bool) {
	numerator, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	if !ok {
		return nil, false
	}
	denominator, ok := new(big.Int).SetString(strings.TrimSpace(scale), 10)
	if !ok || denominator.Sign() <= 0 {
		return nil, false
	}
	return new(big.Rat).SetFrac(numerator, denominator), true
}

func isTradingOpenOrderStatus(status string) bool {
	switch status {
	case TradingOrderStatusBroadcasted,
		TradingOrderStatusSubmitted,
		TradingOrderStatusCancelPending:
		return true
	default:
		return false
	}
}

func addDecimalInto(total *big.Rat, value string) {
	if parsed, ok := parseDecimalRat(value); ok {
		total.Add(total, parsed)
	}
}

func defaultDecimalString(value string) string {
	if parsed, ok := parseDecimalRat(value); ok {
		return ratToDecimalString(parsed, 18)
	}
	return "0"
}

func isZeroDecimalString(value string) bool {
	parsed, ok := parseDecimalRat(value)
	return ok && parsed.Sign() == 0
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func uint32String(value uint32) string {
	return new(big.Int).SetUint64(uint64(value)).String()
}

func winRateString(wins, total int) string {
	if total <= 0 {
		return "0"
	}
	rate := new(big.Rat).Mul(big.NewRat(int64(wins), int64(total)), big.NewRat(100, 1))
	return ratToDecimalString(rate, 4)
}

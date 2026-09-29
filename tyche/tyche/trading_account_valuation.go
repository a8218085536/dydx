package tyche

import (
	"context"
	"errors"
	"math/big"
)

var ErrTradingAccountValuationNotConfigured = errors.New("trading account valuation service is not configured")

type TradingAccountValuationRequest struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress"`
}

type TradingAccountValuation struct {
	UserID         string                         `json:"userId"`
	NECOAddress    string                         `json:"necoAddress"`
	TotalValueUSDT string                         `json:"totalValueUsdt"`
	Assets         []TradingAccountAssetValuation `json:"assets"`
	Trading        TradingAccountTradingValuation `json:"trading"`
}

type TradingAccountAssetValuation struct {
	Asset                 string `json:"asset"`
	NECOTokenAddress      string `json:"necoTokenAddress"`
	Decimals              uint8  `json:"decimals"`
	PriceUSDT             string `json:"priceUsdt"`
	PriceRoute            string `json:"priceRoute,omitempty"`
	PriceSnapshotID       string `json:"priceSnapshotId,omitempty"`
	NECOWalletRaw         string `json:"necoWalletRaw"`
	NECOWalletAmount      string `json:"necoWalletAmount"`
	LedgerAvailableRaw    string `json:"ledgerAvailableRaw"`
	LedgerAvailableAmount string `json:"ledgerAvailableAmount"`
	LedgerFrozenRaw       string `json:"ledgerFrozenRaw"`
	LedgerFrozenAmount    string `json:"ledgerFrozenAmount"`
	TotalRaw              string `json:"totalRaw"`
	TotalAmount           string `json:"totalAmount"`
	ValueUSDT             string `json:"valueUsdt"`
}

type TradingAccountTradingValuation struct {
	PositionMarginUSD  string `json:"positionMarginUsd"`
	OpenOrderMarginUSD string `json:"openOrderMarginUsd"`
	UnrealizedPNLUSD   string `json:"unrealizedPnlUsd"`
	EstimatedEquityUSD string `json:"estimatedEquityUsd"`
}

type TradingAccountValuationService struct {
	balances *TradingBalanceService
	summary  *TradingAccountSummaryService
	oracle   AssetUSDOracle
	mappings []AssetMapping
}

func NewTradingAccountValuationService(balances *TradingBalanceService, summary *TradingAccountSummaryService, oracle AssetUSDOracle, mappings []AssetMapping) *TradingAccountValuationService {
	return &TradingAccountValuationService{
		balances: balances,
		summary:  summary,
		oracle:   oracle,
		mappings: append([]AssetMapping(nil), mappings...),
	}
}

func (s *TradingAccountValuationService) Valuation(ctx context.Context, request TradingAccountValuationRequest) (TradingAccountValuation, error) {
	if s == nil || s.balances == nil || s.summary == nil || s.oracle == nil {
		return TradingAccountValuation{}, ErrTradingAccountValuationNotConfigured
	}
	valuation := TradingAccountValuation{
		UserID:      request.UserID,
		NECOAddress: request.NECOAddress,
	}
	total := big.NewRat(0, 1)

	assets, assetTotal, err := s.assetValuations(ctx, request)
	if err != nil {
		return TradingAccountValuation{}, err
	}
	valuation.Assets = assets
	total.Add(total, assetTotal)

	trading, tradingTotal, err := s.tradingValuation(ctx, request)
	if err != nil {
		return TradingAccountValuation{}, err
	}
	valuation.Trading = trading
	total.Add(total, tradingTotal)
	valuation.TotalValueUSDT = ratToDecimalString(total, 18)
	return valuation, nil
}

func (s *TradingAccountValuationService) assetValuations(ctx context.Context, request TradingAccountValuationRequest) ([]TradingAccountAssetValuation, *big.Rat, error) {
	out := make([]TradingAccountAssetValuation, 0)
	totalValue := big.NewRat(0, 1)
	for _, mapping := range s.mappings {
		if !mapping.EnabledForTrading || mapping.NECOAsset == "" || mapping.NECOTokenAddress == "" {
			continue
		}
		overview, err := s.balances.Overview(ctx, TradingBalanceOverviewRequest{
			UserID:      request.UserID,
			NECOAddress: request.NECOAddress,
			Asset:       mapping.NECOAsset,
		})
		if err != nil {
			return nil, nil, err
		}
		snapshot, err := s.oracle.AssetUSDPrice(ctx, mapping.NECOAsset)
		if err != nil {
			return nil, nil, err
		}
		price, ok := parseDecimalRat(snapshot.USDPrice)
		if !ok || price.Sign() <= 0 {
			return nil, nil, ErrNECOPriceInvalidReserve
		}
		asset, value, err := accountAssetValuationFromOverview(overview, mapping, snapshot, price)
		if err != nil {
			return nil, nil, err
		}
		totalValue.Add(totalValue, value)
		out = append(out, asset)
	}
	return out, totalValue, nil
}

func accountAssetValuationFromOverview(overview TradingBalanceOverview, mapping AssetMapping, snapshot AssetPriceSnapshot, price *big.Rat) (TradingAccountAssetValuation, *big.Rat, error) {
	wallet, err := rawAmountStringToRat(defaultLedgerAmount(overview.NECOWalletAvailable), mapping.Decimals)
	if err != nil {
		return TradingAccountAssetValuation{}, nil, err
	}
	available, err := rawAmountStringToRat(defaultLedgerAmount(overview.LedgerAvailable), mapping.Decimals)
	if err != nil {
		return TradingAccountAssetValuation{}, nil, err
	}
	frozen, err := rawAmountStringToRat(defaultLedgerAmount(overview.LedgerFrozen), mapping.Decimals)
	if err != nil {
		return TradingAccountAssetValuation{}, nil, err
	}
	totalAmount := big.NewRat(0, 1)
	totalAmount.Add(totalAmount, wallet)
	totalAmount.Add(totalAmount, available)
	totalAmount.Add(totalAmount, frozen)
	totalRaw, err := addRawAmountStrings(defaultLedgerAmount(overview.NECOWalletAvailable), defaultLedgerAmount(overview.LedgerAvailable), defaultLedgerAmount(overview.LedgerFrozen))
	if err != nil {
		return TradingAccountAssetValuation{}, nil, err
	}
	value := new(big.Rat).Mul(totalAmount, price)
	return TradingAccountAssetValuation{
		Asset:                 mapping.NECOAsset,
		NECOTokenAddress:      mapping.NECOTokenAddress,
		Decimals:              mapping.Decimals,
		PriceUSDT:             ratToDecimalString(price, 18),
		PriceRoute:            snapshot.RouteName,
		PriceSnapshotID:       snapshot.ID,
		NECOWalletRaw:         defaultLedgerAmount(overview.NECOWalletAvailable),
		NECOWalletAmount:      ratToDecimalString(wallet, 18),
		LedgerAvailableRaw:    defaultLedgerAmount(overview.LedgerAvailable),
		LedgerAvailableAmount: ratToDecimalString(available, 18),
		LedgerFrozenRaw:       defaultLedgerAmount(overview.LedgerFrozen),
		LedgerFrozenAmount:    ratToDecimalString(frozen, 18),
		TotalRaw:              totalRaw,
		TotalAmount:           ratToDecimalString(totalAmount, 18),
		ValueUSDT:             ratToDecimalString(value, 18),
	}, value, nil
}

func (s *TradingAccountValuationService) tradingValuation(ctx context.Context, request TradingAccountValuationRequest) (TradingAccountTradingValuation, *big.Rat, error) {
	summary, err := s.summary.Summary(ctx, TradingAccountSummaryRequest{
		UserID:      request.UserID,
		NECOAddress: request.NECOAddress,
	})
	if err != nil {
		return TradingAccountTradingValuation{}, nil, err
	}
	positionMargin := big.NewRat(0, 1)
	for _, position := range summary.Positions {
		addDecimalInto(positionMargin, position.MarginUSD)
	}
	openOrderMargin := big.NewRat(0, 1)
	for _, order := range summary.OpenOrders {
		addDecimalInto(openOrderMargin, order.MarginUSD)
	}
	unrealized := big.NewRat(0, 1)
	addDecimalInto(unrealized, summary.Stats.UnrealizedPNLUSD)

	estimated := big.NewRat(0, 1)
	estimated.Add(estimated, positionMargin)
	estimated.Add(estimated, openOrderMargin)
	estimated.Add(estimated, unrealized)
	return TradingAccountTradingValuation{
		PositionMarginUSD:  ratToDecimalString(positionMargin, 18),
		OpenOrderMarginUSD: ratToDecimalString(openOrderMargin, 18),
		UnrealizedPNLUSD:   ratToDecimalString(unrealized, 18),
		EstimatedEquityUSD: ratToDecimalString(estimated, 18),
	}, estimated, nil
}

func rawAmountStringToRat(value string, decimals uint8) (*big.Rat, error) {
	raw, ok := parseLedgerAmount(value)
	if !ok {
		return nil, ErrTradingQuoteInvalidAmount
	}
	return scaleAtomicAmount(raw, decimals), nil
}

func addRawAmountStrings(values ...string) (string, error) {
	total := big.NewInt(0)
	for _, value := range values {
		amount, ok := parseLedgerAmount(value)
		if !ok {
			return "", ErrTradingQuoteInvalidAmount
		}
		total.Add(total, amount)
	}
	return total.String(), nil
}

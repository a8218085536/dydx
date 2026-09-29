package tyche

import (
	"context"
	"errors"
	"math/big"
)

var ErrNECOPriceQuoteInvalidAsset = errors.New("neco price quote invalid asset")

type NECOPriceQuoteRequest struct {
	FromAsset string `json:"fromAsset"`
	ToAsset   string `json:"toAsset"`
	Amount    string `json:"amount,omitempty"`
}

type NECOPriceQuote struct {
	FromAsset       string   `json:"fromAsset"`
	ToAsset         string   `json:"toAsset"`
	FromAmount      string   `json:"fromAmount"`
	ToAmount        string   `json:"toAmount"`
	Price           string   `json:"price"`
	FromUSDPrice    string   `json:"fromUsdPrice"`
	ToUSDPrice      string   `json:"toUsdPrice"`
	PriceRoute      string   `json:"priceRoute,omitempty"`
	PriceSnapshotID string   `json:"priceSnapshotId,omitempty"`
	Source          string   `json:"source,omitempty"`
	BlockNumber     uint64   `json:"blockNumber,omitempty"`
	MinLiquidity    string   `json:"minLiquidityUsd,omitempty"`
	Pairs           []string `json:"pairs,omitempty"`
}

type NECOPriceQuoteService struct {
	oracle AssetUSDOracle
}

func NewNECOPriceQuoteService(oracle AssetUSDOracle) *NECOPriceQuoteService {
	return &NECOPriceQuoteService{oracle: oracle}
}

func (s *NECOPriceQuoteService) Quote(ctx context.Context, request NECOPriceQuoteRequest) (NECOPriceQuote, error) {
	if s == nil || s.oracle == nil {
		return NECOPriceQuote{}, ErrNECOPriceOracleNotConfigured
	}
	fromAsset := normalizeAsset(request.FromAsset)
	toAsset := normalizeAsset(request.ToAsset)
	if fromAsset == "" || toAsset == "" {
		return NECOPriceQuote{}, ErrNECOPriceQuoteInvalidAsset
	}
	amountText := defaultString(request.Amount, "1")
	amount, ok := parseDecimalRat(amountText)
	if !ok || amount.Sign() <= 0 {
		return NECOPriceQuote{}, ErrTradingQuoteInvalidAmount
	}

	fromSnapshot, err := s.assetUSDPrice(ctx, fromAsset)
	if err != nil {
		return NECOPriceQuote{}, err
	}
	toSnapshot, err := s.assetUSDPrice(ctx, toAsset)
	if err != nil {
		return NECOPriceQuote{}, err
	}
	fromUSD, ok := parseDecimalRat(fromSnapshot.USDPrice)
	if !ok || fromUSD.Sign() <= 0 {
		return NECOPriceQuote{}, ErrNECOPriceInvalidReserve
	}
	toUSD, ok := parseDecimalRat(toSnapshot.USDPrice)
	if !ok || toUSD.Sign() <= 0 {
		return NECOPriceQuote{}, ErrNECOPriceInvalidReserve
	}

	price := new(big.Rat).Quo(fromUSD, toUSD)
	toAmount := new(big.Rat).Mul(amount, price)
	return NECOPriceQuote{
		FromAsset:       fromAsset,
		ToAsset:         toAsset,
		FromAmount:      ratToDecimalString(amount, 18),
		ToAmount:        ratToDecimalString(toAmount, 18),
		Price:           ratToDecimalString(price, 18),
		FromUSDPrice:    ratToDecimalString(fromUSD, 18),
		ToUSDPrice:      ratToDecimalString(toUSD, 18),
		PriceRoute:      fromSnapshot.RouteName,
		PriceSnapshotID: fromSnapshot.ID,
		Source:          fromSnapshot.Source,
		BlockNumber:     fromSnapshot.BlockNumber,
		MinLiquidity:    fromSnapshot.MinLiquidity,
		Pairs:           append([]string(nil), fromSnapshot.Pairs...),
	}, nil
}

func (s *NECOPriceQuoteService) assetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if asset == "USD" {
		return AssetPriceSnapshot{
			ID:        "neco-stable-route:USD-USD",
			Asset:     "USD",
			USDAsset:  "USD",
			USDPrice:  "1",
			RouteName: "USD-USD",
			Source:    "stable_usd:USD-USD",
		}, nil
	}
	return s.oracle.AssetUSDPrice(ctx, asset)
}

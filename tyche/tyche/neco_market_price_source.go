package tyche

import (
	"context"
	"strings"
)

type NECOAssetMarketPriceSourceConfig struct {
	Markets       []DYDXMarketConfig       `json:"markets" toml:"markets"`
	AssetMappings []AssetMapping           `json:"assetMappings" toml:"assetMappings"`
	Oracle        AssetUSDOracle           `json:"-" toml:"-"`
	Fallback      TradingMarketPriceSource `json:"-" toml:"-"`
}

type NECOAssetMarketPriceSource struct {
	markets  map[string]DYDXMarketConfig
	mappings map[string]AssetMapping
	oracle   AssetUSDOracle
	fallback TradingMarketPriceSource
}

func NewNECOAssetMarketPriceSource(conf NECOAssetMarketPriceSourceConfig) *NECOAssetMarketPriceSource {
	source := &NECOAssetMarketPriceSource{
		markets:  map[string]DYDXMarketConfig{},
		mappings: map[string]AssetMapping{},
		oracle:   conf.Oracle,
		fallback: conf.Fallback,
	}
	for _, market := range conf.Markets {
		source.markets[normalizeDYDXMarket(market.Market)] = market
	}
	for _, mapping := range conf.AssetMappings {
		if !mapping.EnabledForTrading || normalizeAsset(mapping.DYDXAsset) != "USD" {
			continue
		}
		asset := normalizeAsset(mapping.NECOAsset)
		if asset == "" {
			continue
		}
		for _, market := range necoAssetUSDMarketAliases(asset) {
			source.mappings[market] = mapping
		}
	}
	return source
}

func (s *NECOAssetMarketPriceSource) MarketPrice(ctx context.Context, request TradingMarketPriceRequest) (TradingMarketPriceSnapshot, error) {
	if s == nil {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceSourceNotConfigured
	}
	marketName := normalizeDYDXMarket(request.Market)
	if mapping, ok := s.mappings[marketName]; ok {
		return s.necoMarketPrice(ctx, marketName, mapping)
	}
	if s.fallback == nil {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceSourceNotConfigured
	}
	return s.fallback.MarketPrice(ctx, request)
}

func (s *NECOAssetMarketPriceSource) necoMarketPrice(ctx context.Context, marketName string, mapping AssetMapping) (TradingMarketPriceSnapshot, error) {
	if s == nil || s.oracle == nil {
		return TradingMarketPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	snapshot, err := s.oracle.AssetUSDPrice(ctx, mapping.NECOAsset)
	if err != nil {
		return TradingMarketPriceSnapshot{}, err
	}
	if snapshot.USDPrice == "" {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceInvalid
	}
	if _, ok := parseDecimalRat(snapshot.USDPrice); !ok {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceInvalid
	}
	market, ok := s.markets[marketName]
	if !ok {
		return TradingMarketPriceSnapshot{}, ErrDYDXMarketNotConfigured
	}
	source := strings.TrimSpace(snapshot.Source)
	if source == "" {
		source = "neco-swap"
	} else {
		source = "neco-swap:" + source
	}
	return TradingMarketPriceSnapshot{
		Market:     market.Market,
		MarketID:   market.ClobPairID,
		Price:      snapshot.USDPrice,
		Source:     source,
		SnapshotID: snapshot.ID,
	}, nil
}

func necoAssetUSDMarketAliases(asset string) []string {
	asset = normalizeAsset(asset)
	if asset == "" {
		return nil
	}
	markets := []string{asset + "-USD"}
	if asset == "FDEX" {
		markets = append(markets, builtInDEDXUSDMarket)
	}
	return markets
}

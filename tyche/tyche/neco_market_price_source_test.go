package tyche

import (
	"context"
	"testing"
)

func TestNECOAssetMarketPriceSourceUsesSwapPriceForMappedUSDMarket(t *testing.T) {
	source := NewNECOAssetMarketPriceSource(NECOAssetMarketPriceSourceConfig{
		Markets: []DYDXMarketConfig{
			{Market: "FDEX-USD", ClobPairID: 302},
			{Market: "ETH-USD", ClobPairID: 1},
		},
		AssetMappings: []AssetMapping{
			{
				NECOAsset:         "FDEX",
				DYDXAsset:         "USD",
				PriceRoute:        "FDEX-USDT",
				EnabledForTrading: true,
			},
		},
		Oracle: staticMarketAssetUSDOracle{
			snapshots: map[string]AssetPriceSnapshot{
				"FDEX": {
					ID:           "neco-v2-route:123:fdex-usdt",
					Asset:        "FDEX",
					USDAsset:     "USD",
					USDPrice:     "4.125",
					RouteName:    "FDEX-USDT",
					Source:       "v2_pair:FDEX-USDT",
					BlockNumber:  123,
					MinLiquidity: "10000",
					Pairs:        []string{"fdex-usdt"},
				},
			},
		},
		Fallback: staticMarketPriceSource{
			snapshot: TradingMarketPriceSnapshot{
				Market:     "ETH-USD",
				MarketID:   1,
				Price:      "2000",
				Source:     "dydx-rest",
				SnapshotID: "dydx-market-price:1:2000",
			},
		},
	})

	dedx, err := source.MarketPrice(context.Background(), TradingMarketPriceRequest{Market: "FDEX-USD"})
	if err != nil {
		t.Fatalf("DEDX MarketPrice() error = %v", err)
	}
	if dedx.Market != "FDEX-USD" || dedx.MarketID != 302 || dedx.Price != "4.125" || dedx.Source != "neco-swap:v2_pair:FDEX-USDT" || dedx.SnapshotID != "neco-v2-route:123:fdex-usdt" {
		t.Fatalf("DEDX market snapshot = %+v, want FDEX NECO swap price", dedx)
	}

	eth, err := source.MarketPrice(context.Background(), TradingMarketPriceRequest{Market: "ETH-USD"})
	if err != nil {
		t.Fatalf("ETH MarketPrice() error = %v", err)
	}
	if eth.Price != "2000" || eth.Source != "dydx-rest" {
		t.Fatalf("ETH market snapshot = %+v, want fallback dYdX price", eth)
	}
}

func TestNECOAssetMarketPriceSourceDoesNotFallbackForMappedMarketWhenOracleFails(t *testing.T) {
	source := NewNECOAssetMarketPriceSource(NECOAssetMarketPriceSourceConfig{
		Markets: []DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}},
		AssetMappings: []AssetMapping{{
			NECOAsset:         "FDEX",
			DYDXAsset:         "USD",
			PriceRoute:        "FDEX-USDT",
			EnabledForTrading: true,
		}},
		Oracle:   staticMarketAssetUSDOracle{},
		Fallback: staticMarketPriceSource{snapshot: TradingMarketPriceSnapshot{Market: "FDEX-USD", Price: "1"}},
	})

	_, err := source.MarketPrice(context.Background(), TradingMarketPriceRequest{Market: "FDEX-USD"})
	if err == nil {
		t.Fatal("MarketPrice() error = nil, want oracle error instead of fallback for mapped DEDX market")
	}
}

type staticMarketAssetUSDOracle struct {
	snapshots map[string]AssetPriceSnapshot
}

func (o staticMarketAssetUSDOracle) AssetUSDPrice(_ context.Context, asset string) (AssetPriceSnapshot, error) {
	snapshot, ok := o.snapshots[normalizeAsset(asset)]
	if !ok {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	return snapshot, nil
}

type staticMarketPriceSource struct {
	snapshot TradingMarketPriceSnapshot
	err      error
}

func (s staticMarketPriceSource) MarketPrice(context.Context, TradingMarketPriceRequest) (TradingMarketPriceSnapshot, error) {
	return s.snapshot, s.err
}

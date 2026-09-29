package tyche

import (
	"strings"
	"testing"
)

func TestConfigInitAllowsRoutePriceOracleWithoutLegacyDEDXNECOFields(t *testing.T) {
	conf := Config{
		ChainType:           ChainTypeDYDX,
		InternalAPIKey:      strings.Repeat("a", 32),
		DYDXTradingURL:      "http://127.0.0.1:8090/dydx/trading",
		DYDXChainID:         "flydex-mainnet-1",
		DYDXHeightURL:       "http://127.0.0.1:26657/status",
		DYDXMarkets:         []DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}},
		NECORPCURL:          "https://chainrpc.necochain.com/",
		ExpectedMD5Password: "x",
		AllowedIP:           "*",
		AssetMappings: []AssetMapping{
			{
				NECOTokenAddress:  "0x1000000000000000000000000000000000000001",
				NECOAsset:         "FDEX",
				DYDXAsset:         "USD",
				Decimals:          18,
				PriceRoute:        "FDEX-USDT",
				EnabledForTrading: true,
			},
			{
				NECOTokenAddress:  "0x1000000000000000000000000000000000000002",
				NECOAsset:         "USDT",
				DYDXAsset:         "USD",
				Decimals:          18,
				PriceRoute:        "USDT-USD",
				EnabledForTrading: true,
			},
		},
		PriceRoutes: []PriceRouteConfig{
			{
				Name:     "FDEX-USDT",
				Asset:    "FDEX",
				USDAsset: "USD",
				Hops: []PriceRouteHopConfig{{
					Pair:         "0x2000000000000000000000000000000000000001",
					FromToken:    "0x1000000000000000000000000000000000000001",
					ToToken:      "0x1000000000000000000000000000000000000002",
					FromDecimals: 18,
					ToDecimals:   18,
				}},
			},
			{Name: "USDT-USD", Asset: "USDT", USDAsset: "USD"},
		},
		NECOPriceOracle: NECOPriceOracleConfig{
			Enabled:         true,
			Mode:            NECOPriceOracleModeV2Pair,
			MinLiquidityUSD: "1000",
		},
	}

	if err := conf.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if conf.NECOWallet.TokenContracts["FDEX"] != "0x1000000000000000000000000000000000000001" {
		t.Fatalf("FDEX token contract = %q", conf.NECOWallet.TokenContracts["FDEX"])
	}
}

func TestConfigInitRejectsInvalidRoutePriceOracleHop(t *testing.T) {
	conf := Config{
		ChainType:           ChainTypeDYDX,
		InternalAPIKey:      strings.Repeat("a", 32),
		DYDXTradingURL:      "http://127.0.0.1:8090/dydx/trading",
		DYDXChainID:         "flydex-mainnet-1",
		DYDXHeightURL:       "http://127.0.0.1:26657/status",
		DYDXMarkets:         []DYDXMarketConfig{{Market: "FDEX-USD", ClobPairID: 302}},
		NECORPCURL:          "https://chainrpc.necochain.com/",
		ExpectedMD5Password: "x",
		AllowedIP:           "*",
		PriceRoutes: []PriceRouteConfig{{
			Name:     "FDEX-USDT",
			Asset:    "FDEX",
			USDAsset: "USD",
			Hops: []PriceRouteHopConfig{{
				Pair:         "not-an-address",
				FromToken:    "0x1000000000000000000000000000000000000001",
				ToToken:      "0x1000000000000000000000000000000000000002",
				FromDecimals: 18,
				ToDecimals:   18,
			}},
		}},
		NECOPriceOracle: NECOPriceOracleConfig{Enabled: true, Mode: NECOPriceOracleModeV2Pair},
	}

	if err := conf.Init(); err == nil || !strings.Contains(err.Error(), "priceRoutes FDEX-USDT hop pair is invalid address") {
		t.Fatalf("Init() error = %v, want invalid route hop pair", err)
	}
}

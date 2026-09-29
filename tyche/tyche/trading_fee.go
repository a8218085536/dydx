package tyche

import "strings"

const (
	TradingMarketTypeMain  = "main"
	TradingMarketTypeMajor = "major"

	TradingMainMakerFeeRate  = "-0.001"
	TradingMainTakerFeeRate  = "0.0045"
	TradingMajorMakerFeeRate = "-0.0003"
	TradingMajorTakerFeeRate = "0.0015"
)

type TradingFeeProfile struct {
	MarketType   string `json:"marketType"`
	MakerFeeRate string `json:"makerFeeRate"`
	TakerFeeRate string `json:"takerFeeRate"`
}

func TradingFeeProfileForMarket(market string) TradingFeeProfile {
	switch tradingMarketBaseAsset(market) {
	case "DEDX", "NECO":
		return TradingFeeProfile{
			MarketType:   TradingMarketTypeMain,
			MakerFeeRate: TradingMainMakerFeeRate,
			TakerFeeRate: TradingMainTakerFeeRate,
		}
	default:
		return TradingFeeProfile{
			MarketType:   TradingMarketTypeMajor,
			MakerFeeRate: TradingMajorMakerFeeRate,
			TakerFeeRate: TradingMajorTakerFeeRate,
		}
	}
}

func tradingMarketBaseAsset(market string) string {
	market = normalizeDYDXMarket(market)
	if before, _, ok := strings.Cut(market, "-"); ok {
		return before
	}
	return market
}

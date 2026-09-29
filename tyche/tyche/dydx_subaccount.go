package tyche

const (
	builtInDEDXUSDMarket               = "FDEX-USD"
	builtInDEDXUSDClobPairID           = uint32(302)
	builtInDEDXUSDSubaccountNumber     = uint32(127)
	defaultDYDXTradingSubaccountNumber = uint32(0)
)

func builtInDYDXSubaccountNumberForMarket(market string, clobPairID uint32, fallback uint32) uint32 {
	if normalizeDYDXMarket(market) == builtInDEDXUSDMarket || clobPairID == builtInDEDXUSDClobPairID {
		return builtInDEDXUSDSubaccountNumber
	}
	return fallback
}

func explicitOrBuiltInDYDXSubaccountNumber(explicit uint32, market string, clobPairID uint32, fallback uint32) uint32 {
	if explicit != 0 {
		return explicit
	}
	return builtInDYDXSubaccountNumberForMarket(market, clobPairID, fallback)
}

func clobPairIDForMarket(markets map[string]DYDXMarketConfig, market string) uint32 {
	if markets == nil {
		return 0
	}
	config, ok := markets[normalizeDYDXMarket(market)]
	if !ok {
		return 0
	}
	return config.ClobPairID
}

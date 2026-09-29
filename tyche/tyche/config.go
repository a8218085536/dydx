package tyche

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const (
	ChainTypeEVM  = "evm"
	ChainTypeDYDX = "dydx"
)

var (
	ErrDYDXTradingURLNotConfigured        = errors.New("dydx trading url is not configured")
	ErrInternalAPIKeyNotConfigured        = errors.New("internal api key must be at least 32 characters in dydx mode")
	ErrDYDXStaticAddressProviderForbidden = errors.New("dydx static address provider is forbidden in dydx mode")
	ErrDYDXLegacyBroadcastForbidden       = errors.New("dydx legacy broadcast url is forbidden in dydx mode; use dydxTradingURL")
)

type Config struct {
	Listen                        string `toml:"listen" json:"listen"`
	ChainType                     string `toml:"chainType" json:"chainType"`
	ReadTimeout                   int    `toml:"readTimeout" json:"readTimeout"`
	WriteTimeout                  int    `toml:"writeTimeout" json:"writeTimeout"`
	MaxHeaderBytes                int    `toml:"maxHeaderBytes" json:"maxHeaderBytes"`
	Account                       string `toml:"account" json:"account"`
	PrivateKey                    string `toml:"privateKey" json:"privateKey"`
	DYDXBroadcastURL              string `toml:"dydxBroadcastURL" json:"dydxBroadcastURL"`
	DYDXChainBroadcastURL         string `toml:"dydxChainBroadcastURL" json:"dydxChainBroadcastURL"`
	DYDXTradingURL                string `toml:"dydxTradingURL" json:"dydxTradingURL"`
	DYDXWalletCreateURL           string `toml:"dydxWalletCreateURL" json:"dydxWalletCreateURL"`
	DYDXSubaccountDepositURL      string `toml:"dydxSubaccountDepositURL" json:"dydxSubaccountDepositURL"`
	DYDXChainID                   string `toml:"dydxChainID" json:"dydxChainID"`
	DYDXHeightURL                 string `toml:"dydxHeightURL" json:"dydxHeightURL"`
	DYDXSubaccountNumber          uint32 `toml:"dydxSubaccountNumber" json:"dydxSubaccountNumber"`
	DYDXCollateralAssetID         uint32 `toml:"dydxCollateralAssetId" json:"dydxCollateralAssetId"`
	DYDXCollateralQuantumsPerUSD  string `toml:"dydxCollateralQuantumsPerUsd" json:"dydxCollateralQuantumsPerUsd"`
	DYDXMinStatefulOrderMarginUSD string `toml:"dydxMinStatefulOrderMarginUsd" json:"dydxMinStatefulOrderMarginUsd"`
	DYDXRetainedCollateralUSD     string `toml:"dydxRetainedCollateralUsd" json:"dydxRetainedCollateralUsd"`
	// DYDXGoodTilBlockDelta is a legacy short-term order setting retained so older toml files still decode.
	DYDXGoodTilBlockDelta        uint32                `toml:"dydxGoodTilBlockDelta" json:"dydxGoodTilBlockDelta"`
	DYDXGoodTilSeconds           uint32                `toml:"dydxGoodTilSeconds" json:"dydxGoodTilSeconds"`
	DYDXMarkets                  []DYDXMarketConfig    `toml:"dydxMarkets" json:"dydxMarkets"`
	DYDXEventSourceURL           string                `toml:"dydxEventSourceURL" json:"dydxEventSourceURL"`
	DYDXEventSourceAPIKey        string                `toml:"dydxEventSourceAPIKey" json:"-"`
	DYDXMarketPriceURL           string                `toml:"dydxMarketPriceURL" json:"dydxMarketPriceURL"`
	MarketOrderSlippageBps       uint32                `toml:"marketOrderSlippageBps" json:"marketOrderSlippageBps"`
	CloseSettlementMode          string                `toml:"closeSettlementMode" json:"closeSettlementMode"`
	CloseSettlementAsset         string                `toml:"closeSettlementAsset" json:"closeSettlementAsset"`
	CloseSettlementAssetDecimals uint8                 `toml:"closeSettlementAssetDecimals" json:"closeSettlementAssetDecimals"`
	TradeCloseCallbackURL        string                `toml:"tradeCloseCallbackURL" json:"tradeCloseCallbackURL"`
	DYDXAutoAddressPrefix        string                `toml:"dydxAutoAddressPrefix" json:"dydxAutoAddressPrefix"`
	DYDXSignatureVerifyURL       string                `toml:"dydxSignatureVerifyURL" json:"dydxSignatureVerifyURL"`
	NECORPCURL                   string                `toml:"necoRpcURL" json:"necoRpcURL"`
	InternalAPIKey               string                `toml:"internalAPIKey" json:"internalAPIKey"`
	NECODeposit                  NECODepositConfig     `toml:"necoDeposit" json:"necoDeposit"`
	AssetMappings                []AssetMapping        `toml:"assetMappings" json:"assetMappings"`
	PriceRoutes                  []PriceRouteConfig    `toml:"priceRoutes" json:"priceRoutes"`
	NECOWallet                   NECOWalletConfig      `toml:"necoWallet" json:"necoWallet"`
	NECOHotWalletPrivateKey      string                `toml:"necoHotWalletPrivateKey" json:"-"`
	NECOPriceOracle              NECOPriceOracleConfig `toml:"necoPriceOracle" json:"necoPriceOracle"`
	EstimatorJS                  string                `toml:"estimatorJS" json:"estimatorJS"`
	ErrorURI                     string                `toml:"errorURI" json:"errorURI"`
	FailedURI                    string                `toml:"failedURI" json:"failedURI"`
	SucceedURI                   string                `toml:"succeedURI" json:"succeedURI"`
	GasPriceUpdateInterval       int64                 `toml:"gasPriceUpdateInterval" json:"gasPriceUpdateInterval"`
	NonceCheckInterval           int64                 `toml:"nonceCheckInterval" json:"nonceCheckInterval"`
	NonceKeepalive               int64                 `toml:"nonceKeepalive" json:"nonceKeepalive"`
	MaxBumpingGasTimes           int64                 `toml:"maxBumpingGasTimes" json:"maxBumpingGasTimes"`
	ErrorNumberOfConcurrent      int64                 `toml:"errorNumberOfConcurrent" json:"errorNumberOfConcurrent"`
	FailedNumberOfConcurrent     int64                 `toml:"failedNumberOfConcurrent" json:"failedNumberOfConcurrent"`
	PendingNumberOfConcurrent    int64                 `toml:"pendingNumberOfConcurrent" json:"pendingNumberOfConcurrent"`
	SentNumberOfConcurrent       int64                 `toml:"sentNumberOfConcurrent" json:"sentNumberOfConcurrent"`
	SucceedNumberOfConcurrent    int64                 `toml:"succeedNumberOfConcurrent" json:"succeedNumberOfConcurrent"`
	PendingRetryInterval         int64                 `toml:"pendingRetryInterval" json:"pendingRetryInterval"`
	SentRetryInterval            int64                 `toml:"sentRetryInterval" json:"sentRetryInterval"`
	RedisRetryInterval           int64                 `toml:"redisRetryInterval" json:"redisRetryInterval"`
	NetworkRetryInterval         int64                 `toml:"networkRetryInterval" json:"networkRetryInterval"`
	WaitMinedRetryInterval       int64                 `toml:"waitMinedRetryInterval" json:"waitMinedRetryInterval"`
	CallbackRetryInterval        int64                 `toml:"callbackRetryInterval" json:"callbackRetryInterval"`
	EnableTLS                    bool                  `toml:"enableTLS" json:"enableTLS"`
	CertFile                     string                `toml:"certFile" json:"certFile"`
	KeyFile                      string                `toml:"keyFile" json:"keyFile"`
	Contracts                    []ContractConfig      `toml:"contracts" json:"contracts"`
	IsPaused                     bool                  `toml:"is_paused"`
	ExpectedMD5Password          string                `toml:"expectedMD5Password" json:"expectedMD5Password"`
	AllowedIP                    string                `toml:"allowedIP" json:"allowedIP"`
}

type ContractConfig struct {
	Addr string `toml:"addr" json:"addr"`
	ABI  string `toml:"abi" json:"abi"`
}

func (c *Config) Init() error {

	c.IsPaused = false
	c.ChainType = strings.ToLower(c.ChainType)
	if c.ChainType == "" {
		c.ChainType = ChainTypeEVM
	}

	switch c.ChainType {
	case ChainTypeEVM:
		if !common.IsHexAddress(c.Account) {
			return errors.New("account is invalid address")
		}

		for _, v := range c.Contracts {
			if !common.IsHexAddress(v.Addr) {
				return errors.New("contract Addr is invalid address")
			}
			if v.ABI == "" {
				return errors.New("contract ABI cannot be set to empty")
			}
		}
		if c.EstimatorJS == "" {
			return errors.New("estimatorJS cannot be set to empty")
		}
	case ChainTypeDYDX:
		if len(strings.TrimSpace(c.InternalAPIKey)) < 32 {
			return ErrInternalAPIKeyNotConfigured
		}
		if c.DYDXAutoAddressPrefix != "" {
			return ErrDYDXStaticAddressProviderForbidden
		}
		if c.DYDXBroadcastURL != "" || c.DYDXChainBroadcastURL != "" {
			return ErrDYDXLegacyBroadcastForbidden
		}
		if c.DYDXTradingURL == "" {
			return ErrDYDXTradingURLNotConfigured
		}
		if c.DYDXChainID == "" {
			return ErrDYDXChainIDNotConfigured
		}
		if c.DYDXHeightURL == "" {
			return ErrDYDXHeightProviderNotConfigured
		}
		if len(c.DYDXMarkets) == 0 {
			return ErrDYDXMarketNotConfigured
		}
		for i := range c.DYDXMarkets {
			if strings.TrimSpace(c.DYDXMarkets[i].StepBaseQuantums) == "" {
				c.DYDXMarkets[i].StepBaseQuantums = "1000000"
			}
		}
		if c.DYDXGoodTilSeconds == 0 {
			c.DYDXGoodTilSeconds = defaultDYDXGoodTilSeconds
		}
		if c.DYDXGoodTilSeconds > dydxStatefulOrderTimeWindowSeconds {
			return ErrDYDXGoodTilSecondsTooLarge
		}
		if c.DYDXCollateralQuantumsPerUSD == "" {
			c.DYDXCollateralQuantumsPerUSD = "1000000"
		}
		if c.DYDXMinStatefulOrderMarginUSD == "" {
			c.DYDXMinStatefulOrderMarginUSD = defaultDYDXMinStatefulOrderMarginUSD
		}
		if minimum, ok := parseDecimalRat(c.DYDXMinStatefulOrderMarginUSD); !ok || minimum.Sign() < 0 {
			return errors.New("dydxMinStatefulOrderMarginUsd is invalid")
		}
		if c.DYDXRetainedCollateralUSD == "" {
			c.DYDXRetainedCollateralUSD = defaultDYDXRetainedCollateralUSD
		}
		if retained, ok := parseDecimalRat(c.DYDXRetainedCollateralUSD); !ok || retained.Sign() < 0 {
			return errors.New("dydxRetainedCollateralUsd is invalid")
		}
		c.CloseSettlementMode = NormalizeCloseSettlementMode(c.CloseSettlementMode)
		if c.CloseSettlementAsset == "" {
			c.CloseSettlementAsset = defaultCloseSettlementAsset
		}
		if c.CloseSettlementAssetDecimals == 0 {
			c.CloseSettlementAssetDecimals = defaultCloseSettlementAssetDecimals
		}
	default:
		return errors.New("chainType must be evm or dydx")
	}
	if c.ExpectedMD5Password == "" {
		return errors.New("ExpectedMD5Password cannot be set to empty")
	}
	if c.AllowedIP == "" {
		return errors.New("AllowedIP cannot be set to empty")
	}

	if c.GasPriceUpdateInterval < 1 {
		c.GasPriceUpdateInterval = 1
	}
	if c.NonceCheckInterval < 1 {
		c.NonceCheckInterval = 1
	}
	if c.NonceKeepalive < 1 {
		c.NonceKeepalive = 1
	}
	if c.MaxBumpingGasTimes < 0 {
		c.MaxBumpingGasTimes = 0
	}
	if c.ErrorNumberOfConcurrent < 1 {
		c.ErrorNumberOfConcurrent = 1
	}
	if c.FailedNumberOfConcurrent < 1 {
		c.FailedNumberOfConcurrent = 1
	}
	if c.PendingNumberOfConcurrent < 1 {
		c.PendingNumberOfConcurrent = 1
	}
	if c.SentNumberOfConcurrent < c.MaxBumpingGasTimes*2 {
		c.SentNumberOfConcurrent = c.MaxBumpingGasTimes * 2
	}
	if c.SucceedNumberOfConcurrent < 1 {
		c.SucceedNumberOfConcurrent = 1
	}
	if c.PendingRetryInterval < 1 {
		c.PendingRetryInterval = 1
	}
	if c.SentRetryInterval < 1 {
		c.SentRetryInterval = 1
	}
	if c.RedisRetryInterval < 1 {
		c.RedisRetryInterval = 1
	}
	if c.NetworkRetryInterval < 1 {
		c.NetworkRetryInterval = 1
	}
	if c.WaitMinedRetryInterval < 1 {
		c.WaitMinedRetryInterval = 1
	}
	if c.CallbackRetryInterval < 1 {
		c.CallbackRetryInterval = 1
	}
	if c.EnableTLS {
		switch {
		case c.CertFile == "":
			return errors.New("cretFile cannot be set to empty")
		case c.KeyFile == "":
			return errors.New("keyFile cannot be set to empty")
		}
	}
	if c.NECOHotWalletPrivateKey != "" && len(c.NECOWallet.HotWalletKey) == 0 {
		key := strings.TrimPrefix(c.NECOHotWalletPrivateKey, "0x")
		decoded, err := hex.DecodeString(key)
		if err != nil {
			return errors.New("necoHotWalletPrivateKey is invalid hex")
		}
		c.NECOWallet.HotWalletKey = decoded
	}
	c.deriveNECOWalletTokenContracts()
	if err := c.initNECOPriceOracle(); err != nil {
		return err
	}
	return nil
}

func (c *Config) deriveNECOWalletTokenContracts() {
	c.NECOWallet.TokenContracts = normalizeTokenContractMap(c.NECOWallet.TokenContracts)
	for _, mapping := range c.AssetMappings {
		asset := normalizeAsset(mapping.NECOAsset)
		if asset == "" || mapping.NECOTokenAddress == "" {
			continue
		}
		if _, exists := c.NECOWallet.TokenContracts[asset]; exists {
			continue
		}
		c.NECOWallet.TokenContracts[asset] = mapping.NECOTokenAddress
	}
}

func normalizeTokenContractMap(contracts map[string]string) map[string]string {
	normalized := map[string]string{}
	for asset, address := range contracts {
		key := normalizeAsset(asset)
		if key == "" {
			continue
		}
		if _, exists := normalized[key]; exists {
			continue
		}
		normalized[key] = address
	}
	return normalized
}

func (c *Config) initNECOPriceOracle() error {
	if !c.NECOPriceOracle.Enabled {
		return nil
	}
	if c.NECORPCURL == "" {
		return errors.New("necoRpcURL cannot be empty when necoPriceOracle is enabled")
	}
	if c.NECOPriceOracle.Mode == "" {
		c.NECOPriceOracle.Mode = NECOPriceOracleModeV2Pair
	}
	if c.NECOPriceOracle.Mode != NECOPriceOracleModeV2Pair {
		return errors.New("necoPriceOracle mode must be v2_pair")
	}
	if len(c.PriceRoutes) > 0 {
		if err := validateNECOPriceRoutes(c.PriceRoutes); err != nil {
			return err
		}
	} else {
		addresses := map[string]string{
			"dedxNecoPair": c.NECOPriceOracle.DEDXNECOPair,
			"necoUsdPair":  c.NECOPriceOracle.NECOUSDPair,
			"dedxToken":    c.NECOPriceOracle.DEDXToken,
			"necoToken":    c.NECOPriceOracle.NECOToken,
			"usdToken":     c.NECOPriceOracle.USDToken,
		}
		for name, address := range addresses {
			if !common.IsHexAddress(address) {
				return errors.New("necoPriceOracle " + name + " is invalid address")
			}
		}
	}
	if c.NECOPriceOracle.MinLiquidityUSD != "" {
		minLiquidity, ok := parseDecimalRat(c.NECOPriceOracle.MinLiquidityUSD)
		if !ok || minLiquidity.Sign() < 0 {
			return errors.New("necoPriceOracle minLiquidityUsd is invalid")
		}
	}
	if c.NECOPriceOracle.MaxPriceDeviationBps > 10000 {
		return errors.New("necoPriceOracle maxPriceDeviationBps is invalid")
	}
	return nil
}

func validateNECOPriceRoutes(routes []PriceRouteConfig) error {
	for _, route := range routes {
		name := priceRouteDisplayName(route)
		if normalizeAsset(route.Asset) == "" {
			return errors.New("necoPriceOracle priceRoutes " + name + " asset cannot be empty")
		}
		usdAsset := normalizeAsset(defaultString(route.USDAsset, "USD"))
		if usdAsset != "USD" {
			return errors.New("necoPriceOracle priceRoutes " + name + " usdAsset must be USD")
		}
		for _, hop := range route.Hops {
			if err := validateNECOPriceRouteAddress(name, "pair", hop.Pair); err != nil {
				return err
			}
			if err := validateNECOPriceRouteAddress(name, "fromToken", hop.FromToken); err != nil {
				return err
			}
			if err := validateNECOPriceRouteAddress(name, "toToken", hop.ToToken); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNECOPriceRouteAddress(routeName, field, address string) error {
	if !common.IsHexAddress(address) {
		return errors.New("necoPriceOracle priceRoutes " + routeName + " hop " + field + " is invalid address")
	}
	return nil
}

func priceRouteDisplayName(route PriceRouteConfig) string {
	if strings.TrimSpace(route.Name) != "" {
		return strings.TrimSpace(route.Name)
	}
	if normalizeAsset(route.Asset) != "" {
		return normalizeAsset(route.Asset)
	}
	return "<unnamed>"
}

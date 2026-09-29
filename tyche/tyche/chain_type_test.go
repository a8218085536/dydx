package tyche

import (
	"errors"
	"testing"
)

func TestConfigDefaultsToEVMChainType(t *testing.T) {
	c := minimalConfig()

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if c.ChainType != ChainTypeEVM {
		t.Fatalf("ChainType = %q, want %q", c.ChainType, ChainTypeEVM)
	}
}

func TestDYDXConfigDoesNotRequireEVMAccount(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = ""
	c.PrivateKey = ""
	c.Contracts = nil
	c.EstimatorJS = ""
	configureValidDYDXTrading(c)

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
}

func TestDYDXConfigAllowsCosmosTradingAdapterSettings(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = "dydx1exampleaccount"
	c.PrivateKey = "dydx-private-key-placeholder"
	c.EstimatorJS = ""
	configureValidDYDXTrading(c)

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if c.DYDXMarkets[0].ClobPairID != 7 {
		t.Fatalf("market config = %+v", c.DYDXMarkets[0])
	}
	if c.DYDXRetainedCollateralUSD != defaultDYDXRetainedCollateralUSD {
		t.Fatalf("dydx retained collateral = %q, want %q", c.DYDXRetainedCollateralUSD, defaultDYDXRetainedCollateralUSD)
	}
}

func TestDYDXConfigRejectsInvalidRetainedCollateral(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = ""
	c.PrivateKey = ""
	c.Contracts = nil
	c.EstimatorJS = ""
	configureValidDYDXTrading(c)
	c.DYDXRetainedCollateralUSD = "-1"

	err := c.Init()

	if err == nil || err.Error() != "dydxRetainedCollateralUsd is invalid" {
		t.Fatalf("Init() error = %v, want retained collateral validation error", err)
	}
}

func TestDYDXConfigRequiresCosmosTradingAdapterSettingsTogether(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = "dydx1exampleaccount"
	c.PrivateKey = "dydx-private-key-placeholder"
	c.EstimatorJS = ""
	c.DYDXTradingURL = "http://127.0.0.1:8090/dydx/trading"
	c.InternalAPIKey = "12345678901234567890123456789012"

	err := c.Init()

	if !errors.Is(err, ErrDYDXChainIDNotConfigured) {
		t.Fatalf("Init() error = %v, want %v", err, ErrDYDXChainIDNotConfigured)
	}
}

func TestDYDXConfigRequiresTradingSidecarURL(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = ""
	c.PrivateKey = ""
	c.Contracts = nil
	c.EstimatorJS = ""
	c.InternalAPIKey = "12345678901234567890123456789012"

	err := c.Init()

	if !errors.Is(err, ErrDYDXTradingURLNotConfigured) {
		t.Fatalf("Init() error = %v, want %v", err, ErrDYDXTradingURLNotConfigured)
	}
}

func TestDYDXConfigRequiresStrongInternalAPIKey(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = ""
	c.PrivateKey = ""
	c.Contracts = nil
	c.EstimatorJS = ""
	configureValidDYDXTrading(c)
	c.InternalAPIKey = "too-short"

	err := c.Init()

	if !errors.Is(err, ErrInternalAPIKeyNotConfigured) {
		t.Fatalf("Init() error = %v, want %v", err, ErrInternalAPIKeyNotConfigured)
	}
}

func TestDYDXConfigRejectsStaticAddressProvider(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = ""
	c.PrivateKey = ""
	c.Contracts = nil
	c.EstimatorJS = ""
	configureValidDYDXTrading(c)
	c.DYDXAutoAddressPrefix = "dydx1mock"

	err := c.Init()

	if !errors.Is(err, ErrDYDXStaticAddressProviderForbidden) {
		t.Fatalf("Init() error = %v, want %v", err, ErrDYDXStaticAddressProviderForbidden)
	}
}

func TestDYDXConfigRejectsLegacyBroadcastURLs(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = ""
	c.PrivateKey = ""
	c.Contracts = nil
	c.EstimatorJS = ""
	configureValidDYDXTrading(c)
	c.DYDXBroadcastURL = "http://127.0.0.1:8081/dydx/broadcast"

	err := c.Init()

	if !errors.Is(err, ErrDYDXLegacyBroadcastForbidden) {
		t.Fatalf("Init() error = %v, want %v", err, ErrDYDXLegacyBroadcastForbidden)
	}
}

func TestConfigDecodesNECOHotWalletPrivateKey(t *testing.T) {
	c := minimalConfig()
	c.NECOHotWalletPrivateKey = "0000000000000000000000000000000000000000000000000000000000000001"

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if len(c.NECOWallet.HotWalletKey) != 32 {
		t.Fatalf("HotWalletKey length = %d, want 32", len(c.NECOWallet.HotWalletKey))
	}
}

func TestConfigDerivesNECOWalletTokenContractsFromAssetMappings(t *testing.T) {
	c := minimalConfig()
	c.AssetMappings = []AssetMapping{{
		NECOTokenAddress: "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		NECOAsset:        "dedx",
		DYDXAsset:        "USD",
		Decimals:         18,
	}}

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if c.NECOWallet.TokenContracts["DEDX"] != "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E" {
		t.Fatalf("DEDX token contract = %q", c.NECOWallet.TokenContracts["DEDX"])
	}
}

func TestConfigDoesNotOverrideExplicitNECOWalletTokenContracts(t *testing.T) {
	c := minimalConfig()
	c.AssetMappings = []AssetMapping{{
		NECOTokenAddress: "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
		NECOAsset:        "DEDX",
		DYDXAsset:        "USD",
		Decimals:         18,
	}}
	c.NECOWallet.TokenContracts = map[string]string{
		"DEDX": "0x0000000000000000000000000000000000000ded",
	}

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if c.NECOWallet.TokenContracts["DEDX"] != "0x0000000000000000000000000000000000000ded" {
		t.Fatalf("DEDX token contract = %q", c.NECOWallet.TokenContracts["DEDX"])
	}
}

func TestConfigNormalizesExplicitNECOWalletTokenContractAssets(t *testing.T) {
	c := minimalConfig()
	c.NECOWallet.TokenContracts = map[string]string{
		"dedx": "0x0000000000000000000000000000000000000ded",
	}

	if err := c.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if c.NECOWallet.TokenContracts["DEDX"] != "0x0000000000000000000000000000000000000ded" {
		t.Fatalf("DEDX token contract = %q", c.NECOWallet.TokenContracts["DEDX"])
	}
}

func minimalConfig() *Config {
	return &Config{
		Account:                   "0x0000000000000000000000000000000000000001",
		PrivateKey:                "0000000000000000000000000000000000000000000000000000000000000001",
		EstimatorJS:               "./js/gasprice.js",
		ExpectedMD5Password:       "password-md5",
		AllowedIP:                 "127.0.0.1",
		ErrorNumberOfConcurrent:   1,
		FailedNumberOfConcurrent:  1,
		PendingNumberOfConcurrent: 1,
		SentNumberOfConcurrent:    1,
		SucceedNumberOfConcurrent: 1,
	}
}

func configureValidDYDXTrading(c *Config) {
	c.DYDXTradingURL = "http://127.0.0.1:8090/dydx/trading"
	c.DYDXChainID = "dydx-local-1"
	c.DYDXHeightURL = "http://127.0.0.1:26657/status"
	c.DYDXGoodTilSeconds = 3600
	c.DYDXMarkets = []DYDXMarketConfig{{Market: "ETH-USD", ClobPairID: 7, QuantumsPerUnit: "1000000", SubticksPerUnit: "1000000"}}
	c.InternalAPIKey = "12345678901234567890123456789012"
}

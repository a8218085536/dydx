package app

import (
	"context"
	"errors"
	"ethgo/model"
	"ethgo/tyche"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestConfigInitAllowsDYDXWithNECORPCWithoutBackend(t *testing.T) {
	conf := &Config{
		Redis: &model.Config{Domain: "test", URI: "redis://127.0.0.1:6379"},
		Tyche: appTestConfig(tyche.ChainTypeDYDX),
	}
	conf.Tyche.NECORPCURL = "http://127.0.0.1:8545"

	if err := conf.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if conf.Redis.Namespace == "" {
		t.Fatal("Redis namespace is empty")
	}
}

func TestNewNECOBackendConfigUsesDedicatedRPCWithoutBackend(t *testing.T) {
	conf := appTestConfig(tyche.ChainTypeDYDX)
	conf.NECORPCURL = "http://127.0.0.1:8545"
	app := &App{conf: &Config{Tyche: conf}}

	backendConf, err := app.newNECOBackendConfig()

	if err != nil {
		t.Fatalf("newNECOBackendConfig() error = %v", err)
	}
	if backendConf == nil {
		t.Fatal("backend config is nil")
	}
	if backendConf.Addr != conf.NECORPCURL {
		t.Fatalf("backend addr = %q, want %q", backendConf.Addr, conf.NECORPCURL)
	}
	if backendConf.TransactionsPerSecond < 1 {
		t.Fatalf("transactions per second = %d, want at least 1", backendConf.TransactionsPerSecond)
	}
}

func TestAppNewNECOWithdrawalWorkerUsesNECOBackendInDYDXMode(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	app := &App{
		ledger:      tyche.NewLedgerService(tyche.NewMemoryLedgerStore()),
		necoBackend: &configTestNECOLogBackend{chainID: big.NewInt(7260)},
		conf: &Config{Tyche: &tyche.Config{
			NECOWallet: tyche.NECOWalletConfig{
				HotWalletAddress: hotWallet.Hex(),
				HotWalletKey:     crypto.FromECDSA(key),
				TokenContracts: map[string]string{
					"DEDX": "0x7C5E44Ed1E16F92aA6363F3F686da9dC9507984E",
				},
			},
		}},
	}

	if worker := app.newNECOWithdrawalWorker(); worker == nil {
		t.Fatalf("newNECOWithdrawalWorker() returned nil")
	}
}

func TestNewNECODepositSyncWorkerUsesAssetMappingTokens(t *testing.T) {
	conf := appTestConfig(tyche.ChainTypeDYDX)
	conf.NECODeposit = tyche.NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		StartBlock:       90,
	}
	conf.AssetMappings = []tyche.AssetMapping{
		{
			NECOTokenAddress:  "0x0000000000000000000000000000000000000ded",
			NECOAsset:         "DEDX",
			DYDXAsset:         "DEDX",
			EnabledForTrading: true,
		},
	}
	ledgerStore := tyche.NewMemoryLedgerStore()
	authStore := tyche.NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	app := &App{
		conf: &Config{Tyche: conf},
		necoBackend: &configTestNECOLogBackend{
			chainID:     big.NewInt(9001),
			blockNumber: 120,
			logs: []types.Log{
				configTestERC20TransferLog(
					"0x0000000000000000000000000000000000000ded",
					"0x0000000000000000000000000000000000000001",
					"0x0000000000000000000000000000000000000abc",
					big.NewInt(100),
					100,
					3,
					"0xtx1",
				),
			},
		},
		ledger: tyche.NewLedgerService(ledgerStore),
	}
	app.necoDeposits = tyche.NewNECODepositService(tyche.NewMemoryNECODepositStore(), app.ledger, authStore, tyche.NECODepositConfig{
		DepositAddress:   conf.NECODeposit.DepositAddress,
		MinConfirmations: conf.NECODeposit.MinConfirmations,
		AssetMappings:    conf.AssetMappings,
	})
	app.necoDepositSync = app.newNECODepositSyncWorker()

	deposits, err := app.necoDepositSync.PollOnce(context.Background())

	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(deposits) != 1 {
		t.Fatalf("len(deposits) = %d, want 1", len(deposits))
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}
	backend := app.necoBackend.(*configTestNECOLogBackend)
	if len(backend.query.Addresses) != 1 || backend.query.Addresses[0] != common.HexToAddress("0x0000000000000000000000000000000000000ded") {
		t.Fatalf("query addresses = %+v", backend.query.Addresses)
	}
	if backend.query.FromBlock.Uint64() != 90 {
		t.Fatalf("from block = %d, want 90", backend.query.FromBlock.Uint64())
	}
}

func TestNewTradingSyncWorkerRequiresEventSourceURL(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	app := &App{
		conf:        &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		ledger:      ledger,
		tradingSync: tyche.NewTradingSyncService(tyche.NewMemoryTradingSyncStore(), tyche.NewMemoryTradingStore(), ledger),
	}

	if worker := app.newTradingSyncWorker(tyche.NewMemoryTradingSyncCursorStore(tyche.TradingSyncCursor{})); worker != nil {
		t.Fatalf("newTradingSyncWorker() = %+v, want nil without event source URL", worker)
	}

	app.conf.Tyche.DYDXEventSourceURL = "http://127.0.0.1:8080"
	if worker := app.newTradingSyncWorker(tyche.NewMemoryTradingSyncCursorStore(tyche.TradingSyncCursor{})); worker == nil {
		t.Fatal("newTradingSyncWorker() = nil, want worker")
	}
}

func TestNewDYDXMarketPriceSourceWrapsNECOSwapOracleWhenConfigured(t *testing.T) {
	conf := appTestConfig(tyche.ChainTypeDYDX)
	conf.DYDXMarketPriceURL = "http://127.0.0.1:8090/prices"
	conf.DYDXMarkets = []tyche.DYDXMarketConfig{
		{Market: "FDEX-USD", ClobPairID: 302, QuantumsPerUnit: "1000000", StepBaseQuantums: "1000000", SubticksPerUnit: "1000000000", SubticksPerTick: "1000000"},
		{Market: "ETH-USD", ClobPairID: 1, QuantumsPerUnit: "1000000000", StepBaseQuantums: "1000000", SubticksPerUnit: "1000000", SubticksPerTick: "100000"},
	}
	conf.AssetMappings = []tyche.AssetMapping{{
		NECOAsset:         "FDEX",
		DYDXAsset:         "USD",
		PriceRoute:        "FDEX-USDT",
		EnabledForTrading: true,
	}}
	conf.PriceRoutes = []tyche.PriceRouteConfig{{
		Name:     "FDEX-USDT",
		Asset:    "FDEX",
		USDAsset: "USD",
		Hops: []tyche.PriceRouteHopConfig{{
			Pair:         "0x0000000000000000000000000000000000001001",
			FromToken:    "0x0000000000000000000000000000000000000fde",
			ToToken:      "0x0000000000000000000000000000000000000ddd",
			FromDecimals: 18,
			ToDecimals:   18,
		}},
	}}
	conf.NECOPriceOracle = tyche.NECOPriceOracleConfig{
		Enabled:     true,
		Mode:        tyche.NECOPriceOracleModeV2Pair,
		USDToken:    "0x0000000000000000000000000000000000000ddd",
		USDDecimals: 18,
	}
	app := &App{conf: &Config{Tyche: conf}, necoBackend: &configTestNECOLogBackend{chainID: big.NewInt(7260)}}

	source := app.newDYDXMarketPriceSource(app.newAssetUSDOracle())

	if _, ok := source.(*tyche.NECOAssetMarketPriceSource); !ok {
		t.Fatalf("market price source = %T, want NECO asset market price wrapper", source)
	}
}

func TestDeriveDYDXWalletCreateURLFromTradingURL(t *testing.T) {
	got := deriveDYDXWalletCreateURL("http://127.0.0.1:8090/dydx/trading")

	if got != "http://127.0.0.1:8090/wallet/create" {
		t.Fatalf("deriveDYDXWalletCreateURL() = %q", got)
	}
}

func TestDYDXExampleConfigParsesCosmosTradingSettings(t *testing.T) {
	path := filepath.Join("..", "tyche.dydx.example.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	temp := filepath.Join(t.TempDir(), "tyche.dydx.example.toml")
	if err := os.WriteFile(temp, raw, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	conf, err := NewConfig(temp)

	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if conf.Tyche.DYDXTradingURL == "" || conf.Tyche.DYDXChainID != "flydex-mainnet-1" {
		t.Fatalf("dydx trading config = %+v", conf.Tyche)
	}
	if conf.Tyche.DYDXRetainedCollateralUSD != "1" {
		t.Fatalf("dydx retained collateral = %q, want 1", conf.Tyche.DYDXRetainedCollateralUSD)
	}
	if conf.Tyche.DYDXEventSourceURL == "" || conf.Tyche.DYDXEventSourceAPIKey == "" {
		t.Fatalf("dydx event source config = %+v", conf.Tyche)
	}
	if len(conf.Tyche.DYDXMarkets) != 33 {
		t.Fatalf("dydx markets = %+v", conf.Tyche.DYDXMarkets)
	}
	ethMarket, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "ETH-USD")
	if !ok || ethMarket.ClobPairID != 1 || ethMarket.QuantumsPerUnit != "1000000000" || ethMarket.StepBaseQuantums != "1000000" || ethMarket.SubticksPerUnit != "1000000" || ethMarket.SubticksPerTick != "100000" {
		t.Fatalf("ETH-USD market = %+v", ethMarket)
	}
	pepeMarket, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "PEPE-USD")
	if !ok || pepeMarket.ClobPairID != 28 || pepeMarket.QuantumsPerUnit != "0.1" || pepeMarket.StepBaseQuantums != "1000000" || pepeMarket.SubticksPerUnit != "10000000000000000" || pepeMarket.SubticksPerTick != "1000000" {
		t.Fatalf("PEPE-USD market = %+v", pepeMarket)
	}
	if _, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "FDEX-USD"); ok {
		t.Fatal("FDEX-USD must not be exposed as a Tyche tradable market until it has a real dYdX price/CLOB")
	}
	if conf.Tyche.NECOPriceOracle.Enabled {
		t.Fatal("neco price oracle must stay disabled until real NECO pair addresses are configured")
	}
	if conf.Tyche.NECOPriceOracle.Mode != tyche.NECOPriceOracleModeV2Pair {
		t.Fatalf("neco price oracle mode = %q", conf.Tyche.NECOPriceOracle.Mode)
	}
	if conf.Tyche.NECODeposit.DepositAddress != "" {
		t.Fatalf("neco deposit address = %q, want empty until production address is configured", conf.Tyche.NECODeposit.DepositAddress)
	}
	if len(conf.Tyche.AssetMappings) != 2 {
		t.Fatalf("asset mappings = %+v, want disabled placeholders for DEDX and USDT", conf.Tyche.AssetMappings)
	}
	for _, mapping := range conf.Tyche.AssetMappings {
		if mapping.EnabledForTrading {
			t.Fatalf("example mapping = %+v, want trading disabled until real NECO token addresses are configured", mapping)
		}
	}
	if len(conf.Tyche.PriceRoutes) != 2 {
		t.Fatalf("price routes = %+v, want placeholder DEDX and USDT routes", conf.Tyche.PriceRoutes)
	}
}

func TestMockDeployConfigParsesRunnableMockSettings(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "tyche.mock.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	temp := filepath.Join(t.TempDir(), "tyche.mock.toml")
	if err := os.WriteFile(temp, raw, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	conf, err := NewConfig(temp)

	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if conf.Redis.Domain != "flydex:mock" {
		t.Fatalf("redis domain = %q, want flydex:mock", conf.Redis.Domain)
	}
	if conf.Tyche.InternalAPIKey == "" || conf.Tyche.ExpectedMD5Password == "" {
		t.Fatalf("mock config did not parse tyche secrets into tyche section: %+v", conf.Tyche)
	}
	if conf.Tyche.DYDXTradingURL != "http://127.0.0.1:8090/dydx/trading" {
		t.Fatalf("dydx trading URL = %q", conf.Tyche.DYDXTradingURL)
	}
	if conf.Tyche.DYDXRetainedCollateralUSD != "1" {
		t.Fatalf("mock retained collateral = %q, want 1", conf.Tyche.DYDXRetainedCollateralUSD)
	}
	if conf.Tyche.DYDXHeightURL != "http://127.0.0.1:8090/status" {
		t.Fatalf("dydx height URL = %q", conf.Tyche.DYDXHeightURL)
	}
	if len(conf.Tyche.DYDXMarkets) != 33 {
		t.Fatalf("dydx markets = %+v", conf.Tyche.DYDXMarkets)
	}
	market, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "XRP-USD")
	if !ok || market.ClobPairID != 32 || market.QuantumsPerUnit != "100000" || market.SubticksPerUnit != "10000000000" {
		t.Fatalf("XRP-USD market = %+v", market)
	}
}

func TestSingleNodeDeployConfigParsesRunnableDYDXSettings(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "tyche.dydx.single-node.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	raw = []byte(strings.ReplaceAll(string(raw), "CHANGE_ME_FDEX_TOKEN_ADDRESS", "0x1000000000000000000000000000000000000001"))
	raw = []byte(strings.ReplaceAll(string(raw), "CHANGE_ME_USDT_TOKEN_ADDRESS", "0x1000000000000000000000000000000000000002"))
	raw = []byte(strings.ReplaceAll(string(raw), "CHANGE_ME_FDEX_USDT_PAIR_ADDRESS", "0x2000000000000000000000000000000000000001"))
	temp := filepath.Join(t.TempDir(), "tyche.dydx.single-node.toml")
	if err := os.WriteFile(temp, raw, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	conf, err := NewConfig(temp)

	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if conf.Tyche.Listen != "127.0.0.1:8081" {
		t.Fatalf("listen = %q", conf.Tyche.Listen)
	}
	if conf.Tyche.DYDXChainID != "flydex-mainnet-1" || conf.Tyche.DYDXHeightURL != "http://127.0.0.1:26657/status" {
		t.Fatalf("dydx chain config = %+v", conf.Tyche)
	}
	if conf.Tyche.DYDXRetainedCollateralUSD != "1" {
		t.Fatalf("single-node retained collateral = %q, want 1", conf.Tyche.DYDXRetainedCollateralUSD)
	}
	if conf.Tyche.DYDXSignatureVerifyURL != "" {
		t.Fatalf("dydx signature verifier = %q, want empty", conf.Tyche.DYDXSignatureVerifyURL)
	}
	if len(conf.Tyche.DYDXMarkets) != 34 {
		t.Fatalf("dydx markets = %+v", conf.Tyche.DYDXMarkets)
	}
	dedxMarket, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "FDEX-USD")
	if !ok || dedxMarket.ClobPairID != 302 || dedxMarket.QuantumsPerUnit != "1000000" || dedxMarket.StepBaseQuantums != "1000000" || dedxMarket.SubticksPerUnit != "1000000000" || dedxMarket.SubticksPerTick != "1000000" {
		t.Fatalf("FDEX-USD market = %+v", dedxMarket)
	}
	if _, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "EIGEN-USD"); ok {
		t.Fatal("EIGEN-USD must not be exposed until the running chain has a matching CLOB pair")
	}
	if _, ok := findConfigTestDYDXMarket(conf.Tyche.DYDXMarkets, "BOME-USD"); ok {
		t.Fatal("BOME-USD must not be exposed until the running chain has a matching CLOB pair")
	}
	if conf.Tyche.NECORPCURL != "https://chainrpc.necochain.com/" {
		t.Fatalf("necoRpcURL = %q", conf.Tyche.NECORPCURL)
	}
	if conf.Tyche.ExpectedMD5Password != "3c2d35df9139402045fcf0eac90a6938" {
		t.Fatal("single-node admin password hash is not configured")
	}
	if conf.Tyche.NECODeposit.DepositAddress != "0xa3e3b7eFFDEeba1239991a8329D45720F62D3bce" || conf.Tyche.NECODeposit.StartBlock != 23521368 {
		t.Fatalf("neco deposit config = %+v", conf.Tyche.NECODeposit)
	}
	if conf.Tyche.NECODeposit.MaxBlockRange != 512 {
		t.Fatalf("neco deposit maxBlockRange = %d, want 512", conf.Tyche.NECODeposit.MaxBlockRange)
	}
	if len(conf.Tyche.AssetMappings) != 2 {
		t.Fatalf("asset mappings = %+v", conf.Tyche.AssetMappings)
	}
	mapping := conf.Tyche.AssetMappings[0]
	if mapping.NECOTokenAddress != "0x1000000000000000000000000000000000000001" || mapping.NECOAsset != "FDEX" || mapping.DYDXAsset != "USD" || mapping.Decimals != 18 || mapping.PriceRoute != "FDEX-USDT" || !mapping.EnabledForTrading {
		t.Fatalf("FDEX asset mapping = %+v", mapping)
	}
	usdtMapping := conf.Tyche.AssetMappings[1]
	if usdtMapping.NECOTokenAddress != "0x1000000000000000000000000000000000000002" || usdtMapping.NECOAsset != "USDT" || usdtMapping.DYDXAsset != "USD" || usdtMapping.Decimals != 18 || usdtMapping.PriceRoute != "USDT-USD" || !usdtMapping.EnabledForTrading {
		t.Fatalf("USDT asset mapping = %+v", usdtMapping)
	}
	if len(conf.Tyche.PriceRoutes) != 2 {
		t.Fatalf("price routes = %+v, want FDEX and USDT routes", conf.Tyche.PriceRoutes)
	}
	fdexRoute, ok := findConfigTestPriceRoute(conf.Tyche.PriceRoutes, "FDEX-USDT")
	if !ok || fdexRoute.Asset != "FDEX" || fdexRoute.USDAsset != "USD" || len(fdexRoute.Hops) != 1 {
		t.Fatalf("FDEX price route = %+v", fdexRoute)
	}
	usdtRoute, ok := findConfigTestPriceRoute(conf.Tyche.PriceRoutes, "USDT-USD")
	if !ok || usdtRoute.Asset != "USDT" || usdtRoute.USDAsset != "USD" || len(usdtRoute.Hops) != 0 {
		t.Fatalf("USDT price route = %+v", usdtRoute)
	}
	oracle := conf.Tyche.NECOPriceOracle
	if !oracle.Enabled || oracle.DEDXNECOPair != "" || oracle.NECOUSDPair != "" {
		t.Fatalf("neco price oracle pairs = %+v", oracle)
	}
	if oracle.DEDXDecimals != 18 || oracle.NECODecimals != 18 || oracle.USDDecimals != 18 {
		t.Fatalf("neco price oracle decimals = %+v", oracle)
	}
	if oracle.DEDXToken != "" || oracle.NECOToken != "" || oracle.USDToken != "" {
		t.Fatalf("neco price oracle tokens = %+v", oracle)
	}
	if conf.Tyche.NECOWallet.ChainID != 7260 {
		t.Fatalf("neco wallet chain id = %d", conf.Tyche.NECOWallet.ChainID)
	}
}

func findConfigTestDYDXMarket(markets []tyche.DYDXMarketConfig, market string) (tyche.DYDXMarketConfig, bool) {
	for _, candidate := range markets {
		if candidate.Market == market {
			return candidate, true
		}
	}
	return tyche.DYDXMarketConfig{}, false
}

func findConfigTestPriceRoute(routes []tyche.PriceRouteConfig, name string) (tyche.PriceRouteConfig, bool) {
	for _, route := range routes {
		if route.Name == name {
			return route, true
		}
	}
	return tyche.PriceRouteConfig{}, false
}

type configTestNECOLogBackend struct {
	chainID     *big.Int
	blockNumber uint64
	logs        []types.Log
	query       ethereum.FilterQuery
}

func (b *configTestNECOLogBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return b.chainID, nil
}

func (b *configTestNECOLogBackend) BlockNumber(ctx context.Context) (uint64, error) {
	return b.blockNumber, nil
}

func (b *configTestNECOLogBackend) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	b.query = q
	return b.logs, nil
}

func (b *configTestNECOLogBackend) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	return nil, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return nil, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) Close() {}

func (b *configTestNECOLogBackend) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) TransactionCount(ctx context.Context, account common.Hash) (uint, error) {
	return 0, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) SendRawTransaction(ctx context.Context, signedTxData string) error {
	return errors.New("not implemented")
}

func (b *configTestNECOLogBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return errors.New("not implemented")
}

func (b *configTestNECOLogBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) SuggestGasLimit(ctx context.Context) uint64 {
	return 0
}

func (b *configTestNECOLogBackend) TransactionsPerSecond(ctx context.Context) int64 {
	return 1
}

func (b *configTestNECOLogBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	return nil, false, errors.New("not implemented")
}

func (b *configTestNECOLogBackend) WaitMined(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	return nil, errors.New("not implemented")
}

func configTestERC20TransferLog(token, from, to string, amount *big.Int, blockNumber uint64, logIndex uint, txHash string) types.Log {
	return types.Log{
		Address:     common.HexToAddress(token),
		Topics:      []common.Hash{tyche.ERC20TransferTopic(), common.HexToHash(from), common.HexToHash(to)},
		Data:        common.LeftPadBytes(amount.Bytes(), 32),
		BlockNumber: blockNumber,
		Index:       logIndex,
		TxHash:      common.HexToHash(txHash),
	}
}

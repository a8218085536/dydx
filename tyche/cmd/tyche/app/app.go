package app

import (
	"context"
	"ethgo/eth"
	"ethgo/model"
	modelauth "ethgo/model/auth"
	"ethgo/model/ledger"
	"ethgo/model/necodeposits"
	"ethgo/model/necowithdrawals"
	modeltrading "ethgo/model/trading"
	"ethgo/model/walletbindings"
	"ethgo/tyche"
	"ethgo/util/ginx"
	"fmt"
	"net/http"
	"net/url"
	"time"

	_ "ethgo/cmd/tyche/docs"

	"github.com/gin-gonic/gin"
)

type App struct {
	backend                 eth.Backend
	necoBackend             eth.Backend
	base                    tyche.Runner
	chainGateway            *tyche.DYDXChainGateway
	bindingStore            tyche.WalletBindingStore
	walletBindings          *tyche.WalletBindingService
	ledger                  *tyche.LedgerService
	trading                 *tyche.TradingService
	tradingBalances         *tyche.TradingBalanceService
	tradingAccountSummary   *tyche.TradingAccountSummaryService
	tradingAccountValuation *tyche.TradingAccountValuationService
	tradingSync             *tyche.TradingSyncService
	tradingSyncRun          *tyche.TradingSyncWorker
	tradingReconcileRun     *tyche.TradingReconcileWorker
	tradingSettlements      tyche.TradingSettlementStore
	tradingTPSL             *tyche.TradingTPSLService
	tradingTPSLRun          *tyche.TradingTPSLWorker
	closeSettlement         *tyche.CloseSettlementService
	necoDeposits            *tyche.NECODepositService
	necoDepositSync         *tyche.NECODepositSyncWorker
	necoDepositReceiptSync  *tyche.NECODepositReceiptSyncer
	necoWithdrawal          *tyche.NECOWithdrawalWorker
	necoPriceQuotes         *tyche.NECOPriceQuoteService
	auth                    *tyche.AuthService
	conf                    *Config
}

func New(conf *Config) App {
	return App{conf: conf}
}

func (app *App) Init(ctx context.Context) error {
	err := model.Init(app.conf.Redis)
	if err != nil {
		return err
	}

	if app.conf.Tyche.ChainType == tyche.ChainTypeEVM {
		if app.backend, err = eth.New(app.conf.Backend); err != nil {
			return err
		}
	}
	if app.conf.Tyche.NECORPCURL != "" {
		necoConf, err := app.newNECOBackendConfig()
		if err != nil {
			return err
		}
		if app.necoBackend, err = eth.New(necoConf); err != nil {
			return err
		}
	}

	if app.base, err = tyche.NewRunner(app.backend, app.conf.Tyche); err != nil {
		return err
	}
	app.chainGateway = tyche.NewDYDXChainGateway(app.newDYDXChainClient())
	app.bindingStore = walletbindings.NewRedisStore()
	app.walletBindings = tyche.NewWalletBindingService(app.bindingStore, app.newWalletBindingVerifier())
	app.ledger = app.newLedgerService()
	app.tradingBalances = app.newTradingBalanceService()
	app.necoDeposits = app.newNECODepositService()
	app.necoDepositSync = app.newNECODepositSyncWorker()
	app.necoDepositReceiptSync = app.newNECODepositReceiptSyncer()
	app.necoPriceQuotes = app.newNECOPriceQuoteService()
	tradingStore := modeltrading.NewRedisStore()
	tradingSyncStore := modeltrading.NewRedisSyncStore()
	app.tradingSettlements = modeltrading.NewRedisSettlementStore()
	app.trading = app.newTradingService(tradingStore)
	if app.trading != nil {
		app.trading.SetDYDXRetainedCollateralStore(modeltrading.NewRedisRetainedCollateralStore())
	}
	app.tradingSync = tyche.NewTradingSyncService(tradingSyncStore, tradingStore, app.ledger)
	if app.tradingSync != nil {
		app.tradingSync.SetWalletBindingLookup(app.walletBindings)
	}
	if app.trading != nil {
		app.trading.SetPositionProvider(app.tradingSync)
	}
	app.tradingAccountSummary = app.newTradingAccountSummaryService()
	app.tradingAccountValuation = app.newTradingAccountValuationService()
	app.tradingTPSL = app.newTradingTPSLService(modeltrading.NewRedisTPSLStore())
	app.tradingSyncRun = app.newTradingSyncWorker(tradingSyncStore)
	app.tradingReconcileRun = app.newTradingReconcileWorker(tradingStore)
	app.tradingTPSLRun = app.newTradingTPSLWorker()
	app.auth = app.newAuthService()
	if app.necoDeposits != nil {
		app.auth.SetDepositClaimer(app.necoDeposits)
	}
	app.necoWithdrawal = app.newNECOWithdrawalWorker()
	app.closeSettlement = app.newCloseSettlementService()
	if app.necoWithdrawal != nil && app.tradingSettlements != nil {
		app.necoWithdrawal.SetSettlementLookup(app.tradingSettlements)
	}
	if app.trading != nil {
		app.trading.SetFailedOrderSettlementService(app.closeSettlement)
	}
	if app.tradingSync != nil {
		app.tradingSync.SetOrderSettlementService(app.closeSettlement)
	}
	if app.tradingReconcileRun != nil {
		app.tradingReconcileRun.SetCloseSettlementService(app.closeSettlement)
	}
	return app.base.Init(ctx)
}

func (app *App) newAuthService() *tyche.AuthService {
	auth := tyche.NewAuthService(modelauth.NewRedisStore(), tyche.EVMWalletBindingVerifier{}, app.walletBindings)
	if app.bindingStore != nil && app.conf != nil && app.conf.Tyche != nil {
		if provider := app.newDYDXAddressProvider(); provider != nil {
			auth.SetDYDXBindingProvisioner(tyche.NewWalletBindingProvisioner(app.bindingStore, provider))
		}
	}
	return auth
}

func (app *App) newDYDXAddressProvider() tyche.DYDXAddressProvider {
	if app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	walletURL := app.conf.Tyche.DYDXWalletCreateURL
	if walletURL == "" {
		walletURL = deriveDYDXWalletCreateURL(app.conf.Tyche.DYDXTradingURL)
	}
	if walletURL != "" {
		return tyche.NewDYDXProxyWalletHTTPProvider(tyche.DYDXProxyWalletHTTPProviderConfig{
			URL:         walletURL,
			BearerToken: app.conf.Tyche.InternalAPIKey,
		})
	}
	return nil
}

func deriveDYDXWalletCreateURL(tradingURL string) string {
	return deriveDYDXSidecarURL(tradingURL, "/wallet/create")
}

func deriveDYDXSubaccountDepositURL(tradingURL string) string {
	return deriveDYDXSidecarURL(tradingURL, "/subaccount/deposit")
}

func deriveDYDXSidecarURL(tradingURL, path string) string {
	if tradingURL == "" {
		return ""
	}
	parsed, err := url.Parse(tradingURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (app *App) newLedgerService() *tyche.LedgerService {
	return tyche.NewLedgerService(ledger.NewRedisStore())
}

func (app *App) newNECODepositService() *tyche.NECODepositService {
	if app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	conf := app.conf.Tyche.NECODeposit
	conf.AssetMappings = app.conf.Tyche.AssetMappings
	return tyche.NewNECODepositService(necodeposits.NewRedisStore(), app.ledger, modelauth.NewRedisStore(), conf)
}

func (app *App) newNECOBackendConfig() (*eth.Config, error) {
	if app.conf == nil || app.conf.Tyche == nil || app.conf.Tyche.NECORPCURL == "" {
		return nil, nil
	}
	necoConf := &eth.Config{
		Addr:                  app.conf.Tyche.NECORPCURL,
		TransactionsPerSecond: 1,
	}
	if app.conf.Backend != nil {
		cloned := *app.conf.Backend
		cloned.Addr = app.conf.Tyche.NECORPCURL
		necoConf = &cloned
	}
	if err := necoConf.Init(); err != nil {
		return nil, err
	}
	return necoConf, nil
}

func (app *App) newNECODepositSyncWorker() *tyche.NECODepositSyncWorker {
	if app.necoBackend == nil || app.necoDeposits == nil || app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	conf := tyche.NECODepositWatcherConfig{
		DepositAddress:   app.conf.Tyche.NECODeposit.DepositAddress,
		MinConfirmations: app.conf.Tyche.NECODeposit.MinConfirmations,
		MaxBlockRange:    app.conf.Tyche.NECODeposit.MaxBlockRange,
	}
	conf.TokenAddresses = app.newNECODepositTokenAddresses()
	cursor := tyche.NECOBlockCursor(tyche.NewMemoryNECOBlockCursor(app.conf.Tyche.NECODeposit.StartBlock))
	if model.RedisPool != nil {
		cursor = necodeposits.NewRedisCursorStore(
			fmt.Sprintf("%s:neco:deposit:cursor", model.RedisPool.Namespace),
			app.conf.Tyche.NECODeposit.StartBlock,
		)
	}
	return tyche.NewNECODepositSyncWorker(app.necoBackend, app.necoDeposits, cursor, conf)
}

func (app *App) newNECODepositReceiptSyncer() *tyche.NECODepositReceiptSyncer {
	if app.necoBackend == nil || app.necoDeposits == nil || app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	conf := tyche.NECODepositWatcherConfig{
		DepositAddress:   app.conf.Tyche.NECODeposit.DepositAddress,
		TokenAddresses:   app.newNECODepositTokenAddresses(),
		MinConfirmations: app.conf.Tyche.NECODeposit.MinConfirmations,
		MaxBlockRange:    app.conf.Tyche.NECODeposit.MaxBlockRange,
	}
	return tyche.NewNECODepositReceiptSyncer(app.necoBackend, app.necoDeposits, conf)
}

func (app *App) newNECODepositTokenAddresses() []string {
	if app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	var tokenAddresses []string
	for _, token := range app.conf.Tyche.NECODeposit.Tokens {
		tokenAddresses = append(tokenAddresses, token.Address)
	}
	for _, mapping := range app.conf.Tyche.AssetMappings {
		if !mapping.EnabledForTrading {
			continue
		}
		if mapping.NECOTokenAddress != "" {
			tokenAddresses = append(tokenAddresses, mapping.NECOTokenAddress)
		}
	}
	return tokenAddresses
}

func (app *App) newNECOWithdrawalWorker() *tyche.NECOWithdrawalWorker {
	if app.necoBackend == nil || app.conf == nil || app.conf.Tyche == nil || len(app.conf.Tyche.NECOWallet.HotWalletKey) == 0 {
		return nil
	}
	worker, err := tyche.NewNECOWithdrawalWorker(app.ledger, necowithdrawals.NewRedisStore(), app.necoBackend, app.conf.Tyche.NECOWallet)
	if err != nil {
		return nil
	}
	return worker
}

func (app *App) newCloseSettlementService() *tyche.CloseSettlementService {
	if app.conf == nil || app.conf.Tyche == nil || app.tradingSettlements == nil || app.ledger == nil {
		return nil
	}
	service := tyche.NewCloseSettlementService(app.tradingSettlements, app.ledger, app.necoWithdrawal, app.newAssetUSDOracle(), app.walletBindings, tyche.CloseSettlementConfig{
		Mode:                     app.conf.Tyche.CloseSettlementMode,
		SettlementAsset:          app.conf.Tyche.CloseSettlementAsset,
		SettlementAssetDecimals:  app.conf.Tyche.CloseSettlementAssetDecimals,
		CollateralQuantumsPerUSD: app.conf.Tyche.DYDXCollateralQuantumsPerUSD,
		SubaccountNumber:         app.conf.Tyche.DYDXSubaccountNumber,
		Markets:                  app.conf.Tyche.DYDXMarkets,
		AssetMappings:            app.conf.Tyche.AssetMappings,
		TradeCloseCallbackURL:    app.conf.Tyche.TradeCloseCallbackURL,
	})
	if app.tradingSync != nil {
		service.SetFillLookup(app.tradingSync)
	}
	return service
}

func (app *App) newTradingService(store tyche.TradingStore) *tyche.TradingService {
	submitter, ok := app.base.(tyche.DYDXSubmitter)
	if !ok {
		submitter = nilDYDXSubmitter{}
	}
	service := tyche.NewTradingService(store, app.ledger, submitter)
	if app.walletBindings != nil {
		service.SetDYDXAccountResolver(app.walletBindings)
	}
	assetOracle := app.newAssetUSDOracle()
	if assetOracle != nil {
		service.SetMarginQuoter(tyche.NewAssetMarginQuoter(app.conf.Tyche.AssetMappings, assetOracle))
	}
	if source := app.newDYDXMarketPriceSource(assetOracle); source != nil {
		service.SetMarketPriceSource(source)
	}
	if app.conf != nil && app.conf.Tyche != nil {
		service.SetDYDXMarkets(app.conf.Tyche.DYDXMarkets)
		if app.conf.Tyche.DYDXEventSourceURL != "" {
			service.SetDYDXPositionReconcileSource(tyche.NewDYDXOrderReconcileHTTPSource(tyche.DYDXOrderReconcileHTTPSourceConfig{
				URL:    app.conf.Tyche.DYDXEventSourceURL,
				APIKey: app.conf.Tyche.DYDXEventSourceAPIKey,
			}))
		}
	}
	service.SetMarketOrderSlippageBps(app.conf.Tyche.MarketOrderSlippageBps)
	if preparer := app.newTradingFundingPreparer(); preparer != nil {
		service.SetFundingPreparer(preparer)
	}
	if depositor := app.newDYDXSubaccountDepositor(); depositor != nil {
		service.SetDYDXSubaccountDepositor(depositor)
		service.SetDYDXSubaccountFundingConfig(tyche.DYDXSubaccountFundingConfig{
			CollateralAssetID:         app.conf.Tyche.DYDXCollateralAssetID,
			CollateralQuantumsPerUSD:  app.conf.Tyche.DYDXCollateralQuantumsPerUSD,
			SubaccountNumber:          app.conf.Tyche.DYDXSubaccountNumber,
			MinStatefulOrderMarginUSD: app.conf.Tyche.DYDXMinStatefulOrderMarginUSD,
			RetainedCollateralUSD:     app.conf.Tyche.DYDXRetainedCollateralUSD,
		})
	}
	return service
}

func (app *App) newDYDXSubaccountDepositor() tyche.DYDXSubaccountDepositor {
	if app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	depositURL := app.conf.Tyche.DYDXSubaccountDepositURL
	if depositURL == "" {
		depositURL = deriveDYDXSubaccountDepositURL(app.conf.Tyche.DYDXTradingURL)
	}
	if depositURL == "" {
		return nil
	}
	return tyche.NewDYDXSubaccountDepositHTTPClient(tyche.DYDXSubaccountDepositHTTPClientConfig{
		URL:         depositURL,
		BearerToken: app.conf.Tyche.InternalAPIKey,
	})
}

func (app *App) newDYDXMarketPriceSource(assetOracle tyche.AssetUSDOracle) tyche.TradingMarketPriceSource {
	if app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	var fallback tyche.TradingMarketPriceSource
	if app.conf.Tyche.DYDXMarketPriceURL != "" {
		fallback = tyche.NewDYDXHTTPMarketPriceSource(tyche.DYDXHTTPMarketPriceSourceConfig{
			URL:     app.conf.Tyche.DYDXMarketPriceURL,
			APIKey:  app.conf.Tyche.InternalAPIKey,
			Markets: app.conf.Tyche.DYDXMarkets,
		})
	}
	if assetOracle != nil && len(app.conf.Tyche.AssetMappings) > 0 {
		return tyche.NewNECOAssetMarketPriceSource(tyche.NECOAssetMarketPriceSourceConfig{
			Markets:       app.conf.Tyche.DYDXMarkets,
			AssetMappings: app.conf.Tyche.AssetMappings,
			Oracle:        assetOracle,
			Fallback:      fallback,
		})
	}
	return fallback
}

func (app *App) newTradingBalanceService() *tyche.TradingBalanceService {
	if app.ledger == nil || app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	return tyche.NewTradingBalanceService(app.ledger, app.newNECOTokenBalanceReader(), app.conf.Tyche.AssetMappings)
}

func (app *App) newTradingAccountSummaryService() *tyche.TradingAccountSummaryService {
	if app.trading == nil || app.tradingSync == nil || app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	return tyche.NewTradingAccountSummaryService(app.trading, app.tradingSync, app.conf.Tyche.DYDXMarkets)
}

func (app *App) newTradingAccountValuationService() *tyche.TradingAccountValuationService {
	if app.tradingBalances == nil || app.tradingAccountSummary == nil || app.conf == nil || app.conf.Tyche == nil {
		return nil
	}
	oracle := app.newAssetUSDOracle()
	if oracle == nil {
		return nil
	}
	return tyche.NewTradingAccountValuationService(app.tradingBalances, app.tradingAccountSummary, oracle, app.conf.Tyche.AssetMappings)
}

func (app *App) newTradingFundingPreparer() tyche.TradingFundingPreparer {
	if app.conf == nil || app.conf.Tyche == nil || app.conf.Tyche.NECODeposit.DepositAddress == "" || len(app.conf.Tyche.AssetMappings) == 0 {
		return nil
	}
	return tyche.NewNECOWalletFundingPreparer(app.newNECOTokenBalanceReader(), tyche.NECOWalletFundingConfig{
		DepositAddress: app.conf.Tyche.NECODeposit.DepositAddress,
		AssetMappings:  app.conf.Tyche.AssetMappings,
	})
}

func (app *App) newNECOTokenBalanceReader() tyche.NECOTokenBalanceReader {
	if app.necoBackend == nil {
		return nil
	}
	return tyche.NewNECOERC20BalanceReader(app.necoBackend)
}

func (app *App) newNECOPriceOracle() tyche.DEDXUSDOracle {
	if app.necoBackend == nil || app.conf == nil || app.conf.Tyche == nil || !app.conf.Tyche.NECOPriceOracle.Enabled {
		return nil
	}
	return tyche.NewNECOV2PairPriceOracle(app.necoBackend, app.conf.Tyche.NECOPriceOracle)
}

func (app *App) newNECOPriceQuoteService() *tyche.NECOPriceQuoteService {
	oracle := app.newAssetUSDOracle()
	if oracle == nil {
		return nil
	}
	return tyche.NewNECOPriceQuoteService(oracle)
}

func (app *App) newAssetUSDOracle() tyche.AssetUSDOracle {
	if app.necoBackend == nil || app.conf == nil || app.conf.Tyche == nil || !app.conf.Tyche.NECOPriceOracle.Enabled {
		return nil
	}
	if len(app.conf.Tyche.PriceRoutes) > 0 {
		return tyche.NewNECOV2RoutePriceOracle(app.necoBackend, app.conf.Tyche.NECOPriceOracle, app.conf.Tyche.PriceRoutes)
	}
	return tyche.NewNECOV2PairPriceOracle(app.necoBackend, app.conf.Tyche.NECOPriceOracle)
}

func (app *App) newTradingSyncWorker(cursor tyche.TradingSyncCursorStore) *tyche.TradingSyncWorker {
	if app.tradingSync == nil || cursor == nil || app.conf == nil || app.conf.Tyche == nil || app.conf.Tyche.DYDXEventSourceURL == "" {
		return nil
	}
	source := tyche.NewDYDXTradingEventHTTPSource(tyche.DYDXTradingEventHTTPSourceConfig{
		URL:    app.conf.Tyche.DYDXEventSourceURL,
		APIKey: app.conf.Tyche.DYDXEventSourceAPIKey,
	})
	return tyche.NewTradingSyncWorker(source, app.tradingSync, cursor)
}

func (app *App) newTradingReconcileWorker(store tyche.TradingReconcileOrderStore) *tyche.TradingReconcileWorker {
	if app.tradingSync == nil || store == nil || app.conf == nil || app.conf.Tyche == nil || app.conf.Tyche.DYDXEventSourceURL == "" {
		return nil
	}
	source := tyche.NewDYDXOrderReconcileHTTPSource(tyche.DYDXOrderReconcileHTTPSourceConfig{
		URL:    app.conf.Tyche.DYDXEventSourceURL,
		APIKey: app.conf.Tyche.DYDXEventSourceAPIKey,
	})
	service := tyche.NewTradingReconcileService(source, app.tradingSync, tyche.TradingReconcileConfig{
		SubaccountNumber: app.conf.Tyche.DYDXSubaccountNumber,
		OrderFlags:       tyche.DYDXOrderFlagsLongTerm,
		Markets:          app.conf.Tyche.DYDXMarkets,
	})
	return tyche.NewTradingReconcileWorker(store, service, 100)
}

func (app *App) newTradingTPSLService(store tyche.TradingTPSLStore) *tyche.TradingTPSLService {
	if app.trading == nil || app.tradingSync == nil || store == nil {
		return nil
	}
	assetOracle := app.newAssetUSDOracle()
	return tyche.NewTradingTPSLService(store, app.trading, app.newDYDXMarketPriceSource(assetOracle), app.tradingSync)
}

func (app *App) newTradingTPSLWorker() *tyche.TradingTPSLWorker {
	if app.tradingTPSL == nil {
		return nil
	}
	return tyche.NewTradingTPSLWorker(app.tradingTPSL, 100)
}

func (app *App) newDYDXChainClient() tyche.DYDXChainClient {
	if app.conf == nil || app.conf.Tyche == nil || app.conf.Tyche.DYDXChainBroadcastURL == "" {
		return nil
	}
	return tyche.NewDYDXChainHTTPClient(tyche.DYDXChainHTTPClientConfig{
		URL: app.conf.Tyche.DYDXChainBroadcastURL,
	})
}

func (app *App) newWalletBindingVerifier() tyche.WalletBindingVerifier {
	dydxVerifier := tyche.WalletBindingVerifier(tyche.UnconfiguredWalletBindingVerifier{})
	if app.conf != nil && app.conf.Tyche != nil && app.conf.Tyche.DYDXSignatureVerifyURL != "" {
		dydxVerifier = tyche.NewDYDXSignatureHTTPVerifier(tyche.DYDXSignatureHTTPVerifierConfig{
			URL: app.conf.Tyche.DYDXSignatureVerifyURL,
		})
	}

	return tyche.CompositeWalletBindingVerifier{
		NECO: tyche.EVMWalletBindingVerifier{},
		DYDX: dydxVerifier,
	}
}

func (app *App) Run(ctx context.Context) error {
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(gin.Logger())
	engine.RedirectTrailingSlash = false
	engine.Use(ginx.Cors())
	app.Router(engine)

	var c = app.conf.Tyche
	srv := new(http.Server)
	srv.Addr = c.Listen
	srv.ReadTimeout = time.Duration(c.ReadTimeout) * time.Second
	srv.WriteTimeout = time.Duration(c.WriteTimeout) * time.Second
	srv.MaxHeaderBytes = c.MaxHeaderBytes
	srv.Handler = engine

	go func() {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)

		if app.backend != nil {
			app.backend.Close()
		}
		if app.necoBackend != nil {
			app.necoBackend.Close()
		}
	}()

	if app.necoDepositSync != nil {
		go app.runNECODepositSync(ctx)
	}
	if app.necoWithdrawal != nil {
		go app.runNECOWithdrawalStartupRepair(ctx)
	}
	if app.tradingSyncRun != nil {
		go app.runTradingSync(ctx)
	}
	if app.tradingReconcileRun != nil {
		go app.runTradingReconcile(ctx)
	}
	if app.tradingTPSLRun != nil {
		go app.runTradingTPSL(ctx)
	}

	go func() {
		var err error
		if c.EnableTLS {
			err = srv.ListenAndServeTLS(c.CertFile, c.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}

		if err != http.ErrServerClosed {
			panic(err)
		}
	}()

	defer model.Dispose()
	return app.base.Run(ctx)
}

func (app *App) runNECOWithdrawalStartupRepair(ctx context.Context) {
	if app.necoWithdrawal == nil {
		return
	}
	report, err := app.necoWithdrawal.RepairFrozenWithdrawals(ctx, 1000)
	if err != nil {
		if log != nil {
			log.Errorf("NECO withdrawal startup repair failed: %v report=%+v", err, report)
		}
		return
	}
	if report.FrozenWithdrawals > 0 && log != nil {
		log.Infof("NECO withdrawal startup repair scanned frozen=%d synced=%d missing=%d errors=%d",
			report.FrozenWithdrawals,
			report.ExistingRecordsSynced,
			report.MissingRecordsDetected,
			report.Errors,
		)
	}
}

func (app *App) runNECODepositSync(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(app.conf.Tyche.PendingRetryInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deposits, err := app.necoDepositSync.PollOnce(ctx)
			if err != nil {
				if log != nil {
					log.Errorf("NECO deposit sync failed: %v", err)
				}
				continue
			}
			if len(deposits) > 0 && log != nil {
				log.Infof("NECO deposit sync processed %d deposit(s)", len(deposits))
			}
		}
	}
}

func (app *App) runTradingSync(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(app.conf.Tyche.PendingRetryInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := app.tradingSyncRun.PollOnce(ctx); err != nil && log != nil {
				log.Errorf("dYdX trading event sync failed: %v", err)
			}
		}
	}
}

func (app *App) runTradingReconcile(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(app.conf.Tyche.PendingRetryInterval) * time.Second)
	defer ticker.Stop()
	app.pollTradingReconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.pollTradingReconcile(ctx)
		}
	}
}

func (app *App) pollTradingReconcile(ctx context.Context) {
	if err := app.tradingReconcileRun.PollOnce(ctx); err != nil && log != nil {
		log.Errorf("dYdX trading reconcile failed: %v", err)
	}
}

func (app *App) runTradingTPSL(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(app.conf.Tyche.PendingRetryInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := app.tradingTPSLRun.PollOnce(ctx); err != nil && log != nil {
				log.Errorf("dYdX trading TPSL failed: %v", err)
			}
		}
	}
}

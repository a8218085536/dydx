// Production-compatible overlay reconstructed from the deployed OMS JAR.
package com.openexchange.oms.app;

import com.match.domain.FixedPoint;
import com.match.infrastructure.generated.OrderSide;
import com.match.infrastructure.generated.OrderType;
import com.openexchange.oms.api.HttpServer;
import com.openexchange.oms.api.InternalDepositService;
import com.openexchange.oms.api.OrderService;
import com.openexchange.oms.api.audit.AuditLog;
import com.openexchange.oms.api.auth.ApiKeyAuthenticationProvider;
import com.openexchange.oms.api.auth.AuthService;
import com.openexchange.oms.api.auth.AuthenticationProvider;
import com.openexchange.oms.api.auth.DemoAuthenticationProvider;
import com.openexchange.oms.api.auth.DevAuthenticationProvider;
import com.openexchange.oms.api.auth.GrpcAuthInterceptor;
import com.openexchange.oms.api.auth.JwtAuthenticationProvider;
import com.openexchange.oms.api.auth.RoleBasedAuthorizer;
import com.openexchange.oms.api.dto.ExecutionResponse;
import com.openexchange.oms.api.dto.OrderResponse;
import com.openexchange.oms.api.grpc.GrpcAccountService;
import com.openexchange.oms.api.grpc.GrpcOrderService;
import com.openexchange.oms.api.grpc.GrpcServer;
import com.openexchange.oms.api.rest.CorsPolicy;
import com.openexchange.oms.api.websocket.GatewayMarketDataSource;
import com.openexchange.oms.api.websocket.WebSocketHandler;
import com.openexchange.oms.app.AssetsHoldReconciler;
import com.openexchange.oms.app.DemoAuthService;
import com.openexchange.oms.app.MarketOrderPriceProtection;
import com.openexchange.oms.app.NecoInternalDepositService;
import com.openexchange.oms.app.NecoWithdrawalService;
import com.openexchange.oms.app.OmsAdminServiceImpl;
import com.openexchange.oms.app.OmsBalanceChecker;
import com.openexchange.oms.app.OmsConfig;
import com.openexchange.oms.app.OmsEgressAdapter;
import com.openexchange.oms.app.OmsIndexPricePoller;
import com.openexchange.oms.app.OmsMarketDataProvider;
import com.openexchange.oms.app.OmsOrderServiceImpl;
import com.openexchange.oms.app.OpenOrderSlotPolicy;
import com.openexchange.oms.app.PgBalanceReadModelWriter;
import com.openexchange.oms.app.RiskConfigBootstrap;
import com.openexchange.oms.app.TycheAuthenticationProvider;
import com.openexchange.oms.app.TycheRedisSessionValidator;
import com.openexchange.oms.assets.AeronAssetsBalanceStore;
import com.openexchange.oms.assets.AssetsClusterClient;
import com.openexchange.oms.assets.AssetsTransport;
import com.openexchange.oms.cluster.ClusterClient;
import com.openexchange.oms.cluster.OrderSubmission;
import com.openexchange.oms.common.domain.ExecutionReport;
import com.openexchange.oms.common.domain.Market;
import com.openexchange.oms.common.domain.OmsOrder;
import com.openexchange.oms.common.domain.SnowflakeIdGenerator;
import com.openexchange.oms.common.enums.Asset;
import com.openexchange.oms.common.enums.OmsOrderStatus;
import com.openexchange.oms.common.enums.OmsOrderType;
import com.openexchange.oms.core.OmsCoreEngine;
import com.openexchange.oms.core.OrderLifecycleManager;
import com.openexchange.oms.core.StartupStateRebuilder;
import com.openexchange.oms.core.SyntheticOrderEngine;
import com.openexchange.oms.ledger.BalanceStore;
import com.openexchange.oms.ledger.LedgerService;
import com.openexchange.oms.persistence.PostgresBalanceLiabilityRepository;
import com.openexchange.oms.persistence.PostgresBalanceReadModelStore;
import com.openexchange.oms.persistence.PostgresExecutionRepository;
import com.openexchange.oms.persistence.PostgresNecoDepositCreditRepository;
import com.openexchange.oms.persistence.PostgresNecoWithdrawalRepository;
import com.openexchange.oms.persistence.PostgresOrderRepository;
import com.openexchange.oms.persistence.PostgresRiskConfigStore;
import com.openexchange.oms.persistence.PostgresSchemaMigrator;
import com.openexchange.oms.persistence.PostgresTycheUserMappingRepository;
import com.openexchange.oms.persistence.PostgresUserRepository;
import com.openexchange.oms.risk.RiskConfig;
import com.openexchange.oms.risk.RiskConfigManager;
import com.openexchange.oms.risk.RiskConfigStore;
import com.openexchange.oms.risk.RiskEngine;
import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.FunctionCounter;
import io.micrometer.core.instrument.Gauge;
import io.micrometer.core.instrument.Timer;
import io.micrometer.core.instrument.binder.jvm.JvmGcMetrics;
import io.micrometer.core.instrument.binder.jvm.JvmMemoryMetrics;
import io.micrometer.core.instrument.binder.jvm.JvmThreadMetrics;
import io.micrometer.core.instrument.binder.system.ProcessorMetrics;
import io.micrometer.core.instrument.binder.system.UptimeMetrics;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import java.time.Clock;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class OmsApplication {
    private static final Logger log = LoggerFactory.getLogger(OmsApplication.class);
    private HttpServer httpServer;
    private GrpcServer grpcServer;
    private ClusterClient clusterClient;
    private AuditLog auditLog;
    private PrometheusMeterRegistry meterRegistry;
    private AssetsClusterClient assetsClusterClient;
    private AssetsHoldReconciler assetsHoldReconciler;
    private PgBalanceReadModelWriter balanceReadModelWriter;
    private AutoCloseable authProviderCloseable;
    private OmsIndexPricePoller indexPricePoller;

    public static void main(String[] stringArray) {
        OmsApplication omsApplication = new OmsApplication();
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            log.info("Shutdown hook triggered");
            omsApplication.stop();
        }));
        try {
            omsApplication.start();
        }
        catch (Exception exception) {
            log.error("Failed to start OMS application", exception);
            System.exit(1);
        }
    }

    public void start() throws Exception {
        Object object;
        log.info("Starting OMS Application...");
        OmsConfig omsConfig = OmsConfig.loadDefaults();
        log.info("Configuration loaded: httpPort={}, grpcPort={}, nodeId={}", omsConfig.httpPort(), omsConfig.grpcPort(), omsConfig.nodeId());
        HikariDataSource hikariDataSource = null;
        PostgresOrderRepository postgresOrderRepository = null;
        PostgresExecutionRepository postgresExecutionRepository = null;
        PostgresUserRepository postgresUserRepository = null;
        try {
            object = new HikariConfig();
            ((HikariConfig)object).setJdbcUrl(omsConfig.postgresUrl());
            ((HikariConfig)object).setUsername(omsConfig.postgresUser());
            ((HikariConfig)object).setPassword(omsConfig.postgresPassword());
            ((HikariConfig)object).setMaximumPoolSize(5);
            ((HikariConfig)object).setMinimumIdle(1);
            ((HikariConfig)object).setConnectionTimeout(5000L);
            hikariDataSource = new HikariDataSource((HikariConfig)object);
            PostgresSchemaMigrator.applyOmsMigrations(hikariDataSource);
            postgresOrderRepository = new PostgresOrderRepository(hikariDataSource);
            postgresExecutionRepository = new PostgresExecutionRepository(hikariDataSource);
            postgresUserRepository = new PostgresUserRepository(hikariDataSource);
            log.info("PostgreSQL persistence initialized with schema ready: {}", (Object)omsConfig.postgresUrl());
        }
        catch (Exception exception) {
            log.warn("PostgreSQL not available \u2014 running without persistence: {}", (Object)exception.getMessage());
        }
        log.info("Balance store: Assets Engine cluster (the money authority)");
        object = new AssetsClusterClient(omsConfig.balanceFeedChannel(), omsConfig.balanceFeedStreamId());
        AeronAssetsBalanceStore aeronAssetsBalanceStore2 = new AeronAssetsBalanceStore((AssetsTransport)object, Asset.values().length, omsConfig.aeHoldTimeoutMs(), omsConfig.aeAckTimeoutMs());
        Thread thread = new Thread(((AssetsClusterClient)object)::startPolling, "oms-assets-poll");
        thread.setDaemon(true);
        thread.start();
        this.assetsClusterClient = (AssetsClusterClient)object;
        if (!OmsApplication.awaitProjectionReady(aeronAssetsBalanceStore2, omsConfig.aeConnectTimeoutMs())) {
            log.error("FATAL: Assets Engine balance projection not ready within {}ms \u2014 refusing to serve traffic against a stale/unknown balance view. The AE is the money authority; a healthy AE cluster is required to start.", (Object)omsConfig.aeConnectTimeoutMs());
            System.exit(1);
        } else if (postgresExecutionRepository == null) {
            log.error("FATAL: PostgreSQL is unavailable \u2014 the settle dedupe high-water floor (PG max(trade_id)) is money-relevant and cannot start unseeded.");
            System.exit(1);
        } else {
            Object object2;
            Object object3;
            MarketOrderPriceProtection marketOrderPriceProtection;
            aeronAssetsBalanceStore2.initSettleHighWater(postgresExecutionRepository.maxTradeId());
            SnowflakeIdGenerator snowflakeIdGenerator = new SnowflakeIdGenerator(omsConfig.nodeId());
            final LedgerService ledgerService = new LedgerService(aeronAssetsBalanceStore2);
            log.info("Ledger service initialized");
            OmsMarketDataProvider omsMarketDataProvider = new OmsMarketDataProvider();
            OmsBalanceChecker omsBalanceChecker = new OmsBalanceChecker(aeronAssetsBalanceStore2);
            MarketOrderPriceProtection marketOrderPriceProtection2 = marketOrderPriceProtection = omsConfig.marketOrderPriceProtectionEnabled() ? MarketOrderPriceProtection.enabled(omsConfig.marketOrderPriceProtectionPercent(), omsConfig.omsIndexPriceMaxStaleMs()) : MarketOrderPriceProtection.disabled();
            if (marketOrderPriceProtection.enabled()) {
                this.indexPricePoller = new OmsIndexPricePoller(omsConfig.omsIndexPriceUrl(), omsConfig.omsIndexPricePollIntervalMs(), omsConfig.omsIndexPriceRequestTimeoutMs(), omsMarketDataProvider);
                this.indexPricePoller.start();
                log.info("Market-order price protection enabled: percent={} maxStaleMs={}", (Object)marketOrderPriceProtection.percent(), (Object)marketOrderPriceProtection.maxStaleMs());
            } else {
                log.warn("Market-order price protection is disabled");
            }
            int n2 = Market.maxMarketId() + 1;
            RiskEngine riskEngine = new RiskEngine(n2, omsMarketDataProvider, omsBalanceChecker);
            RiskConfigManager riskConfigManager = new RiskConfigManager(riskEngine, n2);
            RiskConfig riskConfig = RiskConfig.builder().minQuantity(1L).maxQuantity(Long.MAX_VALUE).minNotional(0L).maxNotional(Long.MAX_VALUE).priceCollarPercent(20).circuitBreakerPercent(10).circuitBreakerWindowMs(60000L).maxOrdersPerSec(100).maxOrdersPerMin(1000).maxOpenOrders(500).maxPositionPerMarket(Long.MAX_VALUE).build();
            for (Market object42 : Market.ALL) {
                riskConfigManager.setConfig(object42.marketId(), riskConfig);
            }
            log.info("Risk engine initialized with {} markets", (Object)Market.ALL.length);
            Object object5 = null;
            if (hikariDataSource != null) {
                object5 = new PostgresRiskConfigStore(hikariDataSource);
                try {
                    RiskConfigBootstrap.Result result = RiskConfigBootstrap.replay(((PostgresRiskConfigStore)object5).loadAll(), riskConfigManager, riskEngine);
                    log.info("Risk config restored from PG: {} markets loaded, {} manual circuit-breaker trips re-armed", (Object)result.marketsLoaded(), (Object)result.tripsRearmed());
                }
                catch (Exception exception) {
                    log.warn("Risk config load failed (running on hardcoded defaults until the next admin update): {}", (Object)exception.toString());
                }
            } else {
                log.warn("PostgreSQL not configured: risk config is not durable");
            }
            final OrderLifecycleManager orderLifecycleManager = new OrderLifecycleManager();
            SyntheticOrderEngine syntheticOrderEngine = new SyntheticOrderEngine();
            OmsCoreEngine omsCoreEngine = new OmsCoreEngine(orderLifecycleManager, syntheticOrderEngine);
            this.meterRegistry = new PrometheusMeterRegistry(PrometheusConfig.DEFAULT);
            new JvmMemoryMetrics().bindTo(this.meterRegistry);
            new JvmGcMetrics().bindTo(this.meterRegistry);
            new JvmThreadMetrics().bindTo(this.meterRegistry);
            new ProcessorMetrics().bindTo(this.meterRegistry);
            new UptimeMetrics().bindTo(this.meterRegistry);
            Timer timer = Timer.builder("oms_settlement_seconds").description("Trade settlement latency (ledger + risk bookkeeping)").publishPercentiles(0.5, 0.95, 0.99).register(this.meterRegistry);
            Counter counter = Counter.builder("oms_trades_settled_total").description("Trades settled (deduplicated)").register(this.meterRegistry);
            omsCoreEngine.setSettlementHandler((l, l2, l3, n, l4, l5, l6, l7) -> {
                long l8 = System.nanoTime();
                boolean bl = ledgerService.settleTradeExecution(l, l2, l3, n, l4, l5, l6, l7);
                if (!bl) {
                    return false;
                }
                riskEngine.onFill(l2, n, com.openexchange.oms.common.enums.OrderSide.BUY, l5);
                riskEngine.onFill(l3, n, com.openexchange.oms.common.enums.OrderSide.SELL, l5);
                OmsOrder omsOrder = orderLifecycleManager.getOrder(l6);
                if (omsOrder != null && omsOrder.isBuy()) {
                    ledgerService.handleOverlock(l6, l2, n, omsOrder.getPrice(), l4, l5);
                }
                timer.record(System.nanoTime() - l8, TimeUnit.NANOSECONDS);
                counter.increment();
                return true;
            });
            final PostgresOrderRepository postgresOrderRepository2 = postgresOrderRepository;
            final PostgresExecutionRepository postgresExecutionRepository2 = postgresExecutionRepository;
            omsCoreEngine.setPersistenceHandler(new OmsCoreEngine.PersistenceHandler(){

                @Override
                public void persistOrderUpdate(OmsOrder omsOrder) {
                    if (postgresOrderRepository2 != null) {
                        try {
                            postgresOrderRepository2.saveOrder(omsOrder);
                        }
                        catch (Exception exception) {
                            log.error("Failed to persist order update: omsOrderId={}", (Object)omsOrder.getOmsOrderId(), (Object)exception);
                        }
                    }
                }

                @Override
                public void persistExecution(ExecutionReport executionReport) {
                    if (postgresExecutionRepository2 != null) {
                        try {
                            postgresExecutionRepository2.saveExecution(executionReport);
                        }
                        catch (Exception exception) {
                            Exception exception2 = exception;
                            try {
                                OmsOrder omsOrder = orderLifecycleManager.getOrder(executionReport.getOmsOrderId());
                                if (postgresOrderRepository2 != null && omsOrder != null) {
                                    postgresOrderRepository2.saveOrder(omsOrder);
                                    postgresExecutionRepository2.saveExecution(executionReport);
                                    return;
                                }
                            }
                            catch (Exception exception3) {
                                exception2 = exception3;
                            }
                            log.error("Failed to persist execution: tradeId={}", (Object)executionReport.getTradeId(), (Object)exception2);
                        }
                    }
                }
            });
            if (postgresOrderRepository != null && postgresExecutionRepository != null) {
                try {
                    StartupStateRebuilder.rebuild(postgresOrderRepository.findAllOpenOrders(), postgresExecutionRepository.aggregatePositions(), orderLifecycleManager, syntheticOrderEngine, riskEngine);
                }
                catch (Exception omsEgressAdapter2) {
                    log.error("State rebuild from Postgres failed \u2014 starting with empty state (open orders will be repaired by the cluster snapshot reconcile; positions stay unknown until fills arrive)", omsEgressAdapter2);
                }
            } else {
                log.warn("Postgres unavailable \u2014 skipping startup state rebuild (oms#35)");
            }
            this.clusterClient = new ClusterClient();
            OmsEgressAdapter omsEgressAdapter2 = new OmsEgressAdapter(omsCoreEngine, omsMarketDataProvider);
            this.clusterClient.setEgressListener(omsEgressAdapter2);
            omsCoreEngine.setClusterSubmitHandler(new OmsCoreEngine.ClusterSubmitHandler(){

                @Override
                public void submitTriggeredOrder(OmsOrder omsOrder, OmsOrderType omsOrderType, long l) {
                    long l2;
                    OrderType orderType = omsOrderType == OmsOrderType.MARKET ? OrderType.MARKET : OrderType.LIMIT;
                    OrderSide orderSide = omsOrder.getSide() == com.openexchange.oms.common.enums.OrderSide.BUY ? OrderSide.BID : OrderSide.ASK;
                    try {
                        l2 = FixedPoint.multiply(l, omsOrder.getQuantity());
                    }
                    catch (ArithmeticException arithmeticException) {
                        log.error("Triggered child order notional overflows fixed-point \u2014 not submitted: parentOmsOrderId={}, childPrice={}, qty={}", omsOrder.getOmsOrderId(), l, omsOrder.getQuantity());
                        return;
                    }
                    OrderSubmission orderSubmission = OrderSubmission.createOrder(omsOrder.getUserId(), omsOrder.getMarketId(), l, omsOrder.getRemainingQty(), l2, orderType, orderSide, omsOrder.getOmsOrderId());
                    OmsApplication.this.clusterClient.submitOrder(orderSubmission);
                }

                @Override
                public boolean submitIcebergSlice(OmsOrder omsOrder, long l) {
                    long l2;
                    try {
                        l2 = FixedPoint.multiply(omsOrder.getPrice(), l);
                    }
                    catch (ArithmeticException arithmeticException) {
                        log.error("Iceberg slice notional overflows fixed-point \u2014 not submitted: omsOrderId={}, price={}, sliceQty={}", omsOrder.getOmsOrderId(), omsOrder.getPrice(), l);
                        return false;
                    }
                    OrderSide orderSide = omsOrder.getSide() == com.openexchange.oms.common.enums.OrderSide.BUY ? OrderSide.BID : OrderSide.ASK;
                    OrderSubmission orderSubmission = OrderSubmission.createOrder(omsOrder.getUserId(), omsOrder.getMarketId(), omsOrder.getPrice(), l, l2, OrderType.LIMIT, orderSide, omsOrder.getOmsOrderId());
                    return OmsApplication.this.clusterClient.submitOrder(orderSubmission);
                }

                @Override
                public void submitOpenOrdersSnapshotRequest(long l) {
                    OmsApplication.this.clusterClient.submitOrder(OrderSubmission.requestOpenOrdersSnapshot(l));
                }

                @Override
                public void submitCancel(long l, long l2, int n) {
                    OrderSubmission orderSubmission = OrderSubmission.cancelOrder(l2, l, n);
                    OmsApplication.this.clusterClient.submitOrder(orderSubmission);
                }
            });
            try {
                this.clusterClient.connect();
                log.info("Connected to cluster");
            }
            catch (Exception thread2) {
                log.warn("Cluster connection failed at startup (will retry in background): {}", (Object)thread2.getMessage());
            }
            Thread thread2 = new Thread(this.clusterClient::startPolling, "oms-cluster-poll");
            thread2.setDaemon(true);
            thread2.start();
            RoleBasedAuthorizer roleBasedAuthorizer = new RoleBasedAuthorizer();
            CorsPolicy corsPolicy = CorsPolicy.fromSpec(omsConfig.corsOrigins());
            this.auditLog = AuditLog.open(omsConfig.auditLogPath());
            OmsOrderServiceImpl omsOrderServiceImpl = new OmsOrderServiceImpl(omsCoreEngine, riskEngine, ledgerService, this.clusterClient, aeronAssetsBalanceStore2, omsEgressAdapter2, snowflakeIdGenerator, omsMarketDataProvider, marketOrderPriceProtection);
            omsOrderServiceImpl.setMeterRegistry(this.meterRegistry);
            omsOrderServiceImpl.setRepositories(postgresOrderRepository, postgresExecutionRepository);
            WebSocketHandler webSocketHandler = new WebSocketHandler(roleBasedAuthorizer, new GatewayMarketDataSource(omsConfig.marketGatewayUrl()), Set.of(Integer.valueOf(1), Integer.valueOf(2), Integer.valueOf(3)), omsOrderServiceImpl);
            omsCoreEngine.setExecutionListener(executionReport -> webSocketHandler.pushToUser(executionReport.getUserId(), "executions", Map.of("type", "EXECUTION_REPORT", "execution", ExecutionResponse.fromExecution(executionReport))));
            DemoAuthService demoAuthService = "demo".equals(omsConfig.authMode()) ? new DemoAuthService(postgresUserRepository, omsOrderServiceImpl) : null;
            AuthenticationProvider authenticationProvider = OmsApplication.buildAuthProvider(omsConfig, demoAuthService, hikariDataSource);
            if (authenticationProvider instanceof AutoCloseable) {
                object3 = (AutoCloseable)((Object)authenticationProvider);
                this.authProviderCloseable = (AutoCloseable)object3;
            }
            object3 = null;
            NecoWithdrawalService necoWithdrawalService = null;
            if (hikariDataSource != null) {
                object2 = new PostgresTycheUserMappingRepository(hikariDataSource);
                object3 = new NecoInternalDepositService(omsOrderServiceImpl, ((PostgresTycheUserMappingRepository)object2)::findOrCreateOmsUserId, new PostgresNecoDepositCreditRepository(hikariDataSource));
                necoWithdrawalService = new NecoWithdrawalService(omsOrderServiceImpl, new PostgresNecoWithdrawalRepository(hikariDataSource), new PostgresBalanceLiabilityRepository(hikariDataSource));
                log.info("Internal Neco deposit credit, user withdrawal, and reconciliation services initialized");
            } else {
                log.warn("PostgreSQL unavailable \u2014 Neco deposit credit and withdrawal endpoints disabled");
            }
            Gauge.builder("oms_active_orders", omsOrderServiceImpl, OrderService::getActiveOrderCount).description("Orders active in the OMS lifecycle").register(this.meterRegistry);
            Gauge.builder("oms_ws_connections", webSocketHandler, WebSocketHandler::getConnectionCount).description("OMS WebSocket connections").register(this.meterRegistry);
            Gauge.builder("oms_cluster_connected", omsEgressAdapter2, omsEgressAdapter -> omsEgressAdapter.isConnected() ? 1.0 : 0.0).description("Cluster egress session up (1) / down (0)").register(this.meterRegistry);
            FunctionCounter.builder("oms_egress_status_gaps_total", omsEgressAdapter2, OmsEgressAdapter::getStatusGapCount).description("OrderStatus seq gaps detected on the egress wire").register(this.meterRegistry);
            FunctionCounter.builder("oms_egress_trade_gaps_total", omsEgressAdapter2, OmsEgressAdapter::getTradeGapCount).description("TradeExecution id gaps detected on the egress wire").register(this.meterRegistry);
            FunctionCounter.builder("oms_egress_late_healed_total", omsEgressAdapter2, OmsEgressAdapter::getLateHealedCount).description("Trades that arrived out of order and healed their pending hole (benign interleave)").register(this.meterRegistry);
            Gauge.builder("oms_egress_pending_holes", omsEgressAdapter2, OmsEgressAdapter::getPendingHoleCount).description("Open tradeId holes awaiting their late trade").register(this.meterRegistry);
            FunctionCounter.builder("oms_egress_reorder_total", omsEgressAdapter2, OmsEgressAdapter::getEgressReorderCount).description("Layer 2: egress events arriving out of cluster-log order (order key only)").register(this.meterRegistry);
            Gauge.builder("oms_egress_last_seq", omsEgressAdapter2, OmsEgressAdapter::getLastEgressSeq).description("Layer 2: highest egressSeq (cluster-log order key) seen on the egress wire").register(this.meterRegistry);
            FunctionCounter.builder("oms_reconcile_repaired_total", omsCoreEngine, OmsCoreEngine::getTotalRepairedOrders).description("Orders terminalized by membership repair").register(this.meterRegistry);
            FunctionCounter.builder("oms_reconcile_relinked_total", omsCoreEngine, OmsCoreEngine::getTotalRelinkedOrders).description("Orders re-linked to cluster ids by membership repair").register(this.meterRegistry);
            FunctionCounter.builder("oms_ledger_oversettle_total", aeronAssetsBalanceStore2, BalanceStore::getOversettleCount).description("Settlements where locked was under-held and clamped (accounting-invariant break, oms#84)").register(this.meterRegistry);
            if (aeronAssetsBalanceStore2 != null) {
                FunctionCounter.builder("oms_assets_hold_timeouts_total", aeronAssetsBalanceStore2, AeronAssetsBalanceStore::getHoldTimeouts).description("AE hold requests that timed out awaiting HoldAck/HoldReject").register(this.meterRegistry);
                FunctionCounter.builder("oms_assets_amend_orphans_total", aeronAssetsBalanceStore2, AeronAssetsBalanceStore::getAmendOrphans).description("AE amend-delta hold timeouts where the compensator was suppressed (possible transient over-lock until terminal)").register(this.meterRegistry);
                FunctionCounter.builder("oms_assets_late_acks_total", aeronAssetsBalanceStore2, AeronAssetsBalanceStore::getLateAcks).description("AE acks that arrived after their correlation id was already reaped").register(this.meterRegistry);
                FunctionCounter.builder("oms_assets_compensators_total", aeronAssetsBalanceStore2, AeronAssetsBalanceStore::getCompensatorsSent).description("AE compensating releases sent for unknown-outcome holds").register(this.meterRegistry);
                Gauge.builder("oms_assets_settle_high_water", aeronAssetsBalanceStore2, AeronAssetsBalanceStore::getSettleHighWater).description("AE settle-side-effect dedupe high-water tradeId").register(this.meterRegistry);
                Gauge.builder("oms_assets_projection_ready", aeronAssetsBalanceStore2, aeronAssetsBalanceStore -> aeronAssetsBalanceStore.isProjectionReady() ? 1.0 : 0.0).description("AE balance projection ready (1) / not ready (0)").register(this.meterRegistry);
                AssetsHoldReconciler.OrderLookup orderLookup = postgresOrderRepository != null
                        ? postgresOrderRepository::findById : null;
                this.assetsHoldReconciler = new AssetsHoldReconciler(aeronAssetsBalanceStore2, orderLifecycleManager, orderLookup, Clock.systemUTC(), omsCoreEngine::isClusterOpenOmsOrderId);
                this.assetsHoldReconciler.start();
                FunctionCounter.builder("oms_assets_orphan_releases_total", this.assetsHoldReconciler, AssetsHoldReconciler::getOrphanReleasesTotal).description("AE holds released as provably-never-submitted orphans (E5)").register(this.meterRegistry);
                FunctionCounter.builder("oms_assets_reconciler_sweeps_total", this.assetsHoldReconciler, AssetsHoldReconciler::getSweepsTotal).description("AE orphan-hold reconciler sweeps run (E5)").register(this.meterRegistry);
                Gauge.builder("oms_assets_unresolved_orphans", this.assetsHoldReconciler, AssetsHoldReconciler::getUnresolvedOrphansLastSweep).description("AE holds not provably releasable at the last sweep (surfaced/pending) (E5)").register(this.meterRegistry);
                log.info("Assets orphan-hold reconciler started (initial sweep gated on the first ME open-orders reconcile)");
                if (hikariDataSource != null) {
                    this.balanceReadModelWriter = new PgBalanceReadModelWriter(new PostgresBalanceReadModelStore(hikariDataSource));
                    aeronAssetsBalanceStore2.setBalanceChangeConsumer(this.balanceReadModelWriter);
                    this.balanceReadModelWriter.start();
                    FunctionCounter.builder("oms_balance_readmodel_rows_written_total", this.balanceReadModelWriter, PgBalanceReadModelWriter::getRowsWritten).description("Rows upserted into the CQRS balance read model (E6)").register(this.meterRegistry);
                    FunctionCounter.builder("oms_balance_readmodel_flush_failures_total", this.balanceReadModelWriter, PgBalanceReadModelWriter::getFlushFailures).description("Balance read-model flush ticks that failed (PG unavailable etc.) (E6)").register(this.meterRegistry);
                    Gauge.builder("oms_balance_readmodel_dirty", this.balanceReadModelWriter, PgBalanceReadModelWriter::getDirtyCount).description("Balance read-model (user,asset) keys awaiting flush (E6)").register(this.meterRegistry);
                    log.info("CQRS balance read-model writer started (mirroring AE balances to account_balances)");
                } else {
                    log.warn("PG unavailable \u2014 CQRS balance read model disabled (no durable balance mirror)");
                }
            }
            GrpcOrderService grpcOrderService = new GrpcOrderService(omsOrderServiceImpl, roleBasedAuthorizer, this.auditLog);
            GrpcAccountService grpcAccountService = new GrpcAccountService(omsOrderServiceImpl, roleBasedAuthorizer);
            orderLifecycleManager.setReplaceHooks(new OrderLifecycleManager.ReplaceHooks(){

                @Override
                public void onReplaceResolved(OmsOrder omsOrder) {
                    ledgerService.resolveAmendHold(omsOrder);
                }

                @Override
                public void onReplaceAborted(OmsOrder omsOrder) {
                    ledgerService.abortAmendHold(omsOrder);
                }
            });
            orderLifecycleManager.setStateListener((arg_0, arg_1, arg_2) -> OmsApplication.handleStateTransition(webSocketHandler, grpcOrderService, ledgerService, riskEngine, syntheticOrderEngine, arg_0, arg_1, arg_2));
            omsCoreEngine.setPostReconcileHook(() -> {
                HashMap<Long, Long> hashMap = new HashMap<Long, Long>();
                orderLifecycleManager.forEachActiveOrder(omsOrder -> {
                    OmsOrderStatus omsOrderStatus = omsOrder.getStatus();
                    if (omsOrderStatus == OmsOrderStatus.PENDING_NEW || omsOrderStatus == OmsOrderStatus.NEW || omsOrderStatus == OmsOrderStatus.PARTIALLY_FILLED || omsOrderStatus == OmsOrderStatus.PENDING_TRIGGER) {
                        hashMap.merge(omsOrder.getUserId(), 1L, Long::sum);
                    }
                });
                long l = riskEngine.rebaselineOpenOrderCounts(hashMap);
                if (l != 0L) {
                    log.warn("Open-order slot rebaseline corrected drift of {} across {} users (oms#49)", (Object)l, (Object)hashMap.size());
                }
                if (this.assetsHoldReconciler != null) {
                    this.assetsHoldReconciler.onStartupReconcileComplete();
                }
            });
            OmsAdminServiceImpl omsAdminServiceImpl = new OmsAdminServiceImpl(riskConfigManager, riskEngine, (RiskConfigStore)object5);
            FunctionCounter.builder("oms_risk_config_persist_failures_total", omsAdminServiceImpl, OmsAdminServiceImpl::getPersistFailures).description("Admin risk-config/breaker persists that failed (change applied in memory only)").register(this.meterRegistry);
            this.httpServer = new HttpServer(omsConfig.httpPort(), omsOrderServiceImpl, webSocketHandler, omsAdminServiceImpl, authenticationProvider, roleBasedAuthorizer, corsPolicy, this.auditLog, this.meterRegistry, (InternalDepositService)object3, necoWithdrawalService, omsConfig.internalKey());
            this.httpServer.setAuthService(demoAuthService);
            this.httpServer.start();
            this.grpcServer = new GrpcServer(omsConfig.grpcPort(), grpcOrderService, grpcAccountService, new GrpcAuthInterceptor(authenticationProvider));
            this.grpcServer.start();
            log.info("gRPC server started on port {}", (Object)omsConfig.grpcPort());
            ScheduledExecutorService scheduledExecutorService = Executors.newSingleThreadScheduledExecutor(runnable -> {
                Thread gtdThread = new Thread(runnable, "oms-gtd-expiry");
                gtdThread.setDaemon(true);
                return gtdThread;
            });
            scheduledExecutorService.scheduleAtFixedRate(() -> {
                try {
                    omsCoreEngine.checkGtdExpiry(System.currentTimeMillis());
                }
                catch (Exception exception) {
                    log.error("GTD expiry check failed", exception);
                }
                try {
                    omsEgressAdapter2.sweepStaleOrphans();
                }
                catch (Exception exception) {
                    log.error("Stale-orphan sweep failed", exception);
                }
            }, 1L, 1L, TimeUnit.SECONDS);
            log.info("GTD expiry checker started (1s interval)");
            log.info("OMS Application started successfully");
        }
    }

    private static boolean awaitProjectionReady(AeronAssetsBalanceStore aeronAssetsBalanceStore, long l) {
        long l2 = System.currentTimeMillis() + l;
        while (System.currentTimeMillis() < l2) {
            if (aeronAssetsBalanceStore.isProjectionReady()) {
                log.info("AE balance projection ready");
                return true;
            }
            try {
                Thread.sleep(50L);
            }
            catch (InterruptedException interruptedException) {
                Thread.currentThread().interrupt();
                log.error("Interrupted while awaiting AE balance projection readiness");
                return false;
            }
        }
        return aeronAssetsBalanceStore.isProjectionReady();
    }

    private static AuthenticationProvider buildAuthProvider(OmsConfig omsConfig, AuthService authService, HikariDataSource hikariDataSource) {
        String string;
        switch (string = omsConfig.authMode()) {
            case "dev": {
                log.warn("AUTH: dev mode \u2014 every request is accepted with caller-chosen identity. NEVER use in production (set OMS_AUTH_MODE=api-key or jwt).");
                return new DevAuthenticationProvider();
            }
            case "demo": {
                log.info("AUTH: demo mode \u2014 registered users (opaque tokens, self-scoped) plus a dev-token backdoor restricted to userId 1 and the sim range 900000-900999" + (omsConfig.demoAllowAnyDevUser() ? "; local mapped-deposit-user dev tokens are enabled" : ""));
                return new DemoAuthenticationProvider(authService, omsConfig.demoAllowAnyDevUser());
            }
            case "jwt": {
                log.info("AUTH: jwt mode (HS256)");
                return new JwtAuthenticationProvider(omsConfig.jwtSecret());
            }
            case "tyche": {
                if (hikariDataSource == null) {
                    throw new IllegalStateException("OMS_AUTH_MODE=tyche requires PostgreSQL for Tyche-to-OMS user mapping");
                }
                if (omsConfig.tycheRedisNamespace() != null && !omsConfig.tycheRedisNamespace().isBlank()) {
                    log.info("AUTH: tyche mode \u2014 accepting Tyche FlyDEX wallet-login session tokens");
                    TycheRedisSessionValidator tycheRedisSessionValidator = new TycheRedisSessionValidator(omsConfig.tycheRedisUri(), omsConfig.tycheRedisNamespace());
                    PostgresTycheUserMappingRepository postgresTycheUserMappingRepository = new PostgresTycheUserMappingRepository(hikariDataSource);
                    return new TycheAuthenticationProvider(tycheRedisSessionValidator, postgresTycheUserMappingRepository::findOrCreateOmsUserId);
                }
                throw new IllegalArgumentException("TYCHE_REDIS_NAMESPACE is required for OMS_AUTH_MODE=tyche; use Tyche redis.domain + ':' + tyche.account");
            }
            case "tyche-api-key": {
                if (hikariDataSource == null) {
                    throw new IllegalStateException("OMS_AUTH_MODE=tyche-api-key requires PostgreSQL for Tyche-to-OMS user mapping");
                }
                if (omsConfig.tycheRedisNamespace() == null || omsConfig.tycheRedisNamespace().isBlank()) {
                    throw new IllegalArgumentException("TYCHE_REDIS_NAMESPACE is required for OMS_AUTH_MODE=tyche-api-key; use Tyche redis.domain + ':' + tyche.account");
                }
                ApiKeyAuthenticationProvider apiKeyAuthenticationProvider = ApiKeyAuthenticationProvider.parse(omsConfig.apiKeys(), omsConfig.apiKeysFile());
                if (apiKeyAuthenticationProvider.isEmpty()) {
                    throw new IllegalArgumentException("OMS_AUTH_MODE=tyche-api-key requires OMS_API_KEYS or OMS_API_KEYS_FILE");
                }
                log.info("AUTH: tyche-api-key mode - accepting Tyche sessions and dedicated API keys");
                TycheRedisSessionValidator tycheRedisSessionValidator = new TycheRedisSessionValidator(omsConfig.tycheRedisUri(), omsConfig.tycheRedisNamespace());
                PostgresTycheUserMappingRepository postgresTycheUserMappingRepository = new PostgresTycheUserMappingRepository(hikariDataSource);
                return new TycheApiKeyAuthenticationProvider(apiKeyAuthenticationProvider,
                        new TycheAuthenticationProvider(tycheRedisSessionValidator,
                                postgresTycheUserMappingRepository::findOrCreateOmsUserId));
            }
            case "api-key": {
                ApiKeyAuthenticationProvider apiKeyAuthenticationProvider = ApiKeyAuthenticationProvider.parse(omsConfig.apiKeys(), omsConfig.apiKeysFile());
                if (apiKeyAuthenticationProvider.isEmpty()) {
                    log.warn("AUTH: api-key mode with NO keys configured \u2014 every request will be rejected. Set OMS_API_KEYS / OMS_API_KEYS_FILE, or OMS_AUTH_MODE=dev for development.");
                } else {
                    log.info("AUTH: api-key mode");
                }
                return apiKeyAuthenticationProvider;
            }
        }
        throw new IllegalArgumentException("Unknown OMS_AUTH_MODE '" + omsConfig.authMode() + "' (expected api-key, jwt, tyche, tyche-api-key, demo, or dev)");
    }

    public void stop() {
        log.info("Stopping OMS Application...");
        if (this.grpcServer != null) {
            this.grpcServer.stop();
        }
        if (this.httpServer != null) {
            this.httpServer.stop();
        }
        if (this.clusterClient != null) {
            this.clusterClient.stopPolling();
            this.clusterClient.close();
        }
        if (this.indexPricePoller != null) {
            this.indexPricePoller.close();
        }
        if (this.assetsHoldReconciler != null) {
            this.assetsHoldReconciler.stop();
        }
        if (this.assetsClusterClient != null) {
            this.assetsClusterClient.stopPolling();
            this.assetsClusterClient.close();
        }
        if (this.balanceReadModelWriter != null) {
            this.balanceReadModelWriter.stop();
        }
        if (this.authProviderCloseable != null) {
            try {
                this.authProviderCloseable.close();
            }
            catch (Exception exception) {
                log.warn("Auth provider close failed: {}", (Object)exception.getMessage());
            }
        }
        if (this.auditLog != null) {
            try {
                this.auditLog.close();
            }
            catch (Exception exception) {
                log.warn("Audit log close failed: {}", (Object)exception.getMessage());
            }
        }
        log.info("OMS Application stopped");
    }

    private static void handleStateTransition(WebSocketHandler webSocketHandler, GrpcOrderService grpcOrderService, LedgerService ledgerService, RiskEngine riskEngine, SyntheticOrderEngine syntheticOrderEngine, OmsOrder omsOrder, OmsOrderStatus omsOrderStatus, OmsOrderStatus omsOrderStatus2) {
        OrderResponse orderResponse = OrderResponse.fromOrder(omsOrder);
        webSocketHandler.pushToUser(omsOrder.getUserId(), "orders", orderResponse);
        grpcOrderService.pushOrderUpdate(omsOrder.getUserId(), orderResponse);
        if (OpenOrderSlotPolicy.releasesHold(omsOrderStatus, omsOrderStatus2)) {
            ledgerService.releaseForCancel(omsOrder);
        }
        if (OpenOrderSlotPolicy.releasesSlot(omsOrderStatus, omsOrderStatus2)) {
            riskEngine.onOrderClosed(omsOrder.getUserId());
        }
        if (omsOrder.getOrderType() == OmsOrderType.ICEBERG && omsOrderStatus2.isTerminal()) {
            syntheticOrderEngine.removeOrder(omsOrder);
            if (omsOrderStatus2 == OmsOrderStatus.FILLED) {
                ledgerService.releaseForCancel(omsOrder);
            }
        }
    }
}

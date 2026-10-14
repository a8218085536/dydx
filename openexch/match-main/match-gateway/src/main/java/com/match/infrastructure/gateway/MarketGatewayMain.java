// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway;

import com.match.infrastructure.admin.ClusterStatus;
import com.match.infrastructure.gateway.edge.EdgePublisher;
import com.match.infrastructure.gateway.persistence.MarketDataPersistence;
import com.match.infrastructure.gateway.price.IndexPricePoller;
import com.match.infrastructure.gateway.state.GatewayStateManager;
import com.match.infrastructure.websocket.MarketDataWebSocket;

import java.time.Duration;

/**
 * Market Gateway - broadcasts market data from cluster to WebSocket clients.
 * Maintains local state (order book, trades, open orders) for serving client queries.
 * Runs as a separate process on port 8081.
 */
public class MarketGatewayMain implements AutoCloseable {

    private final AeronGateway aeronGateway;
    private final MarketDataWebSocket marketDataWebSocket;
    private final GatewayStateManager stateManager;
    private final ClusterStatus clusterStatus;
    private final MarketDataPersistence persistence; // null when explicitly configured for memory mode
    private final EdgePublisher edgePublisher; // null when MARKET_EDGE_URL unset
    private final IndexPricePoller indexPricePoller;
    private final CandleHeartbeatScheduler candleHeartbeatScheduler;

    public MarketGatewayMain() {
        this.clusterStatus = new ClusterStatus();

        // Create state manager for local state caching
        this.stateManager = new GatewayStateManager();
        this.indexPricePoller = new IndexPricePoller(
                stateManager,
                System.getenv().getOrDefault(
                        "MARKET_INDEX_PRICE_BASE_URL",
                        "http://127.0.0.1:28080/api/v1/index-prices"),
                Duration.ofSeconds(5));
        this.candleHeartbeatScheduler = new CandleHeartbeatScheduler(
                stateManager, Duration.ofSeconds(1));

        // Market-data persistence (TimescaleDB): the source of truth for
        // chart/time-series data. Null (pure in-memory) when not configured.
        this.persistence = MarketDataPersistence.start(stateManager);
        this.stateManager.setPersistence(persistence);

        this.aeronGateway = new AeronGateway();
        this.aeronGateway.setClusterStatus(clusterStatus);

        this.marketDataWebSocket = new MarketDataWebSocket();
        this.marketDataWebSocket.setClusterStatus(clusterStatus);
        this.marketDataWebSocket.setStateManager(stateManager);
        this.marketDataWebSocket.setAeronGateway(aeronGateway); // match#33: /metrics relay stats

        // Optional edge fan-out (edge/market-relay): tee broadcasts once to
        // the relay Worker instead of pushing per-viewer copies upstream.
        this.edgePublisher = EdgePublisher.startOrNull(stateManager);
        this.marketDataWebSocket.setEdgePublisher(edgePublisher);
        if (edgePublisher != null) {
            this.clusterStatus.setEdgeTee(edgePublisher::publish);
        }

        // Wire state manager as egress listener (handles state updates and broadcasts)
        this.stateManager.setWebSocket(marketDataWebSocket);
        this.aeronGateway.setEgressListener(stateManager);
    }

    public void start() throws Exception {
        // Seed candle rings + ticker from the database BEFORE touching the
        // cluster: AeronCluster.connect() can already dispatch egress messages
        // (and thus ring writes) during session establishment, and seed()
        // must never run after live trades have entered the rings.
        if (persistence != null) {
            persistence.hydrate(stateManager.inMemoryCandleProvider());
        }

        // Connect to cluster
        aeronGateway.connect();

        // Start WebSocket server
        marketDataWebSocket.start();

        // Guide prices are independent of trades and order-book state.
        indexPricePoller.start();
        candleHeartbeatScheduler.start();

        clusterStatus.setMarketChannels(marketDataWebSocket.getChannels());
    }

    public void startPolling() {
        aeronGateway.startPolling();
    }

    @Override
    public void close() {
        candleHeartbeatScheduler.close();
        indexPricePoller.close();
        if (edgePublisher != null) {
            edgePublisher.close();
        }
        if (marketDataWebSocket != null) {
            marketDataWebSocket.close();
        }
        if (aeronGateway != null) {
            aeronGateway.close();
        }
        // After the egress producer stops, so the final drain sees everything
        if (persistence != null) {
            persistence.close();
        }
    }

    public static void main(String[] args) {
        // Set global uncaught exception handler to prevent silent JVM death
        Thread.setDefaultUncaughtExceptionHandler((thread, throwable) -> {
            System.err.println("FATAL: Uncaught exception in thread " + thread.getName());
            throwable.printStackTrace();
            System.err.flush();
        });

        try {
            MarketGatewayMain gateway = new MarketGatewayMain();
            Runtime.getRuntime().addShutdownHook(new Thread(gateway::close));
            gateway.start();
            gateway.startPolling();
        } catch (Throwable t) {
            System.err.println("Failed to start market gateway: " + t.getMessage());
            t.printStackTrace();
            System.exit(1);
        }
    }
}

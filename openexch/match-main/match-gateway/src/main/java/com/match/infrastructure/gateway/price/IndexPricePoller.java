// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.price;

import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.match.infrastructure.Logger;
import com.match.infrastructure.gateway.state.GatewayStateManager;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;

/** Polls the existing guide-price service and merges valid prices into spot tickers. */
public final class IndexPricePoller implements AutoCloseable {
    private static final Logger logger = Logger.getLogger(IndexPricePoller.class);
    private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(3);
    private static final Map<Integer, String> MARKET_SYMBOLS = marketSymbols();

    private final GatewayStateManager stateManager;
    private final String baseUrl;
    private final Duration interval;
    private final HttpClient httpClient;
    private final ScheduledExecutorService scheduler;
    private final AtomicBoolean started = new AtomicBoolean();
    private final AtomicLong consecutiveFailedCycles = new AtomicLong();

    public IndexPricePoller(GatewayStateManager stateManager, String baseUrl, Duration interval) {
        if (stateManager == null) {
            throw new IllegalArgumentException("stateManager is required");
        }
        if (baseUrl == null || baseUrl.isBlank()) {
            throw new IllegalArgumentException("baseUrl is required");
        }
        if (interval == null || interval.isZero() || interval.isNegative()) {
            throw new IllegalArgumentException("interval must be positive");
        }
        this.stateManager = stateManager;
        this.baseUrl = trimTrailingSlash(baseUrl.trim());
        this.interval = interval;
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(REQUEST_TIMEOUT)
                .build();
        this.scheduler = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread thread = new Thread(r, "spot-index-price-poller");
            thread.setDaemon(true);
            return thread;
        });
    }

    public void start() {
        if (started.compareAndSet(false, true)) {
            scheduler.scheduleWithFixedDelay(
                    this::refreshSafely, 0, interval.toMillis(), TimeUnit.MILLISECONDS);
        }
    }

    void refreshNow() {
        int updated = 0;
        for (Map.Entry<Integer, String> market : MARKET_SYMBOLS.entrySet()) {
            if (refreshMarket(market.getKey(), market.getValue())) {
                updated++;
            }
        }
        if (updated > 0) {
            consecutiveFailedCycles.set(0);
        } else {
            long failures = consecutiveFailedCycles.incrementAndGet();
            if (failures == 1 || failures % 12 == 0) {
                logger.warn("Guide-price refresh produced no valid values (cycle " + failures + ")");
            }
        }
    }

    private void refreshSafely() {
        try {
            refreshNow();
        } catch (Throwable t) {
            logger.error("Unexpected guide-price refresh failure: " + t.getMessage());
        }
    }

    private boolean refreshMarket(int marketId, String symbol) {
        try {
            HttpRequest request = HttpRequest.newBuilder(URI.create(baseUrl + "/" + symbol))
                    .timeout(REQUEST_TIMEOUT)
                    .header("Accept", "application/json")
                    .GET()
                    .build();
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            if (response.statusCode() < 200 || response.statusCode() >= 300) {
                return false;
            }

            JsonObject payload = JsonParser.parseString(response.body()).getAsJsonObject();
            if (!payload.has("stale") || payload.get("stale").isJsonNull()
                    || payload.get("stale").getAsBoolean()) {
                return false;
            }
            if (!payload.has("indexPrice") || payload.get("indexPrice").isJsonNull()) {
                return false;
            }

            double price = payload.get("indexPrice").getAsDouble();
            if (!Double.isFinite(price) || price <= 0) {
                return false;
            }
            stateManager.updateExternalIndexPrice(marketId, price);
            return true;
        } catch (Exception e) {
            return false;
        }
    }

    @Override
    public void close() {
        scheduler.shutdownNow();
    }

    private static Map<Integer, String> marketSymbols() {
        Map<Integer, String> symbols = new LinkedHashMap<>();
        symbols.put(1, "NETH-USDT");
        symbols.put(2, "NBNB-USDT");
        symbols.put(3, "NBTC-USDT");
        return Map.copyOf(symbols);
    }

    private static String trimTrailingSlash(String value) {
        int end = value.length();
        while (end > 0 && value.charAt(end - 1) == '/') {
            end--;
        }
        return value.substring(0, end);
    }
}

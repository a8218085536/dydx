// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.state;

import com.match.domain.MarketDisplay;

import java.util.Locale;
import java.util.concurrent.locks.StampedLock;

/**
 * Per-market ticker statistics accumulated from trades.
 * Thread-safe: single writer (egress polling thread), multiple readers (WebSocket/HTTP handlers).
 * Uses StampedLock for optimistic reads - same pattern as GatewayOrderBook.
 */
public class TickerStats {
    /** A DB baseline older than this is ignored (falls back to since-boot semantics). */
    static final long BASELINE_FRESH_MS = 30_000;

    private volatile double lastPrice = 0.0;
    private volatile double indexPrice = 0.0;
    private volatile double priceChangePercent1h = 0.0;
    private volatile boolean priceChangePercent1hKnown = false;
    private volatile double openPrice = 0.0;  // First price seen (for change calculation)
    private volatile double high24h = 0.0;
    private volatile double low24h = Double.MAX_VALUE;
    private volatile double volume24h = 0.0;
    private volatile long lastUpdateMs = 0;
    private volatile boolean hasData = false;
    private volatile String cachedJson;

    // Rolling-24h baseline computed from the persisted 1m candles, plus
    // live-since-baseline accumulators. When the baseline is fresh the ticker
    // reports true rolling-24h figures (baseline merged with live trades);
    // when it goes stale (DB down) the legacy since-boot fields above take over.
    private volatile double baseOpen24h = 0.0;
    private volatile double baseHigh24h = 0.0;
    private volatile double baseLow24h = 0.0;
    private volatile double baseQuoteVol24h = 0.0;
    private volatile long baseTradeCount24h = 0;
    private volatile long baselineAsOfMs = 0;
    private volatile double liveHigh = 0.0;
    private volatile double liveLow = Double.MAX_VALUE;
    private volatile double liveQuoteVol = 0.0;
    private volatile long liveTradeCount = 0;
    private volatile boolean liveHasData = false;

    private final StampedLock lock = new StampedLock();
    private final int marketId;
    private final String marketName;

    public TickerStats(int marketId, String marketName) {
        this.marketId = marketId;
        this.marketName = marketName;
    }

    /**
     * Update stats from a trade execution.
     * Called from egress polling thread (single writer).
     */
    public void updateFromTrade(double price, double quantity) {
        updateFromTrade(price, quantity, 1);
    }

    /** Update stats from an aggregated trade and its underlying execution count. */
    public void updateFromTrade(double price, double quantity, int tradeCount) {
        long stamp = lock.writeLock();
        try {
            if (!hasData) {
                openPrice = price;
                high24h = price;
                low24h = price;
                hasData = true;
            } else {
                high24h = Math.max(high24h, price);
                low24h = Math.min(low24h, price);
            }
            lastPrice = price;
            volume24h += price * quantity;
            if (!liveHasData) {
                liveHigh = price;
                liveLow = price;
                liveHasData = true;
            } else {
                liveHigh = Math.max(liveHigh, price);
                liveLow = Math.min(liveLow, price);
            }
            liveQuoteVol += price * quantity;
            liveTradeCount += Math.max(1, tradeCount);
            lastUpdateMs = System.currentTimeMillis();
            cachedJson = buildJson();
        } finally {
            lock.unlockWrite(stamp);
        }
    }

    /**
     * Apply a rolling-24h baseline computed from the persisted 1m candles.
     * Called from the persistence ticker thread every few seconds. Resets the
     * live-since-baseline accumulators (their window is now inside the
     * baseline). Also seeds lastPrice after a restart so the ticker is
     * populated before the first post-boot trade.
     */
    public void applyDbBaseline(double open24h, double high24hDb, double low24hDb,
                                double quoteVol24h, double lastClose, long asOfMs) {
        applyDbBaseline(open24h, high24hDb, low24hDb, quoteVol24h, lastClose, 0, asOfMs);
    }

    /** Apply a rolling-24h baseline including the persisted execution count. */
    public void applyDbBaseline(double open24h, double high24hDb, double low24hDb,
                                double quoteVol24h, double lastClose, long tradeCount24h,
                                long asOfMs) {
        long stamp = lock.writeLock();
        try {
            baseOpen24h = open24h;
            baseHigh24h = high24hDb;
            baseLow24h = low24hDb;
            baseQuoteVol24h = quoteVol24h;
            baseTradeCount24h = Math.max(0, tradeCount24h);
            baselineAsOfMs = asOfMs;
            liveHigh = 0.0;
            liveLow = Double.MAX_VALUE;
            liveQuoteVol = 0.0;
            liveTradeCount = 0;
            liveHasData = false;
            if (!hasData && lastClose > 0) {
                lastPrice = lastClose;
                openPrice = open24h > 0 ? open24h : lastClose;
                high24h = high24hDb > 0 ? high24hDb : lastClose;
                low24h = low24hDb > 0 ? low24hDb : lastClose;
                lastUpdateMs = asOfMs;
                hasData = true;
            }
            cachedJson = buildJson();
        } finally {
            lock.unlockWrite(stamp);
        }
    }

    /** Set the externally sourced spot reference/index price. */
    public void updateIndexPrice(double price) {
        long stamp = lock.writeLock();
        try {
            if (price > 0) {
                indexPrice = price;
                cachedJson = buildJson();
            }
        } finally {
            lock.unlockWrite(stamp);
        }
    }

    /** Set the rolling one-hour percentage change derived from hourly candles. */
    public void updatePriceChangePercent1h(double percent) {
        long stamp = lock.writeLock();
        try {
            priceChangePercent1h = percent;
            priceChangePercent1hKnown = true;
            cachedJson = buildJson();
        } finally {
            lock.unlockWrite(stamp);
        }
    }

    /**
     * Get current stats as JSON. Uses optimistic read for lock-free fast path.
     */
    public String toJson() {
        long stamp = lock.tryOptimisticRead();
        String json = cachedJson;
        if (!lock.validate(stamp)) {
            stamp = lock.readLock();
            try {
                json = cachedJson;
            } finally {
                lock.unlockRead(stamp);
            }
        }
        return json != null ? json : buildEmptyJson();
    }

    /**
     * Check if this ticker has received any trade data.
     */
    public boolean hasData() {
        return hasData;
    }

    /**
     * Get last price (for mid-price calculations).
     */
    public double getLastPrice() {
        return lastPrice;
    }

    private String buildJson() {
        boolean fresh = baselineAsOfMs > 0
                && System.currentTimeMillis() - baselineAsOfMs < BASELINE_FRESH_MS;
        double open, high, low, volume;
        if (fresh) {
            // True rolling 24h: DB baseline merged with trades seen since it
            open = baseOpen24h > 0 ? baseOpen24h : openPrice;
            high = Math.max(baseHigh24h, liveHasData ? liveHigh : 0);
            if (high == 0) {
                high = high24h;
            }
            if (liveHasData && baseLow24h > 0) {
                low = Math.min(baseLow24h, liveLow);
            } else if (liveHasData) {
                low = liveLow;
            } else if (baseLow24h > 0) {
                low = baseLow24h;
            } else {
                low = low24h == Double.MAX_VALUE ? 0 : low24h;
            }
            volume = baseQuoteVol24h + liveQuoteVol;
        } else {
            // Database blip: since-boot semantics, served while markDown (loud + health-visible)
            open = openPrice;
            high = high24h;
            low = low24h == Double.MAX_VALUE ? 0 : low24h;
            volume = volume24h;
        }
        boolean priceKnown = hasData && lastPrice > 0 && open > 0;
        double priceChange = priceKnown ? lastPrice - open : 0;
        double pctChange = priceKnown ? (priceChange / open * 100) : 0;
        boolean volumeKnown = fresh || hasData;
        boolean tradeCountKnown = fresh || hasData;
        long tradeCount = fresh ? baseTradeCount24h + liveTradeCount : liveTradeCount;
        String lastPriceJson = hasData ? format(lastPrice) : "null";
        String indexPriceJson = indexPrice > 0 ? format(indexPrice) : "null";
        String priceChangeJson = priceKnown ? format(priceChange) : "null";
        String pctChangeJson = priceKnown ? format(pctChange) : "null";
        String oneHourJson = priceChangePercent1hKnown ? format(priceChangePercent1h) : "null";
        String highJson = high > 0 ? format(high) : "null";
        String lowJson = low > 0 && low != Double.MAX_VALUE ? format(low) : "null";
        String volumeJson = volumeKnown ? format(volume) : "null";
        String tradeCountJson = tradeCountKnown ? Long.toString(tradeCount) : "null";
        String timestampJson = lastUpdateMs > 0 ? Long.toString(lastUpdateMs) : "null";
        return String.format(Locale.ROOT,
            "{\"type\":\"TICKER_STATS\",\"marketId\":%d,\"market\":\"%s\"," +
            "\"lastPrice\":%s,\"indexPrice\":%s,\"priceChange\":%s," +
            "\"priceChangePercent\":%s,\"priceChangePercent1h\":%s,\"tradeCount24h\":%s," +
            "\"openInterest\":null,\"fundingRate\":null,\"nextFundingTime\":null," +
            "\"high24h\":%s,\"low24h\":%s,\"volume24h\":%s,\"timestamp\":%s}",
            marketId, MarketDisplay.symbolOrUnknown(marketId), lastPriceJson, indexPriceJson, priceChangeJson,
            pctChangeJson, oneHourJson, tradeCountJson, highJson, lowJson, volumeJson, timestampJson);
    }

    private String buildEmptyJson() {
        return String.format(Locale.ROOT,
            "{\"type\":\"TICKER_STATS\",\"marketId\":%d,\"market\":\"%s\"," +
            "\"lastPrice\":null,\"indexPrice\":null,\"priceChange\":null,\"priceChangePercent\":null," +
            "\"priceChangePercent1h\":null,\"tradeCount24h\":null," +
            "\"openInterest\":null,\"fundingRate\":null,\"nextFundingTime\":null," +
            "\"high24h\":null,\"low24h\":null,\"volume24h\":null,\"timestamp\":null}",
            marketId, MarketDisplay.symbolOrUnknown(marketId));
    }

    private static String format(double value) {
        return String.format(Locale.ROOT, "%.8f", value);
    }
}

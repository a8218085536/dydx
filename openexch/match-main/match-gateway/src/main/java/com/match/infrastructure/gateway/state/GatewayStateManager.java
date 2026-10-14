// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.state;

import com.google.gson.JsonArray;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.match.infrastructure.Logger;
import com.match.infrastructure.gateway.AeronGateway;
import com.match.domain.MarketDisplay;
import com.match.infrastructure.generated.BookDeltaDecoder;
import com.match.infrastructure.generated.BookSnapshotDecoder;
import com.match.domain.MarketInfo;
import com.match.infrastructure.generated.OrderSide;
import com.match.infrastructure.generated.OrderStatusBatchDecoder;
import com.match.infrastructure.generated.TradesBatchDecoder;
import com.match.infrastructure.websocket.MarketDataWebSocket;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import static com.match.domain.FixedPoint.SCALE_FACTOR;

/**
 * Centralized state manager for Market Gateway.
 * Coordinates order book, trades, and orders state.
 * Implements EgressMessageListener to receive cluster messages.
 *
 * <p><b>EXECUTABLE SPEC — FROZEN FOR THE C++ GATEWAY PORT. DO NOT REFACTOR BEHAVIOR.</b>
 *
 * <p>The book dedup/staleness ladder in {@link #onBookSnapshot} and {@link #onBookDelta}
 * — leader-switchover dedup (match#19), stale-snapshot dedup (match#95), chain-break drift
 * detection and snapshot re-anchoring (match#96), and the four distinct drop/break counters —
 * is, together with {@link GatewayOrderBook}, the ONLY behavioral specification for the C++
 * market-gateway rewrite. The cluster deliberately does NOT dedup its egress across leader
 * switchovers: it only produces the book-version chain ({@code MarketPublisher}) and
 * periodically resnapshots ({@code AppClusteredService}); every consumer-side guarantee
 * lives in this class and nowhere else.
 *
 * <p>Pinning tests (the executable part of the spec):
 * {@code GatewayStateManagerSbeTest} (the ladder itself, over real SBE frames),
 * {@code GatewayOrderBookTest} (book/stale-flag mechanics),
 * {@code GatewayStateManagerTest} (accessor surface).
 *
 * <p>Behavior-changing refactors are FORBIDDEN until the port lands. Any intentional
 * behavior change is a SPEC change: it must update {@code docs/gateway-port-spec.md} and
 * the pinning tests in the same commit, and say so in the commit message. See
 * {@code docs/gateway-port-spec.md} for the invariant map and port checklist.
 */
public class GatewayStateManager implements AeronGateway.EgressMessageListener {
    private static final Logger logger = Logger.getLogger(GatewayStateManager.class);

    // Per-market order book cache
    private final ConcurrentHashMap<Integer, GatewayOrderBook> orderBooksByMarket = new ConcurrentHashMap<>();
    private final TradeRingBuffer trades = new TradeRingBuffer();

    // Per-market ticker stats (accumulated from trades)
    private final ConcurrentHashMap<Integer, TickerStats> tickerStatsByMarket = new ConcurrentHashMap<>();

    // Candle aggregation (concrete type: persistence hydration seeds the rings at startup)
    private final InMemoryCandleProvider candleProvider = new InMemoryCandleProvider();

    // Reference to WebSocket for broadcasting to clients
    private volatile MarketDataWebSocket webSocket;

    // Candle mutation and CANDLE_UPDATE publication share one order across the
    // egress and heartbeat threads, so an idle update cannot overwrite a real trade.
    private final Object candleUpdateLock = new Object();

    // Last candle bucket emitted per market and interval. Guarded by candleUpdateLock.
    private final long[][] lastIdleCandleBucket = new long[6][InMemoryCandleProvider.INTERVALS.length];
    private final boolean[][] idleHeartbeatInitialized = new boolean[6][InMemoryCandleProvider.INTERVALS.length];
    private static final int MAX_IDLE_CATCH_UP_BUCKETS = 500;

    // Market-data persistence (TimescaleDB). Null when disabled by env — every
    // path below then collapses to the pre-persistence in-memory behavior.
    private volatile com.match.infrastructure.gateway.persistence.MarketDataPersistence persistence;

    // Count of book-version chain breaks (a delta's fromVersion != our version — we missed an
    // update). match#96: the delta is DROPPED and the market's book flagged stale until a
    // resnapshot re-anchors it. Exposed as gateway_chain_breaks_total.
    private final java.util.concurrent.atomic.AtomicLong chainBreaks = new java.util.concurrent.atomic.AtomicLong();

    // Count of stale/duplicate book deltas dropped (switchover-seam monotonic dedup, match#19) — observability.
    private final java.util.concurrent.atomic.AtomicLong staleDeltasDropped = new java.util.concurrent.atomic.AtomicLong(0);

    // match#96: deltas dropped because the target book was already stale (a prior chain break,
    // awaiting a resnapshot). Applying a delta onto a stale base is the match#96 drift bug, so we
    // drop until a snapshot re-anchors. Kept distinct from the match#19 monotonic-dedup counter
    // above so the two causes stay separable in metrics. Exposed as gateway_deltas_dropped_stale_total.
    private final java.util.concurrent.atomic.AtomicLong deltasDroppedWhileStale = new java.util.concurrent.atomic.AtomicLong(0);

    // Count of stale/duplicate book snapshots dropped (switchover-seam dedup, match#95) — observability.
    private final java.util.concurrent.atomic.AtomicLong staleSnapshotsDropped = new java.util.concurrent.atomic.AtomicLong(0);

    /** match#33: /metrics read. */
    public long getStaleDeltasDropped() { return staleDeltasDropped.get(); }

    /** match#95: /metrics read. */
    public long getStaleSnapshotsDropped() { return staleSnapshotsDropped.get(); }

    /** match#96: /metrics read (gateway_chain_breaks_total). */
    public long getChainBreaks() { return chainBreaks.get(); }

    /** match#96: /metrics read (gateway_deltas_dropped_stale_total). */
    public long getDeltasDroppedWhileStale() { return deltasDroppedWhileStale.get(); }

    /** match#96: markets whose local book is currently stale (gateway_books_stale gauge). */
    public long getStaleBookCount() {
        long n = 0;
        for (GatewayOrderBook book : orderBooksByMarket.values()) {
            if (book.isStale()) {
                n++;
            }
        }
        return n;
    }

    public void setWebSocket(MarketDataWebSocket webSocket) {
        this.webSocket = webSocket;
    }

    public void setPersistence(com.match.infrastructure.gateway.persistence.MarketDataPersistence persistence) {
        this.persistence = persistence;
    }

    /** For /metrics (market_pg_* series). Null when persistence is disabled. */
    public com.match.infrastructure.gateway.persistence.MarketDataPersistence getPersistence() {
        return persistence;
    }

    /** Concrete provider accessor for startup hydration (ring seeding). */
    public InMemoryCandleProvider inMemoryCandleProvider() {
        return candleProvider;
    }

    @Override
    public void onBookSnapshot(BookSnapshotDecoder decoder) {
        try {
            int marketId = decoder.marketId();
            long timestamp = decoder.timestamp();
            long bidVersion = decoder.bidVersion();
            long askVersion = decoder.askVersion();
            long bookVersion = decoder.bookVersion();
            if (bookVersion == BookSnapshotDecoder.bookVersionNullValue()) {
                bookVersion = 0; // pre-v4 upstream (mixed-version rolling update)
            }

            GatewayOrderBook orderBook = getOrCreateOrderBook(marketId);

            // match#95: drop a stale/redelivered snapshot (old-leader or reordered egress at a
            // leader-switchover seam) whose book version does not advance the cached book — applying
            // it would rewind the displayed book and rebroadcast stale state as truth. Mirrors the
            // fromVersion chain-break gate below (onBookDelta) — guarded only when BOTH the incoming
            // message and the cached book carry a real (v4+) bookVersion, since a legacy pre-v4
            // upstream never advances bookVersion and dropping everything would freeze the book.
            if (bookVersion > 0 && orderBook.getVersion() > 0 && bookVersion <= orderBook.getVersion()) {
                long n = staleSnapshotsDropped.incrementAndGet();
                logger.debug("match#95 dedup: dropped " + n + " stale/duplicate book snapshot(s); market="
                    + marketId + " incoming bookVersion=" + bookVersion + " have=" + orderBook.getVersion());
                return;
            }

            // Convert bids to JSON array (convert fixed-point to decimal)
            JsonArray bidsArray = new JsonArray();
            for (BookSnapshotDecoder.BidsDecoder bid : decoder.bids()) {
                JsonObject level = new JsonObject();
                level.addProperty("price", (double) bid.price() / SCALE_FACTOR);
                level.addProperty("quantity", (double) bid.quantity() / SCALE_FACTOR);
                level.addProperty("orderCount", bid.orderCount());
                bidsArray.add(level);
            }

            // Convert asks to JSON array (convert fixed-point to decimal)
            JsonArray asksArray = new JsonArray();
            for (BookSnapshotDecoder.AsksDecoder ask : decoder.asks()) {
                JsonObject level = new JsonObject();
                level.addProperty("price", (double) ask.price() / SCALE_FACTOR);
                level.addProperty("quantity", (double) ask.quantity() / SCALE_FACTOR);
                level.addProperty("orderCount", ask.orderCount());
                asksArray.add(level);
            }

            // Update local state (per-market order book)
            String marketName = getMarketName(marketId);
            long version = bookVersion > 0 ? bookVersion : Math.max(bidVersion, askVersion);
            // match#96: a snapshot re-anchors the chain and clears any stale flag (in update()).
            boolean wasStale = orderBook.isStale();
            orderBook.update(marketId, marketName, bidsArray, asksArray, bidVersion, askVersion, version, timestamp);
            if (wasStale) {
                logger.info("match#96: market=" + marketId + " book recovered — snapshot re-anchored"
                    + " chain at bookVersion=" + version + " (was stale)");
            }

            // Build JSON and broadcast to WebSocket
            if (webSocket != null) {
                String json = buildBookSnapshotJson(marketId, marketName, timestamp, bidVersion, askVersion, version, bidsArray, asksArray);
                webSocket.broadcastMarketData(json);
            }
        } catch (Exception e) {
            logger.error("Error processing BOOK_SNAPSHOT: " + e.getMessage());
        }
    }

    @Override
    public void onBookDelta(BookDeltaDecoder decoder) {
        try {
            int marketId = decoder.marketId();
            long timestamp = decoder.timestamp();
            long bidVersion = decoder.bidVersion();
            long askVersion = decoder.askVersion();
            long bookVersion = decoder.bookVersion();
            long fromVersion = decoder.fromVersion();
            if (bookVersion == BookDeltaDecoder.bookVersionNullValue()) {
                bookVersion = 0; // pre-v4 upstream
                fromVersion = 0;
            }
            String marketName = getMarketName(marketId);

            // Get the order book for this market
            GatewayOrderBook orderBook = getOrCreateOrderBook(marketId);

            // match#96: a book that broke its version chain (see the continuity gate below) becomes
            // authoritative again ONLY after a fresh BOOK_SNAPSHOT re-anchors it — never via deltas.
            // Applying a delta onto a known-stale base is exactly the drift bug (findPriceIndex
            // silently no-ops UPDATE/DELETE of levels we never saw). Drop it, and count it distinctly
            // so the chainBreaks counter tracks distinct break events rather than every delta while stale.
            if (orderBook.isStale()) {
                long n = deltasDroppedWhileStale.incrementAndGet();
                if (n == 1 || n % 50 == 0) {
                    logger.info("match#96: dropped " + n + " book delta(s) while market=" + marketId
                        + " book is stale (awaiting resnapshot to re-anchor)");
                }
                return;
            }

            // match#19: drop stale/duplicate deltas (old-leader or re-delivered egress across a
            // leader-switchover seam) — they advance neither side's monotonic version, so applying
            // (and forwarding) them would corrupt the displayed book / double-apply. Checked BEFORE
            // the chain-continuity gate below so a benign redelivery (whose fromVersion trivially
            // mismatches our version) is deduped, NOT mistaken for a real forward gap and flagged stale.
            if (orderBook.isStaleUpdate(bidVersion, askVersion)) {
                long n = staleDeltasDropped.incrementAndGet();
                if (n == 1 || n % 50 == 0) {
                    logger.info("match#19 dedup: dropped " + n + " stale/duplicate book delta(s); market="
                        + marketId + " incoming v=(" + bidVersion + "," + askVersion + ")");
                }
                return;
            }

            // v4 chain continuity: every delta names the published state it extends. A mismatch that
            // survives the dedup above means this consumer genuinely missed a forward update (a
            // dropped/reordered delta on the egress path). match#96: applying it anyway silently
            // drifts the book and serves it as clean until the next snapshot — so DROP the delta and
            // flag the book stale. The next BOOK_SNAPSHOT that passes the match#95 monotonic guard
            // re-anchors the chain and clears the flag. There is no gateway-side per-market resnapshot
            // request path (the resnapshot generation is bumped cluster-side on session open / leader
            // promotion), so recovery rides the cluster's next resnapshot rather than a new command.
            if (fromVersion > 0 && orderBook.getVersion() > 0 && fromVersion != orderBook.getVersion()) {
                long n = chainBreaks.incrementAndGet();
                // ERROR, not warn: the default log level swallows warns, and this line is the
                // ONLY evidence for diagnosing systematic breaks (observed 2026-07-08: exactly
                // one break per market right after the initial snapshot). Includes both sides'
                // versions so the mismatch shape is readable straight from the log.
                logger.error("book-version chain break #" + n + ": market=" + marketId
                    + " have=" + orderBook.getVersion()
                    + " (bidV=" + orderBook.getBidVersion() + " askV=" + orderBook.getAskVersion() + ")"
                    + " delta from=" + fromVersion + " to=" + bookVersion
                    + " (deltaBidV=" + bidVersion + " deltaAskV=" + askVersion + ")"
                    + " - dropping delta, book marked stale until resnapshot");
                orderBook.markStale(orderBook.getVersion());
                return;
            }

            // Convert changes to JSON array
            JsonArray changesArray = new JsonArray();
            for (BookDeltaDecoder.ChangesDecoder change : decoder.changes()) {
                JsonObject c = new JsonObject();
                c.addProperty("price", (double) change.price() / SCALE_FACTOR);
                c.addProperty("quantity", (double) change.quantity() / SCALE_FACTOR);
                c.addProperty("orderCount", change.orderCount());
                c.addProperty("side", change.side().name());
                c.addProperty("updateType", change.updateType().name());
                changesArray.add(c);

                // Apply delta to this market's order book
                orderBook.applyDelta(
                    change.side().name(),
                    (double) change.price() / SCALE_FACTOR,
                    (double) change.quantity() / SCALE_FACTOR,
                    change.orderCount(),
                    change.updateType().name()
                );
            }

            // Update versions on this market's order book
            orderBook.updateVersions(marketId, marketName, bidVersion, askVersion, bookVersion, timestamp);

            // Build JSON and broadcast to WebSocket
            if (webSocket != null && !changesArray.isEmpty()) {
                String json = buildBookDeltaJson(marketId, marketName, timestamp, bidVersion, askVersion,
                        bookVersion, fromVersion, changesArray);
                webSocket.broadcastMarketData(json);
            }
        } catch (Exception e) {
            logger.error("Error processing BOOK_DELTA: " + e.getMessage());
        }
    }

    @Override
    public void onTradesBatch(TradesBatchDecoder decoder) {
        try {
            int marketId = decoder.marketId();
            long timestamp = decoder.timestamp();
            String marketName = getMarketName(marketId);

            // Get or create ticker stats for this market
            TickerStats tickerStats = getOrCreateTickerStats(marketId, marketName);

            // Convert trades to JSON array (convert fixed-point to decimal)
            com.match.infrastructure.gateway.persistence.MarketDataPersistence p = persistence;
            JsonArray tradesArray = new JsonArray();
            for (TradesBatchDecoder.TradesDecoder trade : decoder.trades()) {
                double price = (double) trade.price() / SCALE_FACTOR;
                double quantity = (double) trade.quantity() / SCALE_FACTOR;
                OrderSide takerSide = trade.takerSide();
                Boolean takerIsBuy;
                if (takerSide == OrderSide.BID) {
                    takerIsBuy = Boolean.TRUE;
                } else if (takerSide == OrderSide.ASK) {
                    takerIsBuy = Boolean.FALSE;
                } else {
                    takerIsBuy = null; // pre-v5 upstream (mixed-version rolling update): NULL_VAL
                }

                // Update ticker stats from each trade
                tickerStats.updateFromTrade(price, quantity, trade.tradeCount());

                // Persist the raw trade stream (source of truth for candles).
                // Lock-free enqueue; the writer thread does the JDBC work.
                if (p != null) {
                    p.writer().offer(marketId, price, quantity, trade.tradeCount(), trade.timestamp(), takerIsBuy);
                }

                JsonObject t = new JsonObject();
                t.addProperty("price", price);
                t.addProperty("quantity", quantity);
                t.addProperty("tradeCount", trade.tradeCount());
                t.addProperty("timestamp", trade.timestamp());
                if (takerIsBuy != null) {
                    t.addProperty("side", takerIsBuy ? "BUY" : "SELL");
                }
                tradesArray.add(t);
            }

            // Update local state
            trades.addBatch(marketId, marketName, tradesArray);

            // Build JSON and broadcast to WebSocket
            if (webSocket != null && !tradesArray.isEmpty()) {
                String json = buildTradesBatchJson(marketId, marketName, timestamp, tradesArray);
                webSocket.broadcastMarketData(json);

                // Also broadcast updated ticker stats
                String tickerJson = tickerStats.toJson();
                webSocket.broadcastMarketData(tickerJson);

            }

            // Update and publish candles under the same lock used by the idle
            // heartbeat. This preserves publication order when both threads
            // target the same bucket.
            synchronized (candleUpdateLock) {
                for (int i = 0; i < tradesArray.size(); i++) {
                    JsonObject t = tradesArray.get(i).getAsJsonObject();
                    candleProvider.onTrade(
                        marketId,
                        t.get("price").getAsDouble(),
                        t.get("quantity").getAsDouble(),
                        t.get("tradeCount").getAsInt(),
                        t.get("timestamp").getAsLong()
                    );
                }
                updatePriceChangePercent1h(marketId, tickerStats);

                MarketDataWebSocket target = webSocket;
                if (target != null && !tradesArray.isEmpty()) {
                    // Every interval is already maintained by the shared candle
                    // provider, so publish each current bucket from the same source.
                    int intervalIndex = 0;
                    for (String interval : InMemoryCandleProvider.INTERVALS) {
                        Candle currentCandle = candleProvider.getCurrentCandle(marketId, interval);
                        if (currentCandle != null) {
                            String candleJson = buildCandleUpdateJson(
                                    marketId, marketName, interval, currentCandle);
                            target.broadcastMarketData(candleJson);
                            lastIdleCandleBucket[marketId][intervalIndex] = Math.max(
                                    lastIdleCandleBucket[marketId][intervalIndex], currentCandle.time);
                        }
                        intervalIndex++;
                    }
                }
            }
        } catch (Exception e) {
            logger.error("Error processing TRADES_BATCH: " + e.getMessage());
        }
    }

    /**
     * Get or create ticker stats for a market.
     */
    private TickerStats getOrCreateTickerStats(int marketId, String marketName) {
        return tickerStatsByMarket.computeIfAbsent(marketId, k -> new TickerStats(marketId, marketName));
    }

    /** Compute the one-hour percentage move from the last two hourly candles. */
    private void updatePriceChangePercent1h(int marketId, TickerStats tickerStats) {
        List<Candle> hourly = candleProvider.getCandles(marketId, "1h", 2);
        if (hourly.isEmpty() || tickerStats.getLastPrice() <= 0) {
            return;
        }
        Candle reference = hourly.size() >= 2 ? hourly.get(hourly.size() - 2) : hourly.get(0);
        double base = hourly.size() >= 2 ? reference.close : reference.open;
        if (base > 0) {
            tickerStats.updatePriceChangePercent1h((tickerStats.getLastPrice() - base) / base * 100.0);
        }
    }

    /**
     * Get or create order book for a market.
     */
    private GatewayOrderBook getOrCreateOrderBook(int marketId) {
        return orderBooksByMarket.computeIfAbsent(marketId, k -> new GatewayOrderBook());
    }

    @Override
    public void onOrderStatusBatch(OrderStatusBatchDecoder decoder) {
        // Deliberate no-op: per-user order data now flows only via the OMS's
        // user-scoped WebSocket. Broadcasting every participant's orders on
        // the public market feed was a privacy leak, and the trading UI no
        // longer consumes it. The batch still arrives on this egress (the OMS
        // consumes it on its own session), so we just decode-and-drop here.
    }

    @Override
    public void onNewLeader(int leaderMemberId, long leadershipTermId) {
        // Leader change - could clear state if needed, but current approach is fine
        // State will be updated from next egress messages
        logger.info("New leader detected: member=" + leaderMemberId + ", term=" + leadershipTermId);
    }

    private String getMarketName(int marketId) {
        String displayName = MarketDisplay.symbol(marketId);
        if (displayName != null) {
            return displayName;
        }
        MarketInfo info = MarketInfo.fromId(marketId);
        return info != null ? info.symbol() : "UNKNOWN";
    }

    // Build JSON for WebSocket broadcast - book snapshot
    private String buildBookSnapshotJson(int marketId, String market, long timestamp,
            long bidVersion, long askVersion, long bookVersion, JsonArray bids, JsonArray asks) {
        JsonObject obj = new JsonObject();
        obj.addProperty("type", "BOOK_SNAPSHOT");
        obj.addProperty("marketId", marketId);
        obj.addProperty("market", market);
        obj.addProperty("timestamp", timestamp);
        obj.addProperty("bidVersion", bidVersion);
        obj.addProperty("askVersion", askVersion);
        // v4: the single monotonic book version this snapshot represents.
        obj.addProperty("bookVersion", bookVersion);
        obj.addProperty("version", bookVersion);
        obj.add("bids", bids);
        obj.add("asks", asks);
        return obj.toString();
    }

    // Build JSON for WebSocket broadcast - trades batch
    private String buildTradesBatchJson(int marketId, String market, long timestamp, JsonArray trades) {
        JsonObject obj = new JsonObject();
        obj.addProperty("type", "TRADES_BATCH");
        obj.addProperty("marketId", marketId);
        obj.addProperty("market", market);
        obj.addProperty("timestamp", timestamp);
        obj.add("trades", trades);
        return obj.toString();
    }

    // Build JSON for WebSocket broadcast - book delta
    private String buildBookDeltaJson(int marketId, String market, long timestamp,
            long bidVersion, long askVersion, long bookVersion, long fromVersion, JsonArray changes) {
        JsonObject obj = new JsonObject();
        obj.addProperty("type", "BOOK_DELTA");
        obj.addProperty("marketId", marketId);
        obj.addProperty("market", market);
        obj.addProperty("timestamp", timestamp);
        obj.addProperty("bidVersion", bidVersion);
        obj.addProperty("askVersion", askVersion);
        // v4 chain: this delta advances the book fromVersion -> bookVersion;
        // a client whose current bookVersion != fromVersion missed an update
        // and should re-snapshot (action:refresh).
        obj.addProperty("bookVersion", bookVersion);
        obj.addProperty("fromVersion", fromVersion);
        obj.add("changes", changes);
        return obj.toString();
    }

    // Build JSON for WebSocket broadcast - candle update
    private String buildCandleUpdateJson(int marketId, String market, String interval, Candle candle) {
        JsonObject obj = new JsonObject();
        obj.addProperty("type", "CANDLE_UPDATE");
        obj.addProperty("marketId", marketId);
        obj.addProperty("market", market);
        obj.addProperty("interval", interval);
        obj.add("candle", candleToJson(candle));
        return obj.toString();
    }

    /** Emit zero-activity candles for elapsed buckets, bounded after scheduler delays. */
    public int broadcastIdleCandleUpdates(long nowMs) {
        synchronized (candleUpdateLock) {
            MarketDataWebSocket target = webSocket;
            if (target == null || nowMs < 0) {
                return 0;
            }

            int emitted = 0;
            long nowSec = nowMs / 1_000L;
            for (int marketId = 1; marketId <= 3; marketId++) {
                String market = getMarketName(marketId);
                for (int i = 0; i < InMemoryCandleProvider.INTERVALS.length; i++) {
                    String interval = InMemoryCandleProvider.INTERVALS[i];
                    long intervalSec = CandleTimeline.intervalSeconds(interval);
                    long currentBucket = (nowSec / intervalSec) * intervalSec;
                    Candle latest = candleProvider.getCurrentCandle(marketId, interval);
                    if (latest == null) {
                        continue;
                    }

                    long nextBucket;
                    if (!idleHeartbeatInitialized[marketId][i]) {
                        idleHeartbeatInitialized[marketId][i] = true;
                        nextBucket = currentBucket;
                    } else {
                        nextBucket = lastIdleCandleBucket[marketId][i] + intervalSec;
                    }

                    if (latest.time >= currentBucket || nextBucket > currentBucket) {
                        lastIdleCandleBucket[marketId][i] = Math.max(
                                lastIdleCandleBucket[marketId][i], latest.time);
                        continue;
                    }

                    long firstAllowed = currentBucket
                            - (long) (MAX_IDLE_CATCH_UP_BUCKETS - 1) * intervalSec;
                    nextBucket = Math.max(nextBucket, firstAllowed);
                    while (nextBucket <= currentBucket) {
                        Candle empty = CandleTimeline.emptyCandle(marketId, nextBucket, latest.close);
                        target.broadcastMarketData(buildCandleUpdateJson(marketId, market, interval, empty));
                        lastIdleCandleBucket[marketId][i] = nextBucket;
                        emitted++;
                        nextBucket += intervalSec;
                    }
                }
            }
            return emitted;
        }
    }

    /**
     * Build candle history JSON from the in-memory rings (fallback path and
     * pre-persistence behavior; the DB-first path is buildCandleHistoryJsonAsync).
     */
    public String buildCandleHistoryJson(int marketId, String interval, int limit) {
        return buildCandleHistoryJson(marketId, interval, limit, 0);
    }

    public String buildCandleHistoryJson(int marketId, String interval, int limit,
                                         long beforeExclusiveSec) {
        long effectiveBefore = beforeExclusiveSec > 0
                ? beforeExclusiveSec : System.currentTimeMillis() / 1_000L + 1L;
        java.util.List<Candle> actual = candleProvider.getCandles(marketId, interval, 500);
        java.util.List<Candle> candles = CandleTimeline.fill(
                actual, marketId, interval, limit, effectiveBefore);
        return candleHistoryJson(marketId, getMarketName(marketId), interval, candles);
    }

    /** Build the dYdX-compatible ONE_DAY sparkline response from in-memory candles. */
    public String buildSparklinesJson(String timePeriod) {
        if (!"ONE_DAY".equals(timePeriod)) {
            throw new IllegalArgumentException("Unsupported timePeriod: " + timePeriod);
        }

        Map<String, List<Candle>> candlesByMarket = new LinkedHashMap<>();
        for (int marketId = 1; marketId <= 3; marketId++) {
            candlesByMarket.put(MarketDisplay.symbol(marketId),
                    candleProvider.getCandles(marketId, "1h", 24));
        }
        return sparklinesJson(candlesByMarket);
    }

    /** Shared serializer for DB and in-memory sparkline data. */
    public static String sparklinesJson(Map<String, List<Candle>> candlesByMarket) {
        JsonObject result = new JsonObject();
        for (Map.Entry<String, List<Candle>> entry : candlesByMarket.entrySet()) {
            JsonArray prices = new JsonArray();
            for (Candle candle : entry.getValue()) {
                prices.add(Double.toString(candle.close));
            }
            result.add(entry.getKey(), prices);
        }
        return result.toString();
    }

    /**
     * Shared CANDLE_HISTORY envelope — the DB path and the memory path must
     * emit the identical shape, so both build through here.
     */
    public static String candleHistoryJson(int marketId, String market, String interval,
                                           java.util.List<Candle> candles) {
        JsonObject obj = new JsonObject();
        obj.addProperty("type", "CANDLE_HISTORY");
        obj.addProperty("marketId", marketId);
        obj.addProperty("market", market);
        obj.addProperty("interval", interval);

        JsonArray arr = new JsonArray();
        for (Candle c : candles) {
            arr.add(candleToJson(c));
        }
        obj.add("candles", arr);
        return obj.toString();
    }

    /**
     * Shared TRADES_BATCH envelope for the persisted trade tape — mirrors
     * TradeRingBuffer.toJsonForMarket: "side" ("BUY"/"SELL") is emitted only
     * when the row carries a taker side (rows persisted before the cluster
     * carried takerSide have a NULL taker_side and omit it).
     */
    public static String tradesBatchJson(int marketId, String market,
                                         java.util.List<com.match.infrastructure.gateway.persistence.TimescaleReader.TapeRow> rows) {
        JsonObject json = new JsonObject();
        json.addProperty("type", "TRADES_BATCH");
        json.addProperty("marketId", marketId);
        json.addProperty("market", market);
        json.addProperty("timestamp", System.currentTimeMillis());

        JsonArray tradesArray = new JsonArray();
        for (com.match.infrastructure.gateway.persistence.TimescaleReader.TapeRow t : rows) {
            JsonObject obj = new JsonObject();
            obj.addProperty("price", t.price());
            obj.addProperty("quantity", t.quantity());
            obj.addProperty("tradeCount", t.tradeCount());
            if (t.takerIsBuy() != null) {
                obj.addProperty("side", t.takerIsBuy() ? "BUY" : "SELL");
            }
            obj.addProperty("timestamp", t.tsMs());
            tradesArray.add(obj);
        }
        json.add("trades", tradesArray);
        return json.toString();
    }

    /**
     * DB-first candle history (the database is the source of truth); falls
     * back to the in-memory rings when persistence is disabled or down.
     * Completed on a persistence read thread — never blocks the caller.
     */
    public java.util.concurrent.CompletableFuture<String> buildCandleHistoryJsonAsync(
            int marketId, String interval, int limit) {
        return buildCandleHistoryJsonAsync(marketId, interval, limit, 0);
    }

    public java.util.concurrent.CompletableFuture<String> buildCandleHistoryJsonAsync(
            int marketId, String interval, int limit, long beforeExclusiveSec) {
        com.match.infrastructure.gateway.persistence.MarketDataPersistence p = persistence;
        if (p == null) {
            return java.util.concurrent.CompletableFuture.completedFuture(
                    buildCandleHistoryJson(marketId, interval, limit, beforeExclusiveSec));
        }
        return p.reads().candleHistoryJson(marketId, interval, limit, beforeExclusiveSec,
                () -> buildCandleHistoryJson(marketId, interval, limit, beforeExclusiveSec));
    }

    /** DB-first dYdX-compatible sparkline data with in-memory fallback. */
    public java.util.concurrent.CompletableFuture<String> buildSparklinesJsonAsync(String timePeriod) {
        if (!"ONE_DAY".equals(timePeriod)) {
            return java.util.concurrent.CompletableFuture.failedFuture(
                    new IllegalArgumentException("Unsupported timePeriod: " + timePeriod));
        }
        com.match.infrastructure.gateway.persistence.MarketDataPersistence p = persistence;
        if (p == null) {
            return java.util.concurrent.CompletableFuture.completedFuture(
                    buildSparklinesJson(timePeriod));
        }
        return p.reads().sparklineJson(timePeriod, () -> buildSparklinesJson(timePeriod));
    }

    /** DB-first recent-trades tape with in-memory fallback. marketId 0 = all markets. */
    public java.util.concurrent.CompletableFuture<String> recentTradesJsonAsync(int limit, int marketId) {
        com.match.infrastructure.gateway.persistence.MarketDataPersistence p = persistence;
        if (p == null) {
            return java.util.concurrent.CompletableFuture.completedFuture(
                    trades.toJsonForMarket(limit, marketId));
        }
        return p.reads().recentTradesJson(limit, marketId,
                () -> trades.toJsonForMarket(limit, marketId));
    }

    /** Push a rolling-24h baseline into a market's ticker (persistence ticker thread). */
    public void applyTickerBaseline(int marketId,
                                    com.match.infrastructure.gateway.persistence.TickerBaseline b) {
        TickerStats tickerStats = getOrCreateTickerStats(marketId, getMarketName(marketId));
        tickerStats
                .applyDbBaseline(b.open24h(), b.high24h(), b.low24h(),
                        b.quoteVolume24h(), b.lastClose(), b.tradeCount24h(), b.asOfMs());
        updatePriceChangePercent1h(marketId, tickerStats);
    }

    private static JsonObject candleToJson(Candle c) {
        JsonObject obj = new JsonObject();
        obj.addProperty("time", c.time);
        obj.addProperty("open", c.open);
        obj.addProperty("high", c.high);
        obj.addProperty("low", c.low);
        obj.addProperty("close", c.close);
        obj.addProperty("volume", c.volume);
        obj.addProperty("tradeCount", c.tradeCount);
        return obj;
    }

    // State accessors for HTTP and WebSocket handlers

    /**
     * Get order book for a specific market.
     * Returns null if no data exists for this market.
     */
    public GatewayOrderBook getOrderBook(int marketId) {
        return orderBooksByMarket.get(marketId);
    }

    public TradeRingBuffer getTrades() {
        return trades;
    }

    /**
     * Get initial state JSON for new WebSocket subscribers.
     * Returns the order book snapshot for the specified market.
     */
    public String getInitialBookSnapshot(int marketId) {
        GatewayOrderBook orderBook = orderBooksByMarket.get(marketId);
        return orderBook != null ? orderBook.toJson() : null;
    }

    /**
     * Get recent trades JSON for new WebSocket subscribers.
     */
    public String getRecentTradesJson(int limit) {
        return trades.toJson(limit);
    }

    /**
     * Get ticker stats for a specific market.
     * Returns null if no trades have been received for this market.
     */
    public String getTickerStats(int marketId) {
        TickerStats stats = tickerStatsByMarket.get(marketId);
        return stats != null ? stats.toJson() : null;
    }

    /** Update a market's externally sourced guide price without touching trade statistics. */
    public void updateExternalIndexPrice(int marketId, double price) {
        if (marketId < 1 || marketId > 3 || !Double.isFinite(price) || price <= 0) {
            return;
        }
        getOrCreateTickerStats(marketId, getMarketName(marketId)).updateIndexPrice(price);
    }

    /** Stable ticker snapshot for public subscriptions, including empty markets. */
    public String getTickerStatsOrEmpty(int marketId) {
        if (marketId < 1 || marketId > 3) {
            return null;
        }
        TickerStats stats = tickerStatsByMarket.computeIfAbsent(
                marketId, id -> new TickerStats(id, MarketDisplay.symbolOrUnknown(id)));
        return stats.toJson();
    }

    /**
     * Get 24-hour ticker statistics for every configured market.
     *
     * <p>The list is built from {@link MarketInfo#ALL}, rather than from the
     * markets that have already traded, so the frontend always receives a
     * stable row for every configured trading pair, including empty markets.
     */
    public String getAllTickerStats() {
        JsonArray result = new JsonArray();
        for (int marketId = 1; marketId <= 3; marketId++) {
            TickerStats stats = tickerStatsByMarket.computeIfAbsent(
                    marketId, id -> new TickerStats(id, MarketDisplay.symbolOrUnknown(id)));
            JsonObject ticker = JsonParser.parseString(stats.toJson()).getAsJsonObject();

            ticker.remove("type");
            ticker.add("priceChange24h", ticker.remove("priceChange"));
            ticker.add("priceChangePercent24h", ticker.remove("priceChangePercent"));
            result.add(ticker);
        }
        return result.toString();
    }

    /**
     * Get the candle provider for REST/WebSocket queries.
     */
    public CandleProvider getCandleProvider() {
        return candleProvider;
    }
}

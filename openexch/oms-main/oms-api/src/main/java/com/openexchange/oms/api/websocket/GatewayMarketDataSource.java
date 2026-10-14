// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.websocket;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.WebSocket;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionException;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.Consumer;
import java.util.stream.IntStream;

/** Bridges the existing match-gateway REST and WebSocket APIs into OMS. */
public final class GatewayMarketDataSource implements MarketDataSource {

    private static final Logger log = LoggerFactory.getLogger(GatewayMarketDataSource.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final Set<String> PUBLIC_TYPES = Set.of(
            "BOOK_SNAPSHOT", "BOOK_DELTA", "TRADES_BATCH", "TICKER_STATS",
            "CANDLE_HISTORY", "CANDLE_UPDATE", "CLUSTER_STATUS", "CLUSTER_EVENT");
    private static final int MAX_RESPONSE_BYTES = 2 * 1024 * 1024;
    private static final int MAX_RECONNECT_BUFFER = 512;
    private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(5);

    private final String baseUrl;
    private final URI webSocketUri;
    private final HttpClient client;
    private final ScheduledExecutorService scheduler;
    private final Duration initialReconnectDelay;
    private final Duration maxReconnectDelay;
    private final ConcurrentHashMap<Integer, Upstream> upstreams = new ConcurrentHashMap<>();
    private final AtomicBoolean closed = new AtomicBoolean();

    private volatile Consumer<String> liveFrameConsumer = ignored -> { };

    public GatewayMarketDataSource(String baseUrl) {
        this(baseUrl, Duration.ofSeconds(1), Duration.ofSeconds(30));
    }

    GatewayMarketDataSource(String baseUrl, Duration initialReconnectDelay, Duration maxReconnectDelay) {
        this.baseUrl = normalizeBaseUrl(baseUrl);
        this.webSocketUri = toWebSocketUri(this.baseUrl);
        this.initialReconnectDelay = requirePositive(initialReconnectDelay, "initialReconnectDelay");
        this.maxReconnectDelay = requirePositive(maxReconnectDelay, "maxReconnectDelay");
        this.client = HttpClient.newBuilder().connectTimeout(REQUEST_TIMEOUT).build();
        this.scheduler = Executors.newSingleThreadScheduledExecutor(runnable -> {
            Thread thread = new Thread(runnable, "oms-market-gateway-reconnect");
            thread.setDaemon(true);
            return thread;
        });
    }

    @Override
    public synchronized void start(IntStream marketIds, Consumer<String> liveFrameConsumer) {
        if (closed.get()) throw new IllegalStateException("Market data source is closed");
        this.liveFrameConsumer = java.util.Objects.requireNonNull(liveFrameConsumer, "liveFrameConsumer");
        marketIds.filter(marketId -> marketId > 0).distinct().forEach(marketId ->
                upstreams.computeIfAbsent(marketId, id -> {
                    Upstream upstream = new Upstream(id);
                    connect(upstream);
                    return upstream;
                }));
    }

    @Override
    public CompletableFuture<List<String>> loadInitialFrames(
            int marketId, String interval, int tradeLimit, int candleLimit) {
        if (marketId <= 0) return CompletableFuture.failedFuture(new IllegalArgumentException("marketId"));
        if (interval == null || !interval.matches("[A-Za-z0-9]+")) {
            return CompletableFuture.failedFuture(new IllegalArgumentException("interval"));
        }
        int safeTradeLimit = Math.max(1, Math.min(tradeLimit, 500));
        int safeCandleLimit = Math.max(1, Math.min(candleLimit, 500));
        CompletableFuture<String> book = requestText("/api/orderbook?marketId=" + marketId);
        CompletableFuture<String> trades = requestText(
                "/api/trades?marketId=" + marketId + "&limit=" + safeTradeLimit);
        CompletableFuture<String> candles = requestText(
                "/api/candles?marketId=" + marketId + "&interval=" + interval
                        + "&limit=" + safeCandleLimit);
        return CompletableFuture.allOf(book, trades, candles)
                .thenApply(ignored -> List.of(book.join(), trades.join(), candles.join()));
    }

    private void connect(Upstream upstream) {
        if (closed.get()) return;
        upstream.reconnectScheduled.set(false);
        synchronized (upstream) {
            upstream.ready = false;
            upstream.buffer.clear();
        }
        client.newWebSocketBuilder()
                .connectTimeout(REQUEST_TIMEOUT)
                .buildAsync(webSocketUri, new GatewayListener(upstream))
                .whenComplete((socket, error) -> {
                    if (error != null) {
                        scheduleReconnect(upstream);
                    } else if (closed.get()) {
                        socket.abort();
                    }
                });
    }

    private void anchorAndRelease(Upstream upstream, WebSocket socket) {
        requestText("/api/orderbook?marketId=" + upstream.marketId)
                .whenComplete((snapshot, error) -> {
                    if (closed.get() || upstream.socket != socket) return;
                    if (error != null) {
                        log.warn("Could not re-anchor market {} after gateway connect", upstream.marketId, error);
                        socket.abort();
                        scheduleReconnect(upstream);
                        return;
                    }
                    liveFrameConsumer.accept(snapshot);
                    List<String> buffered;
                    synchronized (upstream) {
                        upstream.ready = true;
                        buffered = new ArrayList<>(upstream.buffer);
                        upstream.buffer.clear();
                    }
                    buffered.forEach(liveFrameConsumer);
                });
    }

    private void acceptFrame(Upstream upstream, String frame) {
        JsonNode root;
        try {
            root = MAPPER.readTree(frame);
        } catch (IOException ignored) {
            return;
        }
        String type = root.path("type").asText("");
        if (!PUBLIC_TYPES.contains(type)) return;
        int marketId = root.path("marketId").asInt(0);
        if (marketId > 0 && marketId != upstream.marketId) return;
        synchronized (upstream) {
            if (!upstream.ready) {
                if (upstream.buffer.size() == MAX_RECONNECT_BUFFER) upstream.buffer.removeFirst();
                upstream.buffer.addLast(frame);
                return;
            }
        }
        liveFrameConsumer.accept(frame);
    }

    private CompletableFuture<String> requestText(String pathAndQuery) {
        HttpRequest request = HttpRequest.newBuilder(URI.create(baseUrl + pathAndQuery))
                .timeout(REQUEST_TIMEOUT)
                .GET()
                .build();
        return client.sendAsync(request, HttpResponse.BodyHandlers.ofInputStream())
                .thenApply(response -> readResponse(pathAndQuery, response));
    }

    private static String readResponse(String path, HttpResponse<InputStream> response) {
        if (response.statusCode() < 200 || response.statusCode() >= 300) {
            try {
                response.body().close();
            } catch (IOException ignored) {
                // Nothing else to do while reporting the upstream status.
            }
            throw new CompletionException(new IOException(
                    "Market gateway returned HTTP " + response.statusCode() + " for " + path));
        }
        try (InputStream body = response.body()) {
            byte[] bytes = body.readNBytes(MAX_RESPONSE_BYTES + 1);
            if (bytes.length > MAX_RESPONSE_BYTES) {
                throw new IOException("Market gateway response exceeded " + MAX_RESPONSE_BYTES + " bytes");
            }
            return new String(bytes, StandardCharsets.UTF_8);
        } catch (IOException error) {
            throw new CompletionException(error);
        }
    }

    private void scheduleReconnect(Upstream upstream) {
        if (closed.get() || !upstream.reconnectScheduled.compareAndSet(false, true)) return;
        int attempt = Math.min(upstream.reconnectAttempts++, 20);
        long multiplier = 1L << Math.min(attempt, 20);
        long delay = Math.min(maxReconnectDelay.toMillis(), initialReconnectDelay.toMillis() * multiplier);
        ScheduledFuture<?> future = scheduler.schedule(() -> connect(upstream), delay, TimeUnit.MILLISECONDS);
        upstream.reconnectTask = future;
    }

    @Override
    public void close() {
        if (!closed.compareAndSet(false, true)) return;
        for (Upstream upstream : upstreams.values()) {
            ScheduledFuture<?> task = upstream.reconnectTask;
            if (task != null) task.cancel(false);
            WebSocket socket = upstream.socket;
            if (socket != null) socket.abort();
        }
        upstreams.clear();
        scheduler.shutdownNow();
    }

    private static String normalizeBaseUrl(String value) {
        if (value == null || value.isBlank()) throw new IllegalArgumentException("baseUrl");
        String normalized = value.trim();
        while (normalized.endsWith("/")) normalized = normalized.substring(0, normalized.length() - 1);
        URI uri = URI.create(normalized);
        if (!("http".equalsIgnoreCase(uri.getScheme()) || "https".equalsIgnoreCase(uri.getScheme()))
                || uri.getRawAuthority() == null || uri.getRawQuery() != null || uri.getRawFragment() != null) {
            throw new IllegalArgumentException("Market gateway URL must be an HTTP(S) origin");
        }
        return normalized;
    }

    private static URI toWebSocketUri(String baseUrl) {
        URI http = URI.create(baseUrl);
        String scheme = "https".equalsIgnoreCase(http.getScheme()) ? "wss" : "ws";
        String path = http.getRawPath();
        if (path == null || path.isEmpty() || "/".equals(path)) path = "";
        return URI.create(scheme + "://" + http.getRawAuthority() + path + "/ws");
    }

    private static Duration requirePositive(Duration value, String name) {
        if (value == null || value.isZero() || value.isNegative()) throw new IllegalArgumentException(name);
        return value;
    }

    private final class GatewayListener implements WebSocket.Listener {
        private final Upstream upstream;
        private final StringBuilder text = new StringBuilder();

        private GatewayListener(Upstream upstream) {
            this.upstream = upstream;
        }

        @Override
        public void onOpen(WebSocket webSocket) {
            upstream.socket = webSocket;
            upstream.reconnectAttempts = 0;
            webSocket.sendText(
                    "{\"action\":\"subscribe\",\"marketId\":" + upstream.marketId + "}", true);
            anchorAndRelease(upstream, webSocket);
            webSocket.request(1);
        }

        @Override
        public java.util.concurrent.CompletionStage<?> onText(
                WebSocket webSocket, CharSequence data, boolean last) {
            text.append(data);
            if (text.length() > MAX_RESPONSE_BYTES) {
                webSocket.abort();
                text.setLength(0);
                scheduleReconnect(upstream);
                return CompletableFuture.completedFuture(null);
            }
            if (last) {
                String frame = text.toString();
                text.setLength(0);
                acceptFrame(upstream, frame);
            }
            webSocket.request(1);
            return CompletableFuture.completedFuture(null);
        }

        @Override
        public java.util.concurrent.CompletionStage<?> onClose(
                WebSocket webSocket, int statusCode, String reason) {
            if (upstream.socket == webSocket) upstream.socket = null;
            scheduleReconnect(upstream);
            return CompletableFuture.completedFuture(null);
        }

        @Override
        public void onError(WebSocket webSocket, Throwable error) {
            if (upstream.socket == webSocket) upstream.socket = null;
            scheduleReconnect(upstream);
        }
    }

    private static final class Upstream {
        private final int marketId;
        private final ArrayDeque<String> buffer = new ArrayDeque<>();
        private final AtomicBoolean reconnectScheduled = new AtomicBoolean();
        private volatile WebSocket socket;
        private volatile ScheduledFuture<?> reconnectTask;
        private volatile int reconnectAttempts;
        private boolean ready;

        private Upstream(int marketId) {
            this.marketId = marketId;
        }
    }
}

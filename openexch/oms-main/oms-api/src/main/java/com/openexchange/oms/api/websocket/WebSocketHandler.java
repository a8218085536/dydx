// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.websocket;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.openexchange.oms.api.auth.Authorizer;
import com.openexchange.oms.api.auth.HttpAuthHandler;
import com.openexchange.oms.api.auth.Principal;
import com.openexchange.oms.api.auth.RoleBasedAuthorizer;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.handler.codec.http.websocketx.TextWebSocketFrame;
import io.netty.handler.codec.http.websocketx.WebSocketFrame;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletionException;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArraySet;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Unified WebSocket handler for public market data and private user updates.
 *
 * Public subscribe: { "op": "subscribe", "channels": ["orderbook","trades","candles"],
 * "marketId": 1, "interval": "1m" }. Private channels use the principal
 * attached during the upgrade and never accept a client-selected user ID.
 *
 * One shared instance is added to every upgrading channel's pipeline
 * (HttpServer), so the handler MUST be @Sharable — without it Netty refuses
 * the second channel's add and closes the socket mid-handshake, limiting the
 * user WS to ONE connection per OMS lifetime (oms#68). All state is already
 * share-safe: concurrent maps keyed by ctx/userId, no per-channel fields.
 */
@ChannelHandler.Sharable
public class WebSocketHandler extends SimpleChannelInboundHandler<WebSocketFrame> {

    private static final Logger log = LoggerFactory.getLogger(WebSocketHandler.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final Set<String> PUBLIC_CHANNELS = Set.of("orderbook", "trades", "candles");
    private static final Set<String> PRIVATE_CHANNELS = Set.of("orders", "executions", "balances");
    private static final Set<String> CANDLE_INTERVALS = Set.of("1m", "5m", "15m", "1h", "4h", "1d");
    private static final int MAX_INIT_BUFFER = 512;

    // userId → set of channels per connection
    private final Map<ChannelHandlerContext, Long> ctxToUserId = new ConcurrentHashMap<>();
    private final Map<ChannelHandlerContext, Set<String>> ctxToChannels = new ConcurrentHashMap<>();

    // userId → set of contexts (for broadcasting updates to all connections of a user)
    private final Map<Long, Set<ChannelHandlerContext>> userConnections = new ConcurrentHashMap<>();
    private final Map<ChannelHandlerContext, PublicSubscription> publicSubscriptions = new ConcurrentHashMap<>();

    private final Authorizer authorizer;
    private final MarketDataSource marketDataSource;
    private final Set<Integer> supportedMarkets;
    private final AtomicLong subscriptionGeneration = new AtomicLong();

    public WebSocketHandler() {
        this(new RoleBasedAuthorizer());
    }

    public WebSocketHandler(Authorizer authorizer) {
        this(authorizer, null, Set.of());
    }

    public WebSocketHandler(
            Authorizer authorizer, MarketDataSource marketDataSource, Set<Integer> supportedMarkets) {
        this.authorizer = authorizer;
        this.marketDataSource = marketDataSource;
        this.supportedMarkets = Set.copyOf(supportedMarkets);
        if (marketDataSource != null) {
            marketDataSource.start(this.supportedMarkets.stream().mapToInt(Integer::intValue), this::onMarketFrame);
        }
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, WebSocketFrame frame) throws Exception {
        if (frame instanceof TextWebSocketFrame textFrame) {
            handleTextMessage(ctx, textFrame.text());
        }
    }

    private void handleTextMessage(ChannelHandlerContext ctx, String text) {
        try {
            JsonNode msg = MAPPER.readTree(text);
            String op = msg.has("op") ? msg.get("op").asText() : "";

            if ("subscribe".equals(op)) {
                subscribe(ctx, msg);
            } else if ("unsubscribe".equals(op)) {
                removeConnection(ctx);
                sendType(ctx, "UNSUBSCRIBED");
            } else if ("ping".equals(op)) {
                // Browser WebSocket clients cannot send protocol-level pings;
                // this JSON ping gives them a liveness/keepalive signal (the
                // server itself sends no heartbeats).
                ObjectNode pong = MAPPER.createObjectNode();
                pong.put("type", "PONG");
                ctx.writeAndFlush(new TextWebSocketFrame(MAPPER.writeValueAsString(pong)));
            } else {
                sendError(ctx, "INVALID_OPERATION", "Unknown WebSocket operation");
            }
        } catch (Exception e) {
            log.warn("Invalid WebSocket message: {}", e.getMessage());
            sendError(ctx, "INVALID_MESSAGE", "Malformed WebSocket message");
        }
    }

    private void subscribe(ChannelHandlerContext ctx, JsonNode msg) throws Exception {
        if (!msg.path("channels").isArray()) {
            sendError(ctx, "INVALID_CHANNEL", "channels must be an array");
            return;
        }
        Set<String> requestedChannels = new HashSet<>();
        for (JsonNode channel : msg.path("channels")) requestedChannels.add(channel.asText());
        Set<String> unknown = new HashSet<>(requestedChannels);
        unknown.removeAll(PUBLIC_CHANNELS);
        unknown.removeAll(PRIVATE_CHANNELS);
        if (!unknown.isEmpty()) {
            sendError(ctx, "INVALID_CHANNEL", "Unsupported channel");
            return;
        }

        Set<String> publicChannels = new HashSet<>(requestedChannels);
        publicChannels.retainAll(PUBLIC_CHANNELS);
        Set<String> privateChannels = new HashSet<>(requestedChannels);
        privateChannels.retainAll(PRIVATE_CHANNELS);

        int marketId = msg.path("marketId").asInt(0);
        String interval = msg.path("interval").asText("1m");
        if (!publicChannels.isEmpty() && !supportedMarkets.contains(marketId)) {
            sendError(ctx, "INVALID_MARKET", "Unsupported market");
            return;
        }
        if (publicChannels.contains("candles") && !CANDLE_INTERVALS.contains(interval)) {
            sendError(ctx, "INVALID_INTERVAL", "Unsupported candle interval");
            return;
        }

        Principal principal = ctx.channel().attr(HttpAuthHandler.PRINCIPAL).get();
        if (!privateChannels.isEmpty() && principal == null) {
            sendError(ctx, "AUTH_REQUIRED", "Authentication is required for private channels");
            privateChannels.clear();
        }
        replacePrivateSubscription(ctx, principal, privateChannels);

        PublicSubscription subscription = null;
        if (!publicChannels.isEmpty()) {
            subscription = new PublicSubscription(
                    marketId, interval, Set.copyOf(publicChannels), subscriptionGeneration.incrementAndGet());
            publicSubscriptions.put(ctx, subscription);
        } else {
            publicSubscriptions.remove(ctx);
        }

        ObjectNode ack = MAPPER.createObjectNode();
        ack.put("type", "SUBSCRIBED");
        if (!publicChannels.isEmpty()) {
            ack.put("marketId", marketId);
            ack.put("interval", interval);
        }
        ctx.writeAndFlush(new TextWebSocketFrame(MAPPER.writeValueAsString(ack)));

        if (subscription != null) loadInitialState(ctx, subscription);
    }

    private void replacePrivateSubscription(
            ChannelHandlerContext ctx, Principal principal, Set<String> privateChannels) {
        Long previousUserId = ctxToUserId.remove(ctx);
        ctxToChannels.remove(ctx);
        if (previousUserId != null) {
            Set<ChannelHandlerContext> previous = userConnections.get(previousUserId);
            if (previous != null) {
                previous.remove(ctx);
                if (previous.isEmpty()) userConnections.remove(previousUserId, previous);
            }
        }
        if (principal == null || privateChannels.isEmpty()) return;
        long userId = principal.userId();
        ctxToUserId.put(ctx, userId);
        ctxToChannels.put(ctx, new CopyOnWriteArraySet<>(privateChannels));
        userConnections.computeIfAbsent(userId, ignored -> new CopyOnWriteArraySet<>()).add(ctx);
    }

    private void loadInitialState(ChannelHandlerContext ctx, PublicSubscription subscription) {
        if (marketDataSource == null) {
            publicSubscriptions.remove(ctx, subscription);
            sendError(ctx, "UPSTREAM_UNAVAILABLE", "Market data source is unavailable");
            return;
        }
        marketDataSource.loadInitialFrames(subscription.marketId, subscription.interval, 50, 200)
                .whenComplete((frames, error) -> ctx.executor().execute(() -> {
                    if (publicSubscriptions.get(ctx) != subscription || !ctx.channel().isActive()) return;
                    if (error != null) {
                        publicSubscriptions.remove(ctx, subscription);
                        sendError(ctx, "UPSTREAM_UNAVAILABLE", rootMessage(error));
                        return;
                    }
                    for (String frame : frames) {
                        PublicFrame parsed = parsePublicFrame(frame);
                        if (parsed != null && matches(subscription, parsed)) writeFrame(ctx, frame);
                    }
                    List<String> buffered;
                    synchronized (subscription) {
                        subscription.initializing = false;
                        buffered = new ArrayList<>(subscription.buffer);
                        subscription.buffer.clear();
                    }
                    buffered.forEach(frame -> writeFrame(ctx, frame));
                }));
    }

    private void onMarketFrame(String frame) {
        PublicFrame parsed = parsePublicFrame(frame);
        if (parsed == null) return;
        for (Map.Entry<ChannelHandlerContext, PublicSubscription> entry : publicSubscriptions.entrySet()) {
            ChannelHandlerContext ctx = entry.getKey();
            PublicSubscription subscription = entry.getValue();
            if (!matches(subscription, parsed)) continue;
            ctx.executor().execute(() -> deliverLive(ctx, subscription, frame));
        }
    }

    private void deliverLive(ChannelHandlerContext ctx, PublicSubscription subscription, String frame) {
        if (publicSubscriptions.get(ctx) != subscription || !ctx.channel().isActive()) return;
        synchronized (subscription) {
            if (subscription.initializing) {
                if (subscription.buffer.size() == MAX_INIT_BUFFER) subscription.buffer.removeFirst();
                subscription.buffer.addLast(frame);
                return;
            }
        }
        writeFrame(ctx, frame);
    }

    private static boolean matches(PublicSubscription subscription, PublicFrame frame) {
        if (frame.marketId > 0 && frame.marketId != subscription.marketId) return false;
        return switch (frame.type) {
            case "BOOK_SNAPSHOT", "BOOK_DELTA" -> subscription.channels.contains("orderbook");
            case "TRADES_BATCH" -> subscription.channels.contains("trades");
            case "CANDLE_HISTORY", "CANDLE_UPDATE" -> subscription.channels.contains("candles")
                    && (frame.interval.isEmpty() || frame.interval.equals(subscription.interval));
            case "TICKER_STATS", "CLUSTER_STATUS", "CLUSTER_EVENT" -> true;
            default -> false;
        };
    }

    private static PublicFrame parsePublicFrame(String frame) {
        try {
            JsonNode root = MAPPER.readTree(frame);
            return new PublicFrame(
                    root.path("type").asText(""),
                    root.path("marketId").asInt(0),
                    root.path("interval").asText(""));
        } catch (Exception ignored) {
            return null;
        }
    }

    private static void writeFrame(ChannelHandlerContext ctx, String frame) {
        ctx.writeAndFlush(new TextWebSocketFrame(frame));
    }

    private static void sendType(ChannelHandlerContext ctx, String type) {
        ObjectNode response = MAPPER.createObjectNode();
        response.put("type", type);
        writeFrame(ctx, response.toString());
    }

    private static void sendError(ChannelHandlerContext ctx, String code, String message) {
        ObjectNode error = MAPPER.createObjectNode();
        error.put("type", "ERROR");
        error.put("code", code);
        error.put("message", message == null || message.isBlank() ? code : message);
        writeFrame(ctx, error.toString());
    }

    private static String rootMessage(Throwable error) {
        Throwable current = error;
        while ((current instanceof CompletionException || current.getCause() != null)
                && current.getCause() != null) {
            current = current.getCause();
        }
        return current.getMessage() == null ? "Market data source is unavailable" : current.getMessage();
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) {
        removeConnection(ctx);
    }

    private void removeConnection(ChannelHandlerContext ctx) {
        publicSubscriptions.remove(ctx);
        Long userId = ctxToUserId.remove(ctx);
        ctxToChannels.remove(ctx);
        if (userId != null) {
            Set<ChannelHandlerContext> connections = userConnections.get(userId);
            if (connections != null) {
                connections.remove(ctx);
                if (connections.isEmpty()) {
                    userConnections.remove(userId);
                }
            }
        }
    }

    /**
     * Push an event to all WebSocket connections for a given user.
     */
    public void pushToUser(long userId, String channel, Object event) {
        Set<ChannelHandlerContext> connections = userConnections.get(userId);
        if (connections == null || connections.isEmpty()) return;

        try {
            String json = MAPPER.writeValueAsString(event);
            TextWebSocketFrame frame = new TextWebSocketFrame(json);

            for (ChannelHandlerContext ctx : connections) {
                Set<String> subscribedChannels = ctxToChannels.get(ctx);
                if (subscribedChannels != null && subscribedChannels.contains(channel)) {
                    if (ctx.channel().isActive()) {
                        ctx.writeAndFlush(frame.retainedDuplicate());
                    }
                }
            }
            frame.release();
        } catch (Exception e) {
            log.error("Error pushing WebSocket event to user {}", userId, e);
        }
    }

    public int getConnectionCount() {
        return ctxToUserId.size();
    }

    public void close() {
        if (marketDataSource != null) marketDataSource.close();
        publicSubscriptions.clear();
        ctxToUserId.clear();
        ctxToChannels.clear();
        userConnections.clear();
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
        log.warn("WebSocket error", cause);
        ctx.close();
    }

    private record PublicFrame(String type, int marketId, String interval) { }

    private static final class PublicSubscription {
        private final int marketId;
        private final String interval;
        private final Set<String> channels;
        @SuppressWarnings("unused")
        private final long generation;
        private final ArrayDeque<String> buffer = new ArrayDeque<>();
        private boolean initializing = true;

        private PublicSubscription(int marketId, String interval, Set<String> channels, long generation) {
            this.marketId = marketId;
            this.interval = interval;
            this.channels = channels;
            this.generation = generation;
        }
    }
}

// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.websocket;

import com.openexchange.oms.api.OrderService;
import com.openexchange.oms.api.auth.HttpAuthHandler;
import com.openexchange.oms.api.auth.Principal;
import com.openexchange.oms.api.auth.RoleBasedAuthorizer;
import com.openexchange.oms.api.dto.ExecutionResponse;
import com.openexchange.oms.common.domain.ExecutionReport;
import com.openexchange.oms.common.enums.OrderSide;
import io.netty.channel.embedded.EmbeddedChannel;
import io.netty.handler.codec.http.websocketx.TextWebSocketFrame;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CompletionStage;
import java.util.function.Consumer;
import java.util.stream.IntStream;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class WebSocketHandlerTest {

    private final List<EmbeddedChannel> channels = new ArrayList<>();
    private FakeMarketDataSource marketData;
    private WebSocketHandler handler;

    @AfterEach
    void tearDown() {
        channels.forEach(EmbeddedChannel::finishAndReleaseAll);
        if (marketData != null) marketData.close();
    }

    @Test
    void anonymousSnapshotPrecedesBufferedLiveFrame() {
        marketData = new FakeMarketDataSource(false);
        EmbeddedChannel channel = channel();

        send(channel, subscribe(1, "1m", "orderbook", "trades", "candles"));
        assertType(read(channel), "SUBSCRIBED");
        marketData.emit("{\"type\":\"BOOK_DELTA\",\"marketId\":1}");
        assertNull(channel.readOutbound(), "live frame must wait behind the snapshots");

        marketData.completeNext(1, "1m");
        channel.runPendingTasks();
        assertType(read(channel), "BOOK_SNAPSHOT");
        assertType(read(channel), "TRADES_BATCH");
        assertType(read(channel), "CANDLE_HISTORY");
        assertType(read(channel), "BOOK_DELTA");
    }

    @Test
    void marketSwitchDiscardsOldSnapshotAndFiltersMarketAndInterval() {
        marketData = new FakeMarketDataSource(false);
        EmbeddedChannel channel = channel();
        send(channel, subscribe(1, "1m", "candles"));
        read(channel);
        send(channel, subscribe(2, "5m", "candles"));
        read(channel);

        marketData.completeNext(1, "1m");
        channel.runPendingTasks();
        assertNull(channel.readOutbound(), "superseded market snapshot must be discarded");
        marketData.completeNext(2, "5m");
        channel.runPendingTasks();
        assertType(read(channel), "CANDLE_HISTORY");

        marketData.emit("{\"type\":\"CANDLE_UPDATE\",\"marketId\":1,\"interval\":\"5m\"}");
        marketData.emit("{\"type\":\"CANDLE_UPDATE\",\"marketId\":2,\"interval\":\"1m\"}");
        channel.runPendingTasks();
        assertNull(channel.readOutbound());
        marketData.emit("{\"type\":\"CANDLE_UPDATE\",\"marketId\":2,\"interval\":\"5m\"}");
        channel.runPendingTasks();
        assertType(read(channel), "CANDLE_UPDATE");
    }

    @Test
    void acceptsEverySupportedCandleInterval() {
        marketData = new FakeMarketDataSource(true);
        for (String interval : List.of("1m", "5m", "15m", "1h", "4h", "1d")) {
            EmbeddedChannel channel = channel();
            send(channel, subscribe(1, interval, "candles"));
            assertType(read(channel), "SUBSCRIBED");
            channel.runPendingTasks();
            assertType(read(channel), "CANDLE_HISTORY");
        }
        assertEquals(List.of("1m", "5m", "15m", "1h", "4h", "1d"),
                marketData.requests.stream().map(Request::interval).toList());
    }

    @Test
    void invalidInputsReturnStructuredErrorsWithoutClosing() {
        marketData = new FakeMarketDataSource(true);
        EmbeddedChannel channel = channel();

        send(channel, subscribe(99, "1m", "trades"));
        assertCode(read(channel), "INVALID_MARKET");
        send(channel, subscribe(1, "2m", "candles"));
        assertCode(read(channel), "INVALID_INTERVAL");
        send(channel, subscribe(1, "1m", "secret"));
        assertCode(read(channel), "INVALID_CHANNEL");
        assertTrue(channel.isActive());
    }

    @Test
    void anonymousPrivateSubscriptionReturnsAuthRequiredAndKeepsPublicStream() {
        marketData = new FakeMarketDataSource(true);
        EmbeddedChannel channel = channel();

        send(channel, subscribe(1, "1m", "trades", "orders", "executions"));
        assertCode(read(channel), "AUTH_REQUIRED");
        assertType(read(channel), "SUBSCRIBED");
        channel.runPendingTasks();
        assertType(read(channel), "TRADES_BATCH");
        marketData.emit("{\"type\":\"TRADES_BATCH\",\"marketId\":1,\"trades\":[]}");
        channel.runPendingTasks();
        assertType(read(channel), "TRADES_BATCH");
        assertTrue(channel.isActive());
    }

    @Test
    void unsubscribeStopsPublicDelivery() {
        marketData = new FakeMarketDataSource(true);
        EmbeddedChannel channel = channel();
        send(channel, subscribe(1, "1m", "orderbook"));
        read(channel);
        channel.runPendingTasks();
        read(channel);

        send(channel, "{\"op\":\"unsubscribe\"}");
        assertType(read(channel), "UNSUBSCRIBED");
        marketData.emit("{\"type\":\"BOOK_DELTA\",\"marketId\":1}");
        assertNull(channel.readOutbound());
    }

    @Test
    void authenticatedHistoryUsesPrincipalAndPrecedesRacingLiveExecution() throws Exception {
        marketData = new FakeMarketDataSource(true);
        OrderService orderService = mock(OrderService.class);
        CountDownLatch queryStarted = new CountDownLatch(1);
        CountDownLatch releaseQuery = new CountDownLatch(1);
        ExecutionResponse historical = execution(11L, 101L);
        when(orderService.getExecutions(101L, 100, 0)).thenAnswer(ignored -> {
            queryStarted.countDown();
            assertTrue(releaseQuery.await(2, TimeUnit.SECONDS));
            return List.of(historical);
        });
        handler = new WebSocketHandler(
                new RoleBasedAuthorizer(), marketData, Set.of(1, 2, 3), orderService);
        EmbeddedChannel channel = authenticatedChannel(101L);

        send(channel, "{\"op\":\"subscribe\",\"channels\":[\"executions\"],\"userId\":999}");
        assertType(read(channel), "SUBSCRIBED");
        assertTrue(queryStarted.await(2, TimeUnit.SECONDS));
        handler.pushToUser(101L, "executions", Map.of(
                "type", "EXECUTION_REPORT", "execution", execution(12L, 101L)));
        assertNull(channel.readOutbound(), "live execution must wait behind history");

        releaseQuery.countDown();
        String history = readEventually(channel);
        assertType(history, "EXECUTION_HISTORY");
        assertTrue(history.contains("\"tradeId\":\"11\""), history);
        String live = readEventually(channel);
        assertType(live, "EXECUTION_REPORT");
        assertTrue(live.contains("\"tradeId\":\"12\""), live);
        verify(orderService).getExecutions(101L, 100, 0);
    }

    @Test
    void liveExecutionsStayIsolatedAcrossUsersAndReachEveryTab() {
        marketData = new FakeMarketDataSource(true);
        OrderService orderService = mock(OrderService.class);
        when(orderService.getExecutions(org.mockito.ArgumentMatchers.anyLong(),
                org.mockito.ArgumentMatchers.eq(100), org.mockito.ArgumentMatchers.eq(0)))
                .thenReturn(List.of());
        handler = new WebSocketHandler(
                new RoleBasedAuthorizer(), marketData, Set.of(1, 2, 3), orderService);
        EmbeddedChannel userOneTabA = authenticatedChannel(101L);
        EmbeddedChannel userOneTabB = authenticatedChannel(101L);
        EmbeddedChannel userTwo = authenticatedChannel(202L);
        for (EmbeddedChannel channel : List.of(userOneTabA, userOneTabB, userTwo)) {
            send(channel, "{\"op\":\"subscribe\",\"channels\":[\"executions\"]}");
            assertType(read(channel), "SUBSCRIBED");
            assertType(readEventually(channel), "EXECUTION_HISTORY");
        }

        handler.pushToUser(101L, "executions", Map.of(
                "type", "EXECUTION_REPORT", "execution", execution(21L, 101L)));

        assertType(readEventually(userOneTabA), "EXECUTION_REPORT");
        assertType(readEventually(userOneTabB), "EXECUTION_REPORT");
        userTwo.runPendingTasks();
        assertNull(userTwo.readOutbound());
    }

    @Test
    void unavailableExecutionHistoryReturnsErrorWithoutClosingConnection() {
        marketData = new FakeMarketDataSource(true);
        OrderService orderService = mock(OrderService.class);
        when(orderService.getExecutions(101L, 100, 0))
                .thenThrow(new IllegalStateException("execution persistence unavailable"));
        handler = new WebSocketHandler(
                new RoleBasedAuthorizer(), marketData, Set.of(1, 2, 3), orderService);
        EmbeddedChannel channel = authenticatedChannel(101L);

        send(channel, "{\"op\":\"subscribe\",\"channels\":[\"executions\"]}");
        assertType(read(channel), "SUBSCRIBED");
        assertCode(readEventually(channel), "HISTORY_UNAVAILABLE");
        assertTrue(channel.isActive());
    }

    private EmbeddedChannel channel() {
        if (handler == null) {
            handler = new WebSocketHandler(
                    new com.openexchange.oms.api.auth.RoleBasedAuthorizer(), marketData, Set.of(1, 2, 3));
        }
        EmbeddedChannel channel = new EmbeddedChannel(handler);
        channels.add(channel);
        return channel;
    }

    private EmbeddedChannel authenticatedChannel(long userId) {
        EmbeddedChannel channel = new EmbeddedChannel(handler);
        channel.attr(HttpAuthHandler.PRINCIPAL)
                .set(new Principal("user-" + userId, userId, Set.of()));
        channels.add(channel);
        return channel;
    }

    private static ExecutionResponse execution(long tradeId, long userId) {
        ExecutionReport report = new ExecutionReport();
        report.setTradeId(tradeId);
        report.setOmsOrderId(5000L + tradeId);
        report.setUserId(userId);
        report.setMarketId(1);
        report.setSide(OrderSide.BUY);
        report.setPrice(123_45000000L);
        report.setQuantity(2_00000000L);
        report.setExecutedAtMs(1_700_000_000_000L + tradeId);
        return ExecutionResponse.fromExecution(report);
    }

    private static String subscribe(int marketId, String interval, String... subscribedChannels) {
        return "{\"op\":\"subscribe\",\"channels\":[\""
                + String.join("\",\"", subscribedChannels)
                + "\"],\"marketId\":" + marketId + ",\"interval\":\"" + interval + "\"}";
    }

    private static void send(EmbeddedChannel channel, String json) {
        channel.writeInbound(new TextWebSocketFrame(json));
    }

    private static String read(EmbeddedChannel channel) {
        TextWebSocketFrame frame = channel.readOutbound();
        String text = frame.text();
        frame.release();
        return text;
    }

    private static String readEventually(EmbeddedChannel channel) {
        for (int attempt = 0; attempt < 200; attempt++) {
            channel.runPendingTasks();
            TextWebSocketFrame frame = channel.readOutbound();
            if (frame != null) {
                String text = frame.text();
                frame.release();
                return text;
            }
            try {
                Thread.sleep(5);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new AssertionError("interrupted while awaiting outbound frame", e);
            }
        }
        throw new AssertionError("timed out awaiting outbound frame");
    }

    private static void assertType(String json, String type) {
        assertTrue(json.contains("\"type\":\"" + type + "\""), json);
    }

    private static void assertCode(String json, String code) {
        assertTrue(json.contains("\"code\":\"" + code + "\""), json);
    }

    private record Request(int marketId, String interval, CompletableFuture<List<String>> future) { }

    private static final class FakeMarketDataSource implements MarketDataSource {
        private final boolean completeImmediately;
        private final List<Request> requests = new ArrayList<>();
        private Consumer<String> consumer = ignored -> { };
        private boolean closed;

        private FakeMarketDataSource(boolean completeImmediately) {
            this.completeImmediately = completeImmediately;
        }

        @Override
        public void start(IntStream marketIds, Consumer<String> liveFrameConsumer) {
            assertEquals(Set.of(1, 2, 3), marketIds.boxed().collect(java.util.stream.Collectors.toSet()));
            this.consumer = liveFrameConsumer;
        }

        @Override
        public CompletionStage<List<String>> loadInitialFrames(
                int marketId, String interval, int tradeLimit, int candleLimit) {
            CompletableFuture<List<String>> future = new CompletableFuture<>();
            Request request = new Request(marketId, interval, future);
            requests.add(request);
            if (completeImmediately) complete(request);
            return future;
        }

        private void completeNext(int marketId, String interval) {
            Request request = requests.stream()
                    .filter(candidate -> candidate.marketId == marketId
                            && candidate.interval.equals(interval)
                            && !candidate.future.isDone())
                    .findFirst().orElseThrow();
            complete(request);
        }

        private static void complete(Request request) {
            request.future.complete(List.of(
                    "{\"type\":\"BOOK_SNAPSHOT\",\"marketId\":" + request.marketId + "}",
                    "{\"type\":\"TRADES_BATCH\",\"marketId\":" + request.marketId + "}",
                    "{\"type\":\"CANDLE_HISTORY\",\"marketId\":" + request.marketId
                            + ",\"interval\":\"" + request.interval + "\"}"));
        }

        private void emit(String frame) {
            consumer.accept(frame);
        }

        @Override
        public void close() {
            closed = true;
        }
    }
}

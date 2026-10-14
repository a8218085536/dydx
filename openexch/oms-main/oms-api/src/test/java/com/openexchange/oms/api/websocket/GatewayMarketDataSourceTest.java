// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.websocket;

import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelInboundHandlerAdapter;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import io.netty.handler.codec.http.DefaultFullHttpResponse;
import io.netty.handler.codec.http.FullHttpRequest;
import io.netty.handler.codec.http.HttpObjectAggregator;
import io.netty.handler.codec.http.HttpResponseStatus;
import io.netty.handler.codec.http.HttpServerCodec;
import io.netty.handler.codec.http.HttpUtil;
import io.netty.handler.codec.http.HttpVersion;
import io.netty.handler.codec.http.websocketx.TextWebSocketFrame;
import io.netty.handler.codec.http.websocketx.WebSocketServerProtocolHandler;
import io.netty.util.CharsetUtil;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.net.InetSocketAddress;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.TimeUnit;
import java.util.function.BooleanSupplier;
import java.util.stream.IntStream;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class GatewayMarketDataSourceTest {

    private LocalGateway gateway;
    private GatewayMarketDataSource source;

    @AfterEach
    void tearDown() {
        if (source != null) source.close();
        if (gateway != null) gateway.close();
    }

    @Test
    void startsOneSharedWebSocketPerDistinctMarketAndRelaysRecognizedFrames() throws Exception {
        gateway = new LocalGateway();
        source = new GatewayMarketDataSource(gateway.baseUrl(), Duration.ofMillis(20), Duration.ofMillis(80));
        List<String> frames = new CopyOnWriteArrayList<>();

        source.start(IntStream.of(1, 1, 2), frames::add);

        await(() -> gateway.subscriptions.size() == 2);
        assertTrue(gateway.subscriptions.contains("{\"action\":\"subscribe\",\"marketId\":1}"));
        assertTrue(gateway.subscriptions.contains("{\"action\":\"subscribe\",\"marketId\":2}"));
        await(() -> frames.stream().filter(frame -> frame.contains("BOOK_SNAPSHOT")).count() >= 2);

        gateway.broadcast("{\"type\":\"BOOK_DELTA\",\"marketId\":1}");
        gateway.broadcast("{\"type\":\"PRIVATE_ORDER\",\"marketId\":1}");

        await(() -> frames.stream().anyMatch(frame -> frame.contains("BOOK_DELTA")));
        assertFalse(frames.stream().anyMatch(frame -> frame.contains("PRIVATE_ORDER")));
    }

    @Test
    void loadsBookTradesAndRequestedCandleSnapshotsFromBoundedPaths() throws Exception {
        gateway = new LocalGateway();
        source = new GatewayMarketDataSource(gateway.baseUrl(), Duration.ofMillis(20), Duration.ofMillis(80));

        List<String> frames = source.loadInitialFrames(2, "5m", 25, 120)
                .toCompletableFuture().get(3, TimeUnit.SECONDS);

        assertEquals(3, frames.size());
        assertTrue(frames.get(0).contains("BOOK_SNAPSHOT"));
        assertTrue(frames.get(1).contains("TRADES_BATCH"));
        assertTrue(frames.get(2).contains("CANDLE_HISTORY"));
        assertTrue(gateway.requests.contains("/api/orderbook?marketId=2"));
        assertTrue(gateway.requests.contains("/api/trades?marketId=2&limit=25"));
        assertTrue(gateway.requests.contains("/api/candles?marketId=2&interval=5m&limit=120"));
    }

    @Test
    void reconnectsAfterDisconnectAndCloseStopsFurtherConnections() throws Exception {
        gateway = new LocalGateway();
        source = new GatewayMarketDataSource(gateway.baseUrl(), Duration.ofMillis(20), Duration.ofMillis(80));
        source.start(IntStream.of(1), ignored -> { });
        await(() -> gateway.connectionCount >= 1);

        gateway.disconnectSockets();
        await(() -> gateway.connectionCount >= 2);

        source.close();
        int connectionsAfterClose = gateway.connectionCount;
        gateway.disconnectSockets();
        Thread.sleep(180);
        assertEquals(connectionsAfterClose, gateway.connectionCount);
    }

    private static void await(BooleanSupplier condition) throws Exception {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
        while (!condition.getAsBoolean() && System.nanoTime() < deadline) {
            Thread.sleep(10);
        }
        assertTrue(condition.getAsBoolean(), "condition did not become true before timeout");
    }

    private static final class LocalGateway implements AutoCloseable {
        private final EventLoopGroup boss = new NioEventLoopGroup(1);
        private final EventLoopGroup workers = new NioEventLoopGroup(1);
        private final List<String> subscriptions = new CopyOnWriteArrayList<>();
        private final List<String> requests = new CopyOnWriteArrayList<>();
        private final List<Channel> sockets = new CopyOnWriteArrayList<>();
        private final Channel server;
        private volatile int connectionCount;

        private LocalGateway() throws InterruptedException {
            server = new ServerBootstrap()
                    .group(boss, workers)
                    .channel(NioServerSocketChannel.class)
                    .childHandler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel channel) {
                            channel.pipeline().addLast(new HttpServerCodec());
                            channel.pipeline().addLast(new HttpObjectAggregator(64 * 1024));
                            channel.pipeline().addLast(new RestHandler());
                            channel.pipeline().addLast(new WebSocketServerProtocolHandler("/ws"));
                            channel.pipeline().addLast(new SocketHandler());
                        }
                    })
                    .bind(0).sync().channel();
        }

        private String baseUrl() {
            return "http://127.0.0.1:" + ((InetSocketAddress) server.localAddress()).getPort();
        }

        private void broadcast(String json) {
            for (Channel socket : sockets) {
                if (socket.isActive()) socket.writeAndFlush(new TextWebSocketFrame(json));
            }
        }

        private void disconnectSockets() {
            for (Channel socket : List.copyOf(sockets)) socket.close();
        }

        @Override
        public void close() {
            disconnectSockets();
            server.close();
            boss.shutdownGracefully();
            workers.shutdownGracefully();
        }

        private final class RestHandler extends SimpleChannelInboundHandler<FullHttpRequest> {
            @Override
            protected void channelRead0(ChannelHandlerContext ctx, FullHttpRequest request) {
                if (request.uri().equals("/ws")) {
                    request.retain();
                    ctx.fireChannelRead(request);
                    return;
                }
                requests.add(request.uri());
                String type = request.uri().startsWith("/api/orderbook") ? "BOOK_SNAPSHOT"
                        : request.uri().startsWith("/api/trades") ? "TRADES_BATCH"
                        : request.uri().startsWith("/api/candles") ? "CANDLE_HISTORY" : "ERROR";
                var response = new DefaultFullHttpResponse(
                        HttpVersion.HTTP_1_1,
                        type.equals("ERROR") ? HttpResponseStatus.NOT_FOUND : HttpResponseStatus.OK,
                        io.netty.buffer.Unpooled.copiedBuffer(
                                "{\"type\":\"" + type + "\",\"marketId\":2}", CharsetUtil.UTF_8));
                HttpUtil.setContentLength(response, response.content().readableBytes());
                ctx.writeAndFlush(response);
            }
        }

        private final class SocketHandler extends ChannelInboundHandlerAdapter {
            @Override
            public void userEventTriggered(ChannelHandlerContext ctx, Object event) throws Exception {
                if (event instanceof WebSocketServerProtocolHandler.HandshakeComplete) {
                    connectionCount++;
                    sockets.add(ctx.channel());
                }
                super.userEventTriggered(ctx, event);
            }

            @Override
            public void channelRead(ChannelHandlerContext ctx, Object message) {
                if (message instanceof TextWebSocketFrame frame) {
                    subscriptions.add(frame.text());
                    frame.release();
                } else {
                    ctx.fireChannelRead(message);
                }
            }
        }
    }
}

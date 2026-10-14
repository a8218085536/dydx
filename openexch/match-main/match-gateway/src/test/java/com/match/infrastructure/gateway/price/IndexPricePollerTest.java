// SPDX-License-Identifier: Apache-2.0
package com.match.infrastructure.gateway.price;

import com.google.gson.JsonArray;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.match.infrastructure.gateway.state.GatewayStateManager;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

public class IndexPricePollerTest {

    private HttpServer server;
    private final Map<String, Response> responses = new ConcurrentHashMap<>();
    private String baseUrl;

    @Before
    public void setUp() throws IOException {
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/api/v1/index-prices/", this::respond);
        server.start();
        baseUrl = "http://127.0.0.1:" + server.getAddress().getPort() + "/api/v1/index-prices";
    }

    @After
    public void tearDown() {
        server.stop(0);
    }

    @Test
    public void refreshMapsAllConfiguredMarketsToTheirGuidePrices() {
        responses.put("NETH-USDT", ok("2496.53000000"));
        responses.put("NBNB-USDT", ok("748.65000000"));
        responses.put("NBTC-USDT", ok("82785.10000000"));
        GatewayStateManager state = new GatewayStateManager();

        try (IndexPricePoller poller = new IndexPricePoller(state, baseUrl, Duration.ofSeconds(5))) {
            poller.refreshNow();
        }

        JsonArray tickers = JsonParser.parseString(state.getAllTickerStats()).getAsJsonArray();
        assertEquals(2496.53, tickers.get(0).getAsJsonObject().get("indexPrice").getAsDouble(), 0.000001);
        assertEquals(748.65, tickers.get(1).getAsJsonObject().get("indexPrice").getAsDouble(), 0.000001);
        assertEquals(82785.10, tickers.get(2).getAsJsonObject().get("indexPrice").getAsDouble(), 0.000001);
        assertTrue(tickers.get(0).getAsJsonObject().get("lastPrice").isJsonNull());
    }

    @Test
    public void failedOrInvalidRefreshRetainsLastValidGuidePrice() {
        responses.put("NETH-USDT", ok("2496.53000000"));
        GatewayStateManager state = new GatewayStateManager();

        try (IndexPricePoller poller = new IndexPricePoller(state, baseUrl, Duration.ofSeconds(5))) {
            poller.refreshNow();
            responses.put("NETH-USDT", new Response(200,
                    "{\"market\":\"NETH/USDT\",\"indexPrice\":\"9999\",\"stale\":true}"));
            poller.refreshNow();
            responses.put("NETH-USDT", new Response(200, "{not-json"));
            poller.refreshNow();
            responses.put("NETH-USDT", ok("0"));
            poller.refreshNow();
            responses.put("NETH-USDT", new Response(503, "unavailable"));
            poller.refreshNow();
        }

        JsonObject ticker = JsonParser.parseString(state.getTickerStats(1)).getAsJsonObject();
        assertEquals(2496.53, ticker.get("indexPrice").getAsDouble(), 0.000001);
    }

    @Test
    public void startRefreshesPricesInTheBackground() throws Exception {
        responses.put("NETH-USDT", ok("2496.53000000"));
        GatewayStateManager state = new GatewayStateManager();

        try (IndexPricePoller poller = new IndexPricePoller(state, baseUrl, Duration.ofMillis(20))) {
            poller.start();
            long deadline = System.nanoTime() + Duration.ofSeconds(1).toNanos();
            while (state.getTickerStats(1) == null && System.nanoTime() < deadline) {
                Thread.sleep(10);
            }
        }

        JsonObject ticker = JsonParser.parseString(state.getTickerStats(1)).getAsJsonObject();
        assertEquals(2496.53, ticker.get("indexPrice").getAsDouble(), 0.000001);
    }

    private Response ok(String price) {
        return new Response(200,
                "{\"market\":\"NETH/USDT\",\"indexPrice\":\"" + price + "\",\"stale\":false}");
    }

    private void respond(HttpExchange exchange) throws IOException {
        String path = exchange.getRequestURI().getPath();
        String symbol = path.substring(path.lastIndexOf('/') + 1);
        Response response = responses.getOrDefault(symbol, new Response(404, "not found"));
        byte[] body = response.body().getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.sendResponseHeaders(response.status(), body.length);
        exchange.getResponseBody().write(body);
        exchange.close();
    }

    private record Response(int status, String body) {
    }
}

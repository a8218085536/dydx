// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.rest;

import com.openexchange.oms.api.AdminService;
import com.openexchange.oms.api.InternalDepositService;
import com.openexchange.oms.api.NecoWithdrawalService;
import com.openexchange.oms.api.OrderService;
import com.openexchange.oms.api.audit.AuditLog;
import com.openexchange.oms.api.auth.HttpAuthHandler;
import com.openexchange.oms.api.auth.Principal;
import com.openexchange.oms.api.auth.RoleBasedAuthorizer;
import com.openexchange.oms.api.dto.NecoWithdrawalResponse;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import io.netty.buffer.Unpooled;
import io.netty.channel.embedded.EmbeddedChannel;
import io.netty.handler.codec.http.DefaultFullHttpRequest;
import io.netty.handler.codec.http.FullHttpRequest;
import io.netty.handler.codec.http.FullHttpResponse;
import io.netty.handler.codec.http.HttpMethod;
import io.netty.handler.codec.http.HttpResponseStatus;
import io.netty.handler.codec.http.HttpVersion;

import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.util.Set;
import java.util.concurrent.atomic.AtomicInteger;

/** Standalone production-JAR compatibility test; run with assertions enabled. */
public final class MarketMakerProductionRouteTest {

    private static final long MAKER_USER_ID = 910001L;
    private static final String WITHDRAWAL_BODY = """
            {"clientWithdrawalId":"mm-deny-1",
             "destinationAddress":"0x1111111111111111111111111111111111111111",
             "assetId":0,"amount":"1","targetChainId":%d}
            """;

    private MarketMakerProductionRouteTest() {
    }

    public static void main(String[] args) {
        marketMakerCannotCreditInternalDeposit();
        marketMakerCannotWithdrawToNeco();
        marketMakerCannotWithdrawThroughBridge();
        ordinaryTychePrincipalRetainsWithdrawalAccess();
        System.out.println("PASS: production money routes enforce MARKET_MAKER denial");
    }

    private static void marketMakerCannotCreditInternalDeposit() {
        AtomicInteger deposits = new AtomicInteger();
        TestServices services = services(deposits, new AtomicInteger());
        FullHttpResponse response = exchange(services, maker(), HttpMethod.POST,
                "/api/v1/internal/deposits/neco", """
                {"reference":"tx:1","tycheUserId":"tyche-1",
                 "necoAddress":"0x1111111111111111111111111111111111111111",
                 "assetId":0,"amount":"1"}
                """);

        require(response.status().equals(HttpResponseStatus.FORBIDDEN),
                "MARKET_MAKER internal deposit must return 403, got " + response.status());
        require(deposits.get() == 0, "internal deposit service must not be called");
        response.release();
    }

    private static void marketMakerCannotWithdrawToNeco() {
        assertMakerWithdrawalDenied(7260L);
    }

    private static void marketMakerCannotWithdrawThroughBridge() {
        assertMakerWithdrawalDenied(56L);
    }

    private static void assertMakerWithdrawalDenied(long targetChainId) {
        AtomicInteger withdrawals = new AtomicInteger();
        TestServices services = services(new AtomicInteger(), withdrawals);
        FullHttpResponse response = exchange(services, maker(), HttpMethod.POST,
                "/api/v1/neco/withdrawals", WITHDRAWAL_BODY.formatted(targetChainId));

        require(response.status().equals(HttpResponseStatus.FORBIDDEN),
                "MARKET_MAKER withdrawal to chain " + targetChainId
                        + " must return 403, got " + response.status());
        require(withdrawals.get() == 0, "withdrawal service must not be called");
        response.release();
    }

    private static void ordinaryTychePrincipalRetainsWithdrawalAccess() {
        AtomicInteger withdrawals = new AtomicInteger();
        TestServices services = services(new AtomicInteger(), withdrawals);
        FullHttpResponse response = exchange(services,
                new Principal("tyche-wallet", 1000001L, Set.of()), HttpMethod.POST,
                "/api/v1/neco/withdrawals", WITHDRAWAL_BODY.formatted(7260L));

        require(!response.status().equals(HttpResponseStatus.FORBIDDEN),
                "ordinary Tyche withdrawal must not be rejected by the funding gate");
        require(withdrawals.get() == 1,
                "ordinary Tyche withdrawal must still reach the withdrawal service");
        response.release();
    }

    private static Principal maker() {
        return new Principal("market-maker", MAKER_USER_ID, Set.of("MARKET_MAKER"));
    }

    private static FullHttpResponse exchange(
            TestServices services, Principal principal, HttpMethod method, String uri, String body) {
        RestApiHandler handler = new RestApiHandler(
                services.orders(), (AdminService) null, new RoleBasedAuthorizer(),
                CorsPolicy.fromSpec(""), AuditLog.disabled(),
                new PrometheusMeterRegistry(PrometheusConfig.DEFAULT),
                services.deposits(), services.withdrawals());
        EmbeddedChannel channel = new EmbeddedChannel(handler);
        channel.attr(HttpAuthHandler.PRINCIPAL).set(principal);
        FullHttpRequest request = new DefaultFullHttpRequest(
                HttpVersion.HTTP_1_1, method, uri,
                Unpooled.copiedBuffer(body, StandardCharsets.UTF_8));
        channel.writeInbound(request);
        FullHttpResponse response = channel.readOutbound();
        require(response != null, "expected an HTTP response for " + method + " " + uri);
        channel.finishAndReleaseAll();
        return response;
    }

    private static TestServices services(
            AtomicInteger depositCalls, AtomicInteger withdrawalCalls) {
        OrderService orders = proxy(OrderService.class, new AtomicInteger());
        InternalDepositService deposits = proxy(InternalDepositService.class, depositCalls);
        NecoWithdrawalService withdrawals = proxy(NecoWithdrawalService.class, withdrawalCalls);
        return new TestServices(orders, deposits, withdrawals);
    }

    @SuppressWarnings("unchecked")
    private static <T> T proxy(Class<T> type, AtomicInteger calls) {
        return (T) Proxy.newProxyInstance(type.getClassLoader(), new Class<?>[]{type},
                (proxy, method, args) -> {
                    if (!method.getDeclaringClass().equals(Object.class)) {
                        calls.incrementAndGet();
                    }
                    if (type == NecoWithdrawalService.class
                            && method.getName().equals("requestWithdrawal")) {
                        NecoWithdrawalResponse response = new NecoWithdrawalResponse();
                        response.setSuccess(true);
                        response.setWithdrawalId("test-withdrawal");
                        response.setAssetId(0);
                        response.setAmount("1.00000000");
                        return response;
                    }
                    Class<?> result = method.getReturnType();
                    if (!result.isPrimitive()) return null;
                    if (result == boolean.class) return false;
                    if (result == byte.class) return (byte) 0;
                    if (result == short.class) return (short) 0;
                    if (result == int.class) return 0;
                    if (result == long.class) return 0L;
                    if (result == float.class) return 0F;
                    if (result == double.class) return 0D;
                    if (result == char.class) return '\0';
                    return null;
                });
    }

    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    private record TestServices(
            OrderService orders,
            InternalDepositService deposits,
            NecoWithdrawalService withdrawals) {
    }
}

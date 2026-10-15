// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.grpc;

import com.openexchange.oms.api.OrderService;
import com.openexchange.oms.api.audit.AuditLog;
import com.openexchange.oms.api.auth.Authorizer;
import com.openexchange.oms.api.auth.GrpcAuthInterceptor;
import com.openexchange.oms.api.auth.Principal;
import com.openexchange.oms.api.auth.RoleBasedAuthorizer;
import com.openexchange.oms.api.dto.CreateOrderResponse;
import com.openexchange.oms.grpc.BalancesResponse;
import com.openexchange.oms.grpc.GetBalancesRequest;
import io.grpc.Context;
import io.grpc.Status;
import io.grpc.StatusRuntimeException;
import io.grpc.stub.StreamObserver;
import org.junit.jupiter.api.Test;

import java.util.Collections;
import java.util.Set;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class GrpcMarketMakerAuthorizationTest {

    private static final Principal MAKER = new Principal(
            "maker", 910001L, Set.of(Principal.ROLE_MARKET_MAKER));

    @Test
    void accountReadsUseReadAuthorization() {
        OrderService orders = mock(OrderService.class);
        when(orders.getBalances(910001L)).thenReturn(Collections.emptyMap());
        Authorizer readOnly = (principal, action, resource) ->
                action.equals(Authorizer.ACTION_READ_AS_USER) && resource.equals("910001");
        GrpcAccountService service = new GrpcAccountService(orders, readOnly);
        RecordingObserver<BalancesResponse> observer = new RecordingObserver<>();

        withPrincipal(MAKER, () -> service.getBalances(
                GetBalancesRequest.newBuilder().setUserId(910001L).build(), observer));

        assertNull(observer.error.get());
        assertEquals(910001L, observer.value.get().getUserId());
        verify(orders).getBalances(910001L);
    }

    @Test
    void orderCreationUsesTradeAuthorization() {
        OrderService orders = mock(OrderService.class);
        when(orders.createOrder(any())).thenReturn(CreateOrderResponse.accepted(1L, "NEW"));
        Authorizer tradeOnly = (principal, action, resource) ->
                action.equals(Authorizer.ACTION_TRADE_AS_USER) && resource.equals("910001");
        GrpcOrderService service = new GrpcOrderService(orders, tradeOnly, AuditLog.disabled());
        RecordingObserver<com.openexchange.oms.grpc.CreateOrderResponse> observer =
                new RecordingObserver<>();
        com.openexchange.oms.grpc.CreateOrderRequest request =
                com.openexchange.oms.grpc.CreateOrderRequest.newBuilder()
                        .setUserId(910001L)
                        .setMarketId(1)
                        .setSide("BUY")
                        .setOrderType("LIMIT")
                        .setPrice("1")
                        .setQuantity("1")
                        .build();

        withPrincipal(MAKER, () -> service.createOrder(request, observer));

        assertNull(observer.error.get());
        assertEquals(1L, observer.value.get().getOmsOrderId());
        verify(orders).createOrder(any());
    }

    @Test
    void marketMakerCannotReadAnotherAccount() {
        OrderService orders = mock(OrderService.class);
        GrpcAccountService service = new GrpcAccountService(orders, new RoleBasedAuthorizer());
        RecordingObserver<BalancesResponse> observer = new RecordingObserver<>();

        withPrincipal(MAKER, () -> service.getBalances(
                GetBalancesRequest.newBuilder().setUserId(910002L).build(), observer));

        assertEquals(Status.Code.PERMISSION_DENIED,
                ((StatusRuntimeException) observer.error.get()).getStatus().getCode());
        verify(orders, never()).getBalances(910002L);
    }

    private static void withPrincipal(Principal principal, Runnable call) {
        Context.current().withValue(GrpcAuthInterceptor.PRINCIPAL, principal).run(call);
    }

    private static final class RecordingObserver<T> implements StreamObserver<T> {
        private final AtomicReference<T> value = new AtomicReference<>();
        private final AtomicReference<Throwable> error = new AtomicReference<>();

        @Override
        public void onNext(T next) {
            value.set(next);
        }

        @Override
        public void onError(Throwable throwable) {
            error.set(throwable);
        }

        @Override
        public void onCompleted() {}
    }
}

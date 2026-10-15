// Production-compatible overlay reconstructed from the deployed OMS JAR.
package com.openexchange.oms.api.rest;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.match.domain.FixedPoint;
import com.openexchange.oms.api.AdminService;
import com.openexchange.oms.api.DepositAddressCatalog;
import com.openexchange.oms.api.InternalDepositService;
import com.openexchange.oms.api.NecoWithdrawalService;
import com.openexchange.oms.api.OrderService;
import com.openexchange.oms.api.audit.AuditLog;
import com.openexchange.oms.api.auth.AuthService;
import com.openexchange.oms.api.auth.Authorizer;
import com.openexchange.oms.api.auth.HttpAuthHandler;
import com.openexchange.oms.api.auth.Principal;
import com.openexchange.oms.api.auth.RoleBasedAuthorizer;
import com.openexchange.oms.api.dto.CancelOrderResponse;
import com.openexchange.oms.api.dto.CreateOrderRequest;
import com.openexchange.oms.api.dto.CreateOrderResponse;
import com.openexchange.oms.api.dto.DepositAddress;
import com.openexchange.oms.api.dto.ExecutionResponse;
import com.openexchange.oms.api.dto.InternalNecoDepositRequest;
import com.openexchange.oms.api.dto.InternalNecoDepositResponse;
import com.openexchange.oms.api.dto.NecoWithdrawalRequest;
import com.openexchange.oms.api.dto.NecoWithdrawalResponse;
import com.openexchange.oms.api.dto.OrderResponse;
import com.openexchange.oms.api.dto.PositionResponse;
import com.openexchange.oms.api.rest.CorsPolicy;
import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.Timer;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import io.netty.buffer.ByteBuf;
import io.netty.buffer.Unpooled;
import io.netty.channel.ChannelFuture;
import io.netty.channel.ChannelFutureListener;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.handler.codec.http.DefaultFullHttpResponse;
import io.netty.handler.codec.http.FullHttpRequest;
import io.netty.handler.codec.http.FullHttpResponse;
import io.netty.handler.codec.http.HttpHeaderNames;
import io.netty.handler.codec.http.HttpMethod;
import io.netty.handler.codec.http.HttpResponseStatus;
import io.netty.handler.codec.http.HttpUtil;
import io.netty.handler.codec.http.HttpVersion;
import io.netty.handler.codec.http.QueryStringDecoder;
import io.netty.util.concurrent.GenericFutureListener;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

public class RestApiHandler
extends SimpleChannelInboundHandler<FullHttpRequest> {
    private static final Logger log = LoggerFactory.getLogger(RestApiHandler.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final long MAX_MONEY_FP = 9000000000000000000L;
    static final String ERR_VALIDATION = "VALIDATION";
    static final String ERR_UNAUTHORIZED = "UNAUTHORIZED";
    static final String ERR_FORBIDDEN = "FORBIDDEN";
    static final String ERR_NOT_FOUND = "NOT_FOUND";
    static final String ERR_REJECTED = "REJECTED";
    static final String ERR_ADMIN_UNAVAILABLE = "ADMIN_UNAVAILABLE";
    static final String ERR_UNAVAILABLE = "UNAVAILABLE";
    static final String ERR_INTERNAL = "INTERNAL";
    private final OrderService orderService;
    private final AdminService adminService;
    private final Authorizer authorizer;
    private final CorsPolicy corsPolicy;
    private final AuditLog auditLog;
    private final PrometheusMeterRegistry meterRegistry;
    private final InternalDepositService internalDepositService;
    private final NecoWithdrawalService necoWithdrawalService;
    private static final List<DepositAddress> DEPOSIT_ADDRESSES = DepositAddressCatalog.load();
    private volatile AuthService authService;
    private static final ExecutorService AUTH_EXECUTOR = Executors.newFixedThreadPool(2, runnable -> {
        Thread thread = new Thread(runnable, "oms-auth");
        thread.setDaemon(true);
        return thread;
    });
    private static final ExecutorService RISK_PERSIST_EXECUTOR = Executors.newSingleThreadExecutor(runnable -> {
        Thread thread = new Thread(runnable, "oms-risk-persist");
        thread.setDaemon(true);
        return thread;
    });

    public RestApiHandler(OrderService orderService) {
        this(orderService, null, new RoleBasedAuthorizer(), CorsPolicy.fromSpec(""), AuditLog.disabled(), new PrometheusMeterRegistry(PrometheusConfig.DEFAULT));
    }

    public RestApiHandler(OrderService orderService, AdminService adminService, Authorizer authorizer, CorsPolicy corsPolicy, AuditLog auditLog, PrometheusMeterRegistry prometheusMeterRegistry) {
        this(orderService, adminService, authorizer, corsPolicy, auditLog, prometheusMeterRegistry, null, null);
    }

    public RestApiHandler(OrderService orderService, AdminService adminService, Authorizer authorizer, CorsPolicy corsPolicy, AuditLog auditLog, PrometheusMeterRegistry prometheusMeterRegistry, InternalDepositService internalDepositService) {
        this(orderService, adminService, authorizer, corsPolicy, auditLog, prometheusMeterRegistry, internalDepositService, null);
    }

    public RestApiHandler(OrderService orderService, AdminService adminService, Authorizer authorizer, CorsPolicy corsPolicy, AuditLog auditLog, PrometheusMeterRegistry prometheusMeterRegistry, InternalDepositService internalDepositService, NecoWithdrawalService necoWithdrawalService) {
        this.orderService = orderService;
        this.adminService = adminService;
        this.authorizer = authorizer;
        this.corsPolicy = corsPolicy;
        this.auditLog = auditLog;
        this.meterRegistry = prometheusMeterRegistry;
        this.internalDepositService = internalDepositService;
        this.necoWithdrawalService = necoWithdrawalService;
    }

    public void setAuthService(AuthService authService) {
        this.authService = authService;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext channelHandlerContext, FullHttpRequest fullHttpRequest) {
        String string = fullHttpRequest.uri();
        HttpMethod httpMethod = fullHttpRequest.method();
        RequestContext requestContext = new RequestContext(channelHandlerContext, fullHttpRequest.headers().get(HttpHeaderNames.ORIGIN), HttpUtil.isKeepAlive(fullHttpRequest));
        if (httpMethod == HttpMethod.OPTIONS) {
            this.sendCorsPreflightResponse(requestContext);
            return;
        }
        long l = System.nanoTime();
        try {
            this.route(requestContext, fullHttpRequest, string, httpMethod);
        }
        catch (Throwable throwable) {
            Timer.builder("oms_http_request_seconds").description("OMS REST request latency").tag("route", RestApiHandler.routeLabel(string, httpMethod)).publishPercentiles(0.5, 0.95, 0.99).register(this.meterRegistry).record(System.nanoTime() - l, TimeUnit.NANOSECONDS);
            throw throwable;
        }
        Timer.builder("oms_http_request_seconds").description("OMS REST request latency").tag("route", RestApiHandler.routeLabel(string, httpMethod)).publishPercentiles(0.5, 0.95, 0.99).register(this.meterRegistry).record(System.nanoTime() - l, TimeUnit.NANOSECONDS);
    }

    private void route(RequestContext requestContext, FullHttpRequest fullHttpRequest, String string, HttpMethod httpMethod) {
        try {
            if (string.equals("/metrics") && httpMethod == HttpMethod.GET) {
                this.handleMetrics(requestContext);
            } else if (string.equals("/api/v1/health") && httpMethod == HttpMethod.GET) {
                this.handleHealth(requestContext);
            } else if (string.equals("/api/v1/auth/register") && httpMethod == HttpMethod.POST) {
                this.handleRegister(requestContext, fullHttpRequest);
            } else if (string.equals("/api/v1/auth/login") && httpMethod == HttpMethod.POST) {
                this.handleLogin(requestContext, fullHttpRequest);
            } else if (string.equals("/api/v1/auth/me") && httpMethod == HttpMethod.GET) {
                this.handleMe(requestContext);
            } else if (string.equals("/api/v1/balances") && httpMethod == HttpMethod.GET) {
                this.handleGetBalances(requestContext);
            } else if (string.equals("/api/v1/deposit-addresses") && httpMethod == HttpMethod.GET) {
                this.handleGetDepositAddresses(requestContext);
            } else if (string.equals("/api/v1/internal/deposits/neco") && httpMethod == HttpMethod.POST) {
                this.handleInternalNecoDeposit(requestContext, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/internal/neco/liabilities") && httpMethod == HttpMethod.GET) {
                this.handleInternalNecoLiabilities(requestContext);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/internal/withdrawals/neco/claim") && httpMethod == HttpMethod.POST) {
                this.handleInternalNecoWithdrawalClaim(requestContext, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/internal/withdrawals/neco/broadcasted") && httpMethod == HttpMethod.GET) {
                this.handleInternalNecoWithdrawalsPendingConfirmation(requestContext, string);
            } else if (RestApiHandler.stripQuery(string).startsWith("/api/v1/internal/withdrawals/neco/") && httpMethod == HttpMethod.POST) {
                this.handleInternalNecoWithdrawalStatus(requestContext, string, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/neco/withdrawals") && httpMethod == HttpMethod.POST) {
                this.handleRequestNecoWithdrawal(requestContext, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).startsWith("/api/v1/neco/withdrawals/") && httpMethod == HttpMethod.GET) {
                this.handleGetNecoWithdrawal(requestContext, string);
            } else if (string.equals("/api/v1/orders") && httpMethod == HttpMethod.POST) {
                this.handleCreateOrder(requestContext, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/orders/history") && httpMethod == HttpMethod.GET) {
                this.handleOrderHistory(requestContext, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/executions") && httpMethod == HttpMethod.GET) {
                this.handleExecutions(requestContext, fullHttpRequest);
            } else if (RestApiHandler.stripQuery(string).equals("/api/v1/positions") && httpMethod == HttpMethod.GET) {
                this.handlePositions(requestContext, fullHttpRequest);
            } else if (string.startsWith("/api/v1/orders/") && httpMethod == HttpMethod.DELETE) {
                this.handleCancelOrder(requestContext, string);
            } else if (string.startsWith("/api/v1/orders/") && httpMethod == HttpMethod.PUT) {
                this.handleUpdateOrder(requestContext, fullHttpRequest, string);
            } else if (string.startsWith("/api/v1/orders/") && httpMethod == HttpMethod.GET) {
                this.handleGetOrder(requestContext, string);
            } else if (string.startsWith("/api/v1/orders") && httpMethod == HttpMethod.GET) {
                this.handleQueryOrders(requestContext, fullHttpRequest);
            } else if (string.matches("/api/v1/accounts/\\d+/deposit") && httpMethod == HttpMethod.POST) {
                this.handleDeposit(requestContext, string, fullHttpRequest);
            } else if (string.matches("/api/v1/accounts/\\d+/withdraw") && httpMethod == HttpMethod.POST) {
                this.handleWithdraw(requestContext, string, fullHttpRequest);
            } else if (string.startsWith("/api/v1/accounts/") && httpMethod == HttpMethod.GET) {
                this.handleGetAccount(requestContext, string);
            } else if (string.equals("/api/v1/markets") && httpMethod == HttpMethod.GET) {
                this.handleGetMarkets(requestContext);
            } else if (string.startsWith("/api/v1/admin/")) {
                String string2 = RestApiHandler.stripQuery(string);
                Principal principal = this.principal(requestContext);
                if (principal == null || !this.authorizer.allow(principal, "admin", String.valueOf(httpMethod) + " " + string)) {
                    this.sendError(requestContext, HttpResponseStatus.FORBIDDEN, ERR_FORBIDDEN, "Admin role required");
                } else if (string2.equals("/api/v1/admin/neco/withdrawals/summary") && httpMethod == HttpMethod.GET) {
                    this.handleAdminNecoWithdrawalSummary(requestContext);
                } else if (string2.equals("/api/v1/admin/neco/withdrawals") && httpMethod == HttpMethod.GET) {
                    this.handleAdminNecoWithdrawals(requestContext, string);
                } else if (string.startsWith("/api/v1/admin/risk/circuit-breaker/") && httpMethod == HttpMethod.POST) {
                    this.handleCircuitBreaker(requestContext, string);
                } else if (string.startsWith("/api/v1/admin/risk/config/") && httpMethod == HttpMethod.PUT) {
                    this.handleUpdateRiskConfig(requestContext, string, fullHttpRequest);
                } else if (string.startsWith("/api/v1/admin/risk/config") && httpMethod == HttpMethod.GET) {
                    this.handleGetRiskConfig(requestContext, string);
                } else {
                    this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Not Found");
                }
            } else {
                this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Not Found");
            }
        }
        catch (Exception exception) {
            log.error("Error handling request: {} {}", httpMethod, string, exception);
            this.sendError(requestContext, HttpResponseStatus.INTERNAL_SERVER_ERROR, ERR_INTERNAL, exception.getMessage());
        }
    }

    private static String stripQuery(String string) {
        int n = string.indexOf(63);
        return n >= 0 ? string.substring(0, n) : string;
    }

    private static String routeLabel(String string, HttpMethod httpMethod) {
        String string2 = RestApiHandler.stripQuery(string);
        String string3 = httpMethod.name();
        if (string2.equals("/metrics")) {
            return "metrics";
        }
        if (string2.equals("/api/v1/health")) {
            return "health";
        }
        if (string2.equals("/api/v1/auth/register")) {
            return "auth.register";
        }
        if (string2.equals("/api/v1/auth/login")) {
            return "auth.login";
        }
        if (string2.equals("/api/v1/auth/me")) {
            return "auth.me";
        }
        if (string2.equals("/api/v1/balances")) {
            return "balances";
        }
        if (string2.equals("/api/v1/deposit-addresses")) {
            return "deposit-addresses";
        }
        if (string2.equals("/api/v1/internal/deposits/neco")) {
            return "internal.deposits.neco";
        }
        if (string2.equals("/api/v1/internal/neco/liabilities")) {
            return "internal.neco.liabilities";
        }
        if (string2.equals("/api/v1/internal/withdrawals/neco/claim")) {
            return "internal.withdrawals.neco.claim";
        }
        if (string2.equals("/api/v1/internal/withdrawals/neco/broadcasted")) {
            return "internal.withdrawals.neco.broadcasted";
        }
        if (string2.startsWith("/api/v1/internal/withdrawals/neco/")) {
            return "internal.withdrawals.neco.status";
        }
        if (string2.equals("/api/v1/neco/withdrawals")) {
            return "neco.withdrawals.create";
        }
        if (string2.startsWith("/api/v1/neco/withdrawals/")) {
            return "neco.withdrawals.get";
        }
        if (string2.equals("/api/v1/admin/neco/withdrawals")) {
            return "admin.neco.withdrawals";
        }
        if (string2.equals("/api/v1/admin/neco/withdrawals/summary")) {
            return "admin.neco.withdrawals.summary";
        }
        if (string2.equals("/api/v1/markets")) {
            return "markets";
        }
        if (string2.equals("/api/v1/orders")) {
            return string3.equals("POST") ? "orders.create" : "orders.query";
        }
        if (string2.equals("/api/v1/orders/history")) {
            return "orders.history";
        }
        if (string2.equals("/api/v1/executions")) {
            return "executions";
        }
        if (string2.equals("/api/v1/positions")) {
            return "positions";
        }
        if (string2.startsWith("/api/v1/orders/")) {
            return switch (string3) {
                case "DELETE" -> "orders.cancel";
                case "PUT" -> "orders.update";
                default -> "orders.get";
            };
        }
        if (string2.startsWith("/api/v1/accounts/")) {
            if (string2.endsWith("/deposit")) {
                return "accounts.deposit";
            }
            if (string2.endsWith("/withdraw")) {
                return "accounts.withdraw";
            }
            return "accounts.get";
        }
        if (string2.startsWith("/api/v1/admin/")) {
            return "admin";
        }
        return "other";
    }

    private void handleMetrics(RequestContext requestContext) {
        String string = this.meterRegistry.scrape();
        byte[] byArray = string.getBytes(StandardCharsets.UTF_8);
        DefaultFullHttpResponse defaultFullHttpResponse = new DefaultFullHttpResponse(HttpVersion.HTTP_1_1, HttpResponseStatus.OK, Unpooled.wrappedBuffer(byArray));
        defaultFullHttpResponse.headers().set((CharSequence)HttpHeaderNames.CONTENT_TYPE, (Object)"text/plain; version=0.0.4; charset=utf-8");
        defaultFullHttpResponse.headers().set((CharSequence)HttpHeaderNames.CONTENT_LENGTH, (Object)byArray.length);
        RestApiHandler.finishResponse(requestContext, defaultFullHttpResponse);
    }

    private Principal principal(RequestContext requestContext) {
        return requestContext.ctx().channel().attr(HttpAuthHandler.PRINCIPAL).get();
    }

    private void handleGetDepositAddresses(RequestContext requestContext) throws Exception {
        if (DEPOSIT_ADDRESSES.isEmpty()) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "NECO deposit configuration is unavailable");
            return;
        }
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(DEPOSIT_ADDRESSES));
    }

    private boolean canPerformAs(RequestContext requestContext, String action, long l) {
        Principal principal = this.principal(requestContext);
        return principal != null && this.authorizer.allow(principal, action, Long.toString(l));
    }

    private boolean canFundAsPrincipal(RequestContext requestContext) {
        Principal principal = this.principal(requestContext);
        return principal != null && this.canPerformAs(
                requestContext, Authorizer.ACTION_FUND_AS_USER, principal.userId());
    }

    private boolean canMoveBalancesManually(RequestContext requestContext, long userId) {
        Principal principal = this.principal(requestContext);
        if (principal == null || principal.hasRole(Principal.ROLE_MARKET_MAKER)) {
            return false;
        }
        return this.authorizer.allow(principal, Authorizer.ACTION_ADMIN, "account.balance")
                || principal.hasRole(Principal.ROLE_FUNDING_ADMIN)
                && this.canPerformAs(requestContext, Authorizer.ACTION_FUND_AS_USER, userId);
    }

    private void handleRegister(RequestContext requestContext, FullHttpRequest fullHttpRequest) {
        this.runAuthRequest(requestContext, fullHttpRequest, (string, string2) -> {
            AuthService.Session session = this.authService.register(string, string2);
            log.info("Registered demo user '{}' (userId={})", (Object)session.username(), (Object)session.userId());
            return session;
        });
    }

    private void handleLogin(RequestContext requestContext, FullHttpRequest fullHttpRequest) {
        this.runAuthRequest(requestContext, fullHttpRequest, (string, string2) -> this.authService.login(string, string2));
    }

    private void handleMe(RequestContext requestContext) {
        Principal principal = this.principal(requestContext);
        if (principal == null) {
            this.sendError(requestContext, HttpResponseStatus.UNAUTHORIZED, ERR_UNAUTHORIZED, "Unauthorized");
            return;
        }
        String string = principal.subject();
        String string2 = string.startsWith("user-") ? string.substring(5) : string;
        this.sendResponse(requestContext, HttpResponseStatus.OK, "{\"userId\":" + principal.userId() + ",\"username\":\"" + RestApiHandler.escapeJson(string2) + "\"}");
    }

    private void runAuthRequest(RequestContext requestContext, FullHttpRequest fullHttpRequest, AuthCall authCall) {
        if (this.authService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "User accounts unavailable: OMS is not running in demo auth mode with persistence");
            return;
        }
        String string = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        AUTH_EXECUTOR.execute(() -> {
            try {
                JsonNode jsonNode = MAPPER.readTree(string);
                String string2 = jsonNode.path("username").asText(null);
                String string3 = jsonNode.path("password").asText(null);
                AuthService.Session session = authCall.run(string2, string3);
                if (session == null) {
                    this.sendError(requestContext, HttpResponseStatus.UNAUTHORIZED, ERR_UNAUTHORIZED, "Invalid username or password");
                    return;
                }
                this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(session));
            }
            catch (AuthService.UsernameTakenException usernameTakenException) {
                this.sendError(requestContext, HttpResponseStatus.CONFLICT, ERR_VALIDATION, usernameTakenException.getMessage());
            }
            catch (IllegalArgumentException illegalArgumentException) {
                this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, illegalArgumentException.getMessage());
            }
            catch (IllegalStateException illegalStateException) {
                this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
            }
            catch (Exception exception) {
                log.error("Auth request failed", exception);
                this.sendError(requestContext, HttpResponseStatus.INTERNAL_SERVER_ERROR, ERR_INTERNAL, RestApiHandler.rootMessage(exception));
            }
        });
    }

    private void handleInternalNecoDeposit(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        if (!this.canFundAsPrincipal(requestContext)) {
            this.sendError(requestContext, HttpResponseStatus.FORBIDDEN, ERR_FORBIDDEN, "Funding access denied");
            return;
        }
        if (this.internalDepositService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Internal deposit service unavailable");
            return;
        }
        String string = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            JsonNode jsonNode = MAPPER.readTree(string);
            String string2 = RestApiHandler.textField(jsonNode, "reference");
            String string3 = RestApiHandler.textField(jsonNode, "tycheUserId");
            String string4 = RestApiHandler.textField(jsonNode, "necoAddress");
            JsonNode jsonNode2 = jsonNode.get("assetId");
            Long l = RestApiHandler.money(jsonNode, "amount");
            if (!(string2 != null && string3 != null && string4 != null && jsonNode2 != null && jsonNode2.isNumber() && jsonNode2.canConvertToExactIntegral() && jsonNode2.asInt() >= 0 && jsonNode2.asInt() <= 1000000 && l != null && RestApiHandler.validMoney(l))) {
                throw new IllegalArgumentException("reference, tycheUserId, necoAddress, assetId, and amount are required");
            }
            InternalNecoDepositRequest internalNecoDepositRequest = new InternalNecoDepositRequest(string2, string3, string4, jsonNode2.asInt(), FixedPoint.format(l), l);
            InternalNecoDepositResponse internalNecoDepositResponse = this.internalDepositService.creditNecoDeposit(internalNecoDepositRequest);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(internalNecoDepositResponse));
        }
        catch (IOException | IllegalArgumentException exception) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(exception));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.CONFLICT, ERR_REJECTED, illegalStateException.getMessage());
        }
    }

    private void handleInternalNecoLiabilities(RequestContext requestContext) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco reconciliation service unavailable");
            return;
        }
        try {
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(this.necoWithdrawalService.balanceLiabilitiesSummary()));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
        }
    }

    private void handleInternalNecoWithdrawalsPendingConfirmation(RequestContext requestContext, String string) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        try {
            QueryStringDecoder queryStringDecoder = new QueryStringDecoder(string);
            int n = 100;
            List<String> list = queryStringDecoder.parameters().get("limit");
            if (list != null && !list.isEmpty() && !list.get(0).isBlank()) {
                n = Integer.parseInt(list.get(0));
            }
            if (n <= 0 || n > 1000) {
                throw new IllegalArgumentException("limit must be between 1 and 1000");
            }
            List<NecoWithdrawalResponse> list2 = this.necoWithdrawalService.listBroadcastedWithdrawalsPendingConfirmation(n);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list2));
        }
        catch (NumberFormatException numberFormatException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "limit must be an integer");
        }
        catch (IllegalArgumentException illegalArgumentException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(illegalArgumentException));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.CONFLICT, ERR_REJECTED, illegalStateException.getMessage());
        }
    }

    private void handleInternalNecoWithdrawalClaim(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        String string = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            NecoWithdrawalResponse necoWithdrawalResponse;
            ObjectNode objectNode = string.isBlank() ? MAPPER.createObjectNode() : (ObjectNode)MAPPER.readTree(string);
            int n = RestApiHandler.intField(objectNode, "maxAttempts", 3);
            if (n <= 0 || n > 100) {
                throw new IllegalArgumentException("maxAttempts must be between 1 and 100");
            }
            Set<Long> set = RestApiHandler.targetChainIdsField(objectNode);
            NecoWithdrawalResponse necoWithdrawalResponse2 = necoWithdrawalResponse = set.isEmpty() ? this.necoWithdrawalService.claimNextReservedWithdrawal(n) : this.necoWithdrawalService.claimNextReservedWithdrawal(n, set);
            if (necoWithdrawalResponse == null) {
                this.sendNoContent(requestContext);
                return;
            }
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(necoWithdrawalResponse));
        }
        catch (IOException | IllegalArgumentException exception) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(exception));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.CONFLICT, ERR_REJECTED, illegalStateException.getMessage());
        }
    }

    private void handleInternalNecoWithdrawalStatus(RequestContext requestContext, String string, FullHttpRequest fullHttpRequest) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        String string2 = new QueryStringDecoder(string).path();
        String string3 = string2.substring("/api/v1/internal/withdrawals/neco/".length());
        String[] stringArray = string3.split("/", 2);
        if (stringArray.length != 2 || stringArray[0].isBlank() || stringArray[1].isBlank()) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "withdrawalId and action are required");
            return;
        }
        String string4 = stringArray[0].trim();
        String string5 = stringArray[1].trim();
        String string6 = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            NecoWithdrawalResponse necoWithdrawalResponse;
            ObjectNode objectNode = string6.isBlank() ? MAPPER.createObjectNode() : (ObjectNode)MAPPER.readTree(string6);
            switch (string5) {
                case "broadcasted": {
                    String string7 = RestApiHandler.textField(objectNode, "txHash");
                    if (string7 == null) {
                        throw new IllegalArgumentException("txHash is required");
                    }
                    necoWithdrawalResponse = this.necoWithdrawalService.markWithdrawalBroadcasted(string4, string7);
                    break;
                }
                case "confirmed": {
                    String string8 = RestApiHandler.textField(objectNode, "txHash");
                    if (string8 == null) {
                        throw new IllegalArgumentException("txHash is required");
                    }
                    necoWithdrawalResponse = this.necoWithdrawalService.markWithdrawalConfirmed(string4, string8);
                    break;
                }
                case "retry": {
                    necoWithdrawalResponse = this.necoWithdrawalService.releaseWithdrawalForRetry(string4, RestApiHandler.optionalTextField(objectNode, "reason"));
                    break;
                }
                case "failed": {
                    necoWithdrawalResponse = this.necoWithdrawalService.markWithdrawalFailed(string4, RestApiHandler.optionalTextField(objectNode, "reason"));
                    break;
                }
                case "refund": {
                    necoWithdrawalResponse = this.necoWithdrawalService.refundFailedWithdrawal(string4, RestApiHandler.optionalTextField(objectNode, "reason"));
                    break;
                }
                default: {
                    this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Not Found");
                    return;
                }
            }
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(necoWithdrawalResponse));
        }
        catch (IOException | IllegalArgumentException exception) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(exception));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.CONFLICT, ERR_REJECTED, illegalStateException.getMessage());
        }
    }

    private void handleRequestNecoWithdrawal(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        Principal principal = this.principal(requestContext);
        if (principal == null) {
            this.sendError(requestContext, HttpResponseStatus.UNAUTHORIZED, ERR_UNAUTHORIZED, "Unauthorized");
            return;
        }
        if (!this.canPerformAs(requestContext, Authorizer.ACTION_FUND_AS_USER, principal.userId())) {
            this.sendForbidden(requestContext, principal.userId());
            return;
        }
        String string = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            JsonNode jsonNode = MAPPER.readTree(string);
            String string2 = RestApiHandler.textField(jsonNode, "clientWithdrawalId");
            String string3 = RestApiHandler.textField(jsonNode, "destinationAddress");
            JsonNode jsonNode2 = jsonNode.get("assetId");
            long l = RestApiHandler.targetChainIdField(jsonNode);
            Long l2 = RestApiHandler.money(jsonNode, "amount");
            if (!(string2 != null && string3 != null && RestApiHandler.isEvmAddress(string3) && jsonNode2 != null && jsonNode2.isNumber() && jsonNode2.canConvertToExactIntegral() && jsonNode2.asInt() >= 0 && jsonNode2.asInt() <= 1000000 && l2 != null && RestApiHandler.validMoney(l2))) {
                throw new IllegalArgumentException("clientWithdrawalId, destinationAddress, assetId, and amount are required");
            }
            NecoWithdrawalRequest necoWithdrawalRequest = new NecoWithdrawalRequest(principal.userId(), string2, RestApiHandler.normalizeEvmAddress(string3), jsonNode2.asInt(), FixedPoint.format(l2), l2, l);
            NecoWithdrawalResponse necoWithdrawalResponse = this.necoWithdrawalService.requestWithdrawal(necoWithdrawalRequest);
            this.audit(requestContext, "neco.withdrawal.request", "user:" + principal.userId(), true, "withdrawalId=" + necoWithdrawalResponse.getWithdrawalId() + " assetId=" + necoWithdrawalResponse.getAssetId() + " amount=" + necoWithdrawalResponse.getAmount());
            this.sendResponse(requestContext, necoWithdrawalResponse.isDuplicate() ? HttpResponseStatus.OK : HttpResponseStatus.CREATED, MAPPER.writeValueAsString(necoWithdrawalResponse));
        }
        catch (IOException | IllegalArgumentException exception) {
            this.audit(requestContext, "neco.withdrawal.request", "user:" + principal.userId(), false, "invalid input");
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(exception));
        }
        catch (IllegalStateException illegalStateException) {
            this.audit(requestContext, "neco.withdrawal.request", "user:" + principal.userId(), false, illegalStateException.getMessage());
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_REJECTED, illegalStateException.getMessage());
        }
    }

    private void handleGetNecoWithdrawal(RequestContext requestContext, String string) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        Principal principal = this.principal(requestContext);
        if (principal == null) {
            this.sendError(requestContext, HttpResponseStatus.UNAUTHORIZED, ERR_UNAUTHORIZED, "Unauthorized");
            return;
        }
        if (!this.canPerformAs(requestContext, Authorizer.ACTION_FUND_AS_USER, principal.userId())) {
            this.sendForbidden(requestContext, principal.userId());
            return;
        }
        String string2 = new QueryStringDecoder(string).path();
        String string3 = string2.substring("/api/v1/neco/withdrawals/".length()).trim();
        if (string3.isEmpty()) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "withdrawalId required");
            return;
        }
        NecoWithdrawalResponse necoWithdrawalResponse = this.necoWithdrawalService.getWithdrawal(principal.userId(), string3);
        if (necoWithdrawalResponse == null) {
            this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Withdrawal not found");
            return;
        }
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(necoWithdrawalResponse));
    }

    private void handleAdminNecoWithdrawals(RequestContext requestContext, String string) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        QueryStringDecoder queryStringDecoder = new QueryStringDecoder(string);
        Integer n = this.intParam(requestContext, queryStringDecoder, "limit", 100);
        if (n == null) {
            return;
        }
        Integer n2 = this.intParam(requestContext, queryStringDecoder, "offset", 0);
        if (n2 == null) {
            return;
        }
        try {
            List<NecoWithdrawalResponse> list = this.necoWithdrawalService.listWithdrawalsForAdmin(RestApiHandler.firstParam(queryStringDecoder, "status"), n, n2);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list));
        }
        catch (IllegalArgumentException illegalArgumentException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(illegalArgumentException));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
        }
    }

    private void handleAdminNecoWithdrawalSummary(RequestContext requestContext) throws Exception {
        if (this.necoWithdrawalService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, "Neco withdrawal service unavailable");
            return;
        }
        try {
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(this.necoWithdrawalService.withdrawalStatusSummary()));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
        }
    }

    private static String optionalTextField(JsonNode jsonNode, String string) {
        JsonNode jsonNode2 = jsonNode.get(string);
        if (jsonNode2 == null || jsonNode2.isNull()) {
            return null;
        }
        if (!jsonNode2.isTextual()) {
            throw new IllegalArgumentException(string + " must be a string");
        }
        String string2 = jsonNode2.textValue().trim();
        return string2.isEmpty() ? null : string2;
    }

    private static int intField(JsonNode jsonNode, String string, int n) {
        JsonNode jsonNode2 = jsonNode.get(string);
        if (jsonNode2 == null || jsonNode2.isNull()) {
            return n;
        }
        if (!jsonNode2.isNumber() || !jsonNode2.canConvertToExactIntegral()) {
            throw new IllegalArgumentException(string + " must be an integer");
        }
        return jsonNode2.asInt();
    }

    private static Set<Long> targetChainIdsField(JsonNode jsonNode) {
        JsonNode jsonNode2 = jsonNode.get("targetChainIds");
        if (jsonNode2 == null || jsonNode2.isNull()) {
            return Set.of();
        }
        if (!jsonNode2.isArray() || jsonNode2.isEmpty()) {
            throw new IllegalArgumentException("targetChainIds must be a non-empty integer array");
        }
        LinkedHashSet<Long> linkedHashSet = new LinkedHashSet<Long>();
        for (JsonNode jsonNode3 : jsonNode2) {
            if (!jsonNode3.isIntegralNumber() || !jsonNode3.canConvertToLong()) {
                throw new IllegalArgumentException("targetChainIds must contain only integers");
            }
            long l = jsonNode3.asLong();
            if (l != 7260L && l != 56L) {
                throw new IllegalArgumentException("unsupported target chain: " + l);
            }
            linkedHashSet.add(l);
        }
        return Set.copyOf(linkedHashSet);
    }

    private static long targetChainIdField(JsonNode jsonNode) {
        JsonNode jsonNode2 = jsonNode.get("targetChainId");
        if (jsonNode2 == null || jsonNode2.isNull()) {
            return 7260L;
        }
        if (!jsonNode2.isIntegralNumber() || !jsonNode2.canConvertToLong() || jsonNode2.asLong() <= 0L) {
            throw new IllegalArgumentException("targetChainId must be a positive integer");
        }
        return jsonNode2.asLong();
    }

    private static String textField(JsonNode jsonNode, String string) {
        JsonNode jsonNode2 = jsonNode.get(string);
        if (jsonNode2 == null || !jsonNode2.isTextual()) {
            return null;
        }
        String string2 = jsonNode2.textValue().trim();
        return string2.isEmpty() ? null : string2;
    }

    private static String normalizeEvmAddress(String string) {
        return string == null ? "" : string.trim().toLowerCase(Locale.ROOT);
    }

    private static boolean isEvmAddress(String string) {
        String string2 = RestApiHandler.normalizeEvmAddress(string);
        if (string2.length() != 42 || !string2.startsWith("0x")) {
            return false;
        }
        for (int i = 2; i < string2.length(); ++i) {
            char c = string2.charAt(i);
            if (c >= '0' && c <= '9' || c >= 'a' && c <= 'f') continue;
            return false;
        }
        return true;
    }

    private void sendForbidden(RequestContext requestContext, long l) {
        this.sendError(requestContext, HttpResponseStatus.FORBIDDEN, ERR_FORBIDDEN, "Forbidden: cannot act as user " + l);
    }

    private boolean deniedOrderAccess(RequestContext requestContext, long l) {
        OrderResponse orderResponse = this.orderService.getOrder(l);
        if (orderResponse != null && !this.canPerformAs(
                requestContext, Authorizer.ACTION_TRADE_AS_USER, orderResponse.getUserId())) {
            this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Order not found");
            return true;
        }
        return false;
    }

    private static Long money(JsonNode jsonNode, String string) {
        JsonNode jsonNode2 = jsonNode.get(string);
        if (jsonNode2 == null || jsonNode2.isNull()) {
            return null;
        }
        if (jsonNode2.isTextual()) {
            try {
                return FixedPoint.parse(jsonNode2.textValue());
            }
            catch (FixedPoint.OverflowException | NumberFormatException runtimeException) {
                throw new IllegalArgumentException(string + " must be a decimal string with at most 8 fractional digits");
            }
        }
        if (jsonNode2.isNumber()) {
            double d = jsonNode2.doubleValue();
            if (!Double.isFinite(d)) {
                throw new IllegalArgumentException(string + " must be finite");
            }
            return FixedPoint.fromDouble(d);
        }
        throw new IllegalArgumentException(string + " must be a decimal string");
    }

    private static boolean validMoney(long l) {
        return l > 0L && l <= 9000000000000000000L;
    }

    private static boolean validOptionalMoney(long l) {
        return l == 0L || RestApiHandler.validMoney(l);
    }

    private void audit(RequestContext requestContext, String string, String string2, boolean bl, String string3) {
        this.auditLog.record(this.principal(requestContext), string, string2, bl, string3);
    }

    private void handleHealth(RequestContext requestContext) throws Exception {
        ObjectNode objectNode = MAPPER.createObjectNode();
        objectNode.put("status", "ok");
        objectNode.put("clusterConnected", this.orderService.isClusterConnected());
        objectNode.put("activeOrders", this.orderService.getActiveOrderCount());
        Boolean bl = this.orderService.isAssetsProjectionReady();
        if (bl != null) {
            objectNode.put("assetsProjectionReady", bl);
        }
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(objectNode));
    }

    private void handleCreateOrder(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        long l;
        CreateOrderRequest createOrderRequest;
        String string = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            createOrderRequest = MAPPER.readValue(string, CreateOrderRequest.class);
        }
        catch (IOException iOException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(iOException));
            return;
        }
        Principal principal = this.principal(requestContext);
        long l2 = l = principal == null ? createOrderRequest.getUserId() : principal.resolveUserId(createOrderRequest.getUserId());
        if (!this.canPerformAs(requestContext, Authorizer.ACTION_TRADE_AS_USER, l)) {
            this.audit(requestContext, "order.create", "user:" + l, false, "forbidden");
            this.sendForbidden(requestContext, l);
            return;
        }
        createOrderRequest.setUserId(l);
        String string2 = RestApiHandler.validateCreateOrder(createOrderRequest);
        if (string2 != null) {
            this.audit(requestContext, "order.create", "user:" + l, false, string2);
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, string2);
            return;
        }
        CreateOrderResponse createOrderResponse = this.orderService.createOrder(createOrderRequest);
        Counter.builder("oms_orders_total").description("Order submissions by outcome").tag("result", createOrderResponse.isDuplicate() ? "duplicate" : (createOrderResponse.isAccepted() ? "accepted" : "rejected")).tag("reason", createOrderResponse.isAccepted() ? "none" : (createOrderResponse.getRejectReason() != null ? createOrderResponse.getRejectReason().replace(' ', '_') : "unknown")).register(this.meterRegistry).increment();
        this.audit(requestContext, "order.create", "user:" + l, createOrderResponse.isAccepted(), (String)(createOrderResponse.isAccepted() ? "omsOrderId=" + createOrderResponse.getOmsOrderId() + (createOrderResponse.isDuplicate() ? " duplicate" : "") : createOrderResponse.getRejectReason()));
        HttpResponseStatus httpResponseStatus = !createOrderResponse.isAccepted() ? HttpResponseStatus.BAD_REQUEST : (createOrderResponse.isDuplicate() ? HttpResponseStatus.OK : HttpResponseStatus.CREATED);
        this.sendResponse(requestContext, httpResponseStatus, MAPPER.writeValueAsString(createOrderResponse));
    }

    private static String validateCreateOrder(CreateOrderRequest createOrderRequest) {
        if (createOrderRequest.getClientOrderId() != null && createOrderRequest.getClientOrderId().length() > 64) {
            return "clientOrderId too long (max 64)";
        }
        if (!RestApiHandler.validMoney(createOrderRequest.getQuantity())) {
            return "quantity must be a positive decimal";
        }
        if (!RestApiHandler.validOptionalMoney(createOrderRequest.getPrice())) {
            return "price must be a positive decimal";
        }
        if (!RestApiHandler.validOptionalMoney(createOrderRequest.getStopPrice())) {
            return "stopPrice must be a positive decimal";
        }
        if (!RestApiHandler.validOptionalMoney(createOrderRequest.getTrailingDelta())) {
            return "trailingDelta must be a positive decimal";
        }
        if (!RestApiHandler.validOptionalMoney(createOrderRequest.getDisplayQuantity())) {
            return "displayQuantity must be a positive decimal";
        }
        return null;
    }

    private void handleCancelOrder(RequestContext requestContext, String string) throws Exception {
        String string2 = string.substring("/api/v1/orders/".length());
        long l = Long.parseLong(string2);
        if (this.deniedOrderAccess(requestContext, l)) {
            return;
        }
        CancelOrderResponse cancelOrderResponse = this.orderService.cancelOrder(l);
        this.audit(requestContext, "order.cancel", "order:" + l, cancelOrderResponse.isAccepted(), cancelOrderResponse.getMessage());
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(cancelOrderResponse));
    }

    private void handleUpdateOrder(RequestContext requestContext, FullHttpRequest fullHttpRequest, String string) throws Exception {
        long l;
        long l2;
        Map<String, Object> object;
        String string2 = string.substring("/api/v1/orders/".length());
        long l3 = Long.parseLong(string2);
        if (this.deniedOrderAccess(requestContext, l3)) {
            return;
        }
        String string3 = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            JsonNode params = MAPPER.readTree(string3);
            Long l4 = RestApiHandler.money(params, "price");
            Long quantity = RestApiHandler.money(params, "quantity");
            l2 = l4 != null ? l4 : 0L;
            l = quantity != null ? quantity : 0L;
        }
        catch (IOException | IllegalArgumentException exception) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(exception));
            return;
        }
        if (l2 <= 0L && l <= 0L) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "At least one of price or quantity must be provided");
            return;
        }
        if (!RestApiHandler.validOptionalMoney(l2) || !RestApiHandler.validOptionalMoney(l)) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "price/quantity must be positive decimals");
            return;
        }
        object = this.orderService.updateOrder(l3, l2, l);
        boolean bl = Boolean.TRUE.equals(object.get("accepted"));
        this.audit(requestContext, "order.update", "order:" + l3, bl, "price=" + FixedPoint.format(l2) + " quantity=" + FixedPoint.format(l));
        HttpResponseStatus status = bl ? HttpResponseStatus.OK : HttpResponseStatus.BAD_REQUEST;
        this.sendResponse(requestContext, status, MAPPER.writeValueAsString(object));
    }

    private void handleGetOrder(RequestContext requestContext, String string) throws Exception {
        String string2 = string.substring("/api/v1/orders/".length());
        long l = Long.parseLong(string2);
        OrderResponse orderResponse = this.orderService.getOrder(l);
        if (orderResponse == null || !this.canPerformAs(
                requestContext, Authorizer.ACTION_READ_AS_USER, orderResponse.getUserId())) {
            this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Order not found");
        } else {
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(orderResponse));
        }
    }

    private void handleQueryOrders(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        QueryStringDecoder queryStringDecoder = new QueryStringDecoder(fullHttpRequest.uri());
        Long l = this.resolveQueryUserId(requestContext, queryStringDecoder);
        if (l == null) {
            return;
        }
        List<OrderResponse> list = this.orderService.queryOrders(l, RestApiHandler.firstParam(queryStringDecoder, "status"));
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list));
    }

    private void handleOrderHistory(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        Integer n;
        QueryStringDecoder queryStringDecoder = new QueryStringDecoder(fullHttpRequest.uri());
        Long l = this.resolveQueryUserId(requestContext, queryStringDecoder);
        if (l == null) {
            return;
        }
        String string = RestApiHandler.firstParam(queryStringDecoder, "status");
        Integer n2 = this.intParam(requestContext, queryStringDecoder, "limit", 100);
        Integer n3 = n = n2 != null ? this.intParam(requestContext, queryStringDecoder, "offset", 0) : null;
        if (n == null) {
            return;
        }
        try {
            List<OrderResponse> list = this.orderService.getOrderHistory(l, string, n2, n);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list));
        }
        catch (IllegalArgumentException illegalArgumentException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "Unknown status: " + string);
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
        }
    }

    private void handleExecutions(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        Integer n;
        QueryStringDecoder queryStringDecoder = new QueryStringDecoder(fullHttpRequest.uri());
        Long l = this.resolveQueryUserId(requestContext, queryStringDecoder);
        if (l == null) {
            return;
        }
        Integer n2 = this.intParam(requestContext, queryStringDecoder, "limit", 100);
        Integer n3 = n = n2 != null ? this.intParam(requestContext, queryStringDecoder, "offset", 0) : null;
        if (n == null) {
            return;
        }
        try {
            List<ExecutionResponse> list = this.orderService.getExecutions(l, n2, n);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
        }
    }

    private void handlePositions(RequestContext requestContext, FullHttpRequest fullHttpRequest) throws Exception {
        QueryStringDecoder queryStringDecoder = new QueryStringDecoder(fullHttpRequest.uri());
        Long l = this.resolveQueryUserId(requestContext, queryStringDecoder);
        if (l == null) {
            return;
        }
        try {
            List<PositionResponse> list = this.orderService.getPositions(l);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list));
        }
        catch (IllegalStateException illegalStateException) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_UNAVAILABLE, illegalStateException.getMessage());
        }
    }

    private Long resolveQueryUserId(RequestContext requestContext, QueryStringDecoder queryStringDecoder) {
        long l;
        String string = RestApiHandler.firstParam(queryStringDecoder, "userId");
        Principal principal = this.principal(requestContext);
        if (string == null && principal == null) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "userId required");
            return null;
        }
        try {
            l = string != null ? Long.parseLong(string) : principal.userId();
        }
        catch (NumberFormatException numberFormatException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "userId must be an integer");
            return null;
        }
        if (!this.canPerformAs(requestContext, Authorizer.ACTION_READ_AS_USER, l)) {
            this.sendForbidden(requestContext, l);
            return null;
        }
        return l;
    }

    private static String firstParam(QueryStringDecoder queryStringDecoder, String string) {
        List<String> list = queryStringDecoder.parameters().get(string);
        return list != null && !list.isEmpty() ? (String)list.getFirst() : null;
    }

    private Integer intParam(RequestContext requestContext, QueryStringDecoder queryStringDecoder, String string, int n) {
        String string2 = RestApiHandler.firstParam(queryStringDecoder, string);
        if (string2 == null) {
            return n;
        }
        try {
            int n2 = Integer.parseInt(string2);
            if (n2 < 0) {
                throw new NumberFormatException();
            }
            return n2;
        }
        catch (NumberFormatException numberFormatException) {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, string + " must be a non-negative integer");
            return null;
        }
    }

    private void handleGetAccount(RequestContext requestContext, String string) throws Exception {
        long l;
        String string2 = string.substring("/api/v1/accounts/".length());
        int n = string2.indexOf(47);
        if (n > 0) {
            string2 = string2.substring(0, n);
        }
        if (!this.canPerformAs(
                requestContext, Authorizer.ACTION_READ_AS_USER, l = Long.parseLong(string2))) {
            this.sendForbidden(requestContext, l);
            return;
        }
        Map<String, Object> map = this.orderService.getBalances(l);
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(map));
    }

    private void handleGetBalances(RequestContext requestContext) throws Exception {
        Principal principal = this.principal(requestContext);
        if (principal == null) {
            this.sendError(requestContext, HttpResponseStatus.UNAUTHORIZED,
                    ERR_UNAUTHORIZED, "Unauthorized");
            return;
        }
        if (!this.canPerformAs(
                requestContext, Authorizer.ACTION_READ_AS_USER, principal.userId())) {
            this.sendForbidden(requestContext, principal.userId());
            return;
        }
        Map<String, Object> map = this.orderService.getBalances(principal.userId());
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(map));
    }

    private void handleDeposit(RequestContext requestContext, String string, FullHttpRequest fullHttpRequest) throws Exception {
        long l = this.extractAccountUserId(string);
        if (!this.canMoveBalancesManually(requestContext, l)) {
            this.audit(requestContext, "account.deposit", "user:" + l, false, "admin role required");
            this.sendError(requestContext, HttpResponseStatus.FORBIDDEN, ERR_FORBIDDEN, "Admin role required for manual balance movement");
            return;
        }
        long[] lArray = this.parseAssetAmount(requestContext, fullHttpRequest, "account.deposit", l);
        if (lArray == null) {
            return;
        }
        int n = (int)lArray[0];
        long l2 = lArray[1];
        this.orderService.deposit(l, n, l2);
        String string2 = FixedPoint.format(l2);
        this.audit(requestContext, "account.deposit", "user:" + l, true, "assetId=" + n + " amount=" + string2);
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(this.moneyMovementResponse(l, n, string2)));
    }

    private void handleWithdraw(RequestContext requestContext, String string, FullHttpRequest fullHttpRequest) throws Exception {
        long l = this.extractAccountUserId(string);
        if (!this.canMoveBalancesManually(requestContext, l)) {
            this.audit(requestContext, "account.withdraw", "user:" + l, false, "admin role required");
            this.sendError(requestContext, HttpResponseStatus.FORBIDDEN, ERR_FORBIDDEN, "Admin role required for manual balance movement");
            return;
        }
        long[] lArray = this.parseAssetAmount(requestContext, fullHttpRequest, "account.withdraw", l);
        if (lArray == null) {
            return;
        }
        int n = (int)lArray[0];
        long l2 = lArray[1];
        String string2 = FixedPoint.format(l2);
        try {
            this.orderService.withdraw(l, n, l2);
            this.audit(requestContext, "account.withdraw", "user:" + l, true, "assetId=" + n + " amount=" + string2);
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(this.moneyMovementResponse(l, n, string2)));
        }
        catch (IllegalStateException illegalStateException) {
            this.audit(requestContext, "account.withdraw", "user:" + l, false, illegalStateException.getMessage());
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_REJECTED, illegalStateException.getMessage());
        }
    }

    private long[] parseAssetAmount(RequestContext requestContext, FullHttpRequest fullHttpRequest, String string, long l) throws Exception {
        String string2 = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        try {
            JsonNode jsonNode = MAPPER.readTree(string2);
            JsonNode jsonNode2 = jsonNode.get("assetId");
            Long l2 = RestApiHandler.money(jsonNode, "amount");
            if (!(jsonNode2 != null && jsonNode2.isNumber() && jsonNode2.canConvertToExactIntegral() && jsonNode2.asInt() >= 0 && jsonNode2.asInt() <= 1000000 && l2 != null && RestApiHandler.validMoney(l2))) {
                throw new IllegalArgumentException("assetId (non-negative integer) and amount (positive decimal string) required");
            }
            return new long[]{jsonNode2.asInt(), l2};
        }
        catch (IOException | IllegalArgumentException exception) {
            this.audit(requestContext, string, "user:" + l, false, "invalid input");
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, RestApiHandler.rootMessage(exception));
            return null;
        }
    }

    private ObjectNode moneyMovementResponse(long l, int n, String string) {
        ObjectNode objectNode = MAPPER.createObjectNode();
        objectNode.put("success", true);
        objectNode.put("userId", l);
        objectNode.put("assetId", n);
        objectNode.put("amount", string);
        return objectNode;
    }

    private long extractAccountUserId(String string) {
        String string2 = string.substring("/api/v1/accounts/".length());
        int n = string2.indexOf(47);
        return Long.parseLong(string2.substring(0, n));
    }

    private void handleGetMarkets(RequestContext requestContext) throws Exception {
        List<Map<String, Object>> list = this.orderService.getMarkets();
        this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(list));
    }

    private void handleGetRiskConfig(RequestContext requestContext, String string) throws Exception {
        if (this.adminService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_ADMIN_UNAVAILABLE, "Admin not available");
            return;
        }
        if (string.equals("/api/v1/admin/risk/config")) {
            Map<String, Map<String, Object>> map = this.adminService.getAllRiskConfigs();
            this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(map));
        } else {
            String string2 = string.substring("/api/v1/admin/risk/config/".length());
            int n = Integer.parseInt(string2);
            Map<String, Object> map = this.adminService.getRiskConfig(n);
            if (map == null) {
                this.sendError(requestContext, HttpResponseStatus.NOT_FOUND, ERR_NOT_FOUND, "Market not configured");
            } else {
                this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(map));
            }
        }
    }

    private void handleUpdateRiskConfig(RequestContext requestContext, String string, FullHttpRequest fullHttpRequest) throws Exception {
        if (this.adminService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_ADMIN_UNAVAILABLE, "Admin not available");
            return;
        }
        String string2 = string.substring("/api/v1/admin/risk/config/".length());
        int n = Integer.parseInt(string2);
        String string3 = fullHttpRequest.content().toString(StandardCharsets.UTF_8);
        Map map = MAPPER.readValue(string3, Map.class);
        this.adminService.updateRiskConfig(n, map);
        this.audit(requestContext, "admin.risk.update", "market:" + n, true, String.valueOf(map.keySet()));
        RISK_PERSIST_EXECUTOR.execute(() -> {
            try {
                boolean bl = this.adminService.persistRiskConfig(n);
                ObjectNode objectNode = MAPPER.createObjectNode();
                objectNode.put("success", true);
                objectNode.put("marketId", n);
                objectNode.put("persisted", bl);
                this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(objectNode));
            }
            catch (Exception exception) {
                log.error("Risk config persist dispatch failed for market {}", (Object)n, (Object)exception);
                this.sendError(requestContext, HttpResponseStatus.INTERNAL_SERVER_ERROR, ERR_INTERNAL, RestApiHandler.rootMessage(exception));
            }
        });
    }

    private void handleCircuitBreaker(RequestContext requestContext, String string) throws Exception {
        boolean bl;
        if (this.adminService == null) {
            this.sendError(requestContext, HttpResponseStatus.SERVICE_UNAVAILABLE, ERR_ADMIN_UNAVAILABLE, "Admin not available");
            return;
        }
        String string2 = string.substring("/api/v1/admin/risk/circuit-breaker/".length());
        String[] stringArray = string2.split("/");
        int n = Integer.parseInt(stringArray[0]);
        String string3 = stringArray[1];
        if ("trip".equals(string3)) {
            this.adminService.tripCircuitBreaker(n);
            bl = true;
        } else if ("reset".equals(string3)) {
            this.adminService.resetCircuitBreaker(n);
            bl = false;
        } else {
            this.sendError(requestContext, HttpResponseStatus.BAD_REQUEST, ERR_VALIDATION, "Unknown action: " + string3);
            return;
        }
        this.audit(requestContext, "admin.circuit-breaker", "market:" + n, true, string3);
        RISK_PERSIST_EXECUTOR.execute(() -> {
            try {
                boolean bl2 = this.adminService.persistManualTrip(n, bl);
                ObjectNode objectNode = MAPPER.createObjectNode();
                objectNode.put("success", true);
                objectNode.put("marketId", n);
                objectNode.put("action", string3);
                objectNode.put("persisted", bl2);
                this.sendResponse(requestContext, HttpResponseStatus.OK, MAPPER.writeValueAsString(objectNode));
            }
            catch (Exception exception) {
                log.error("Circuit-breaker persist dispatch failed for market {}", (Object)n, (Object)exception);
                this.sendError(requestContext, HttpResponseStatus.INTERNAL_SERVER_ERROR, ERR_INTERNAL, RestApiHandler.rootMessage(exception));
            }
        });
    }

    private void sendResponse(RequestContext requestContext, HttpResponseStatus httpResponseStatus, String string) {
        byte[] byArray = string.getBytes(StandardCharsets.UTF_8);
        ByteBuf byteBuf = Unpooled.wrappedBuffer(byArray);
        DefaultFullHttpResponse defaultFullHttpResponse = new DefaultFullHttpResponse(HttpVersion.HTTP_1_1, httpResponseStatus, byteBuf);
        defaultFullHttpResponse.headers().set((CharSequence)HttpHeaderNames.CONTENT_TYPE, (Object)"application/json");
        defaultFullHttpResponse.headers().set((CharSequence)HttpHeaderNames.CONTENT_LENGTH, (Object)byArray.length);
        this.corsPolicy.apply(requestContext.origin(), defaultFullHttpResponse);
        RestApiHandler.finishResponse(requestContext, defaultFullHttpResponse);
    }

    private void sendNoContent(RequestContext requestContext) {
        DefaultFullHttpResponse defaultFullHttpResponse = new DefaultFullHttpResponse(HttpVersion.HTTP_1_1, HttpResponseStatus.NO_CONTENT);
        defaultFullHttpResponse.headers().set((CharSequence)HttpHeaderNames.CONTENT_LENGTH, (Object)0);
        this.corsPolicy.apply(requestContext.origin(), defaultFullHttpResponse);
        RestApiHandler.finishResponse(requestContext, defaultFullHttpResponse);
    }

    private void sendCorsPreflightResponse(RequestContext requestContext) {
        DefaultFullHttpResponse defaultFullHttpResponse = new DefaultFullHttpResponse(HttpVersion.HTTP_1_1, HttpResponseStatus.NO_CONTENT);
        this.corsPolicy.apply(requestContext.origin(), defaultFullHttpResponse);
        defaultFullHttpResponse.headers().set((CharSequence)HttpHeaderNames.CONTENT_LENGTH, (Object)0);
        RestApiHandler.finishResponse(requestContext, defaultFullHttpResponse);
    }

    private static void finishResponse(RequestContext requestContext, FullHttpResponse fullHttpResponse) {
        HttpUtil.setKeepAlive(fullHttpResponse, requestContext.keepAlive());
        ChannelFuture channelFuture = requestContext.ctx().writeAndFlush(fullHttpResponse);
        if (!requestContext.keepAlive()) {
            channelFuture.addListener((GenericFutureListener)ChannelFutureListener.CLOSE);
        }
    }

    private static String escapeJson(String string) {
        if (string == null) {
            return "null";
        }
        return string.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private void sendError(RequestContext requestContext, HttpResponseStatus httpResponseStatus, String string, String string2) {
        this.sendResponse(requestContext, httpResponseStatus, "{\"error\":\"" + RestApiHandler.escapeJson(string2) + "\",\"code\":\"" + string + "\"}");
    }

    private static String rootMessage(Throwable throwable) {
        Throwable throwable2;
        for (throwable2 = throwable; throwable2.getCause() != null && throwable2.getCause() != throwable2; throwable2 = throwable2.getCause()) {
        }
        String string = throwable2.getMessage();
        return string != null ? string : "malformed request body";
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext channelHandlerContext, Throwable throwable) {
        log.error("Unhandled exception in REST handler", throwable);
        channelHandlerContext.close();
    }

    private record RequestContext(ChannelHandlerContext ctx, String origin, boolean keepAlive) {
    }

    private static interface AuthCall {
        public AuthService.Session run(String var1, String var2);
    }
}

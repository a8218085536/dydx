# Unified Spot WebSocket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/ws/v1` anonymously accessible for spot candles, order books, and public trades while adding authenticated current-user order and execution events plus recent execution history on the same connection.

**Architecture:** OMS remains the owner of `/ws/v1` and private user routing. A shared OMS-side market-data bridge consumes the existing match-gateway WebSocket and REST snapshot APIs, then the OMS WebSocket handler filters and fans those frames out by market, channel, and candle interval. Authentication on upgrade becomes optional; private subscriptions resolve identity only from the authenticated channel principal.

**Tech Stack:** Java 21, Netty 4.2, Jackson, JDK `HttpClient`/`WebSocket`, Maven/JUnit/Mockito, React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-14-unified-spot-websocket-design.md`

## Global Constraints

- `/ws/v1` upgrade and public subscriptions must work without credentials.
- Missing, invalid, or expired credentials leave the connection anonymous; they do not reject the upgrade.
- Private identity comes only from `HttpAuthHandler.PRINCIPAL`; a payload `userId` cannot select another user.
- Public event calculations stay in match-gateway; OMS only snapshots, filters, and relays them.
- Existing `/spot/ws` behavior remains available during migration.
- Existing bare `OrderResponse` messages remain accepted for compatibility; new execution messages use explicit event envelopes.
- No session token may appear in URLs or logs.
- Do not overwrite or revert unrelated dirty worktree changes.
- Do not deploy the already-built intermediate match-gateway JAR; produce and deploy one final compatible build set.

## Review Focus

- Invalid `Sec-WebSocket-Protocol` token: connection opens anonymously and private subscription returns `AUTH_REQUIRED` without closing.
- Snapshot/live race: book, trades, and candle snapshots arrive before buffered live frames for that subscription.
- Market switch during asynchronous snapshot loading: stale snapshots and buffered frames from the previous market are discarded.
- Upstream reconnect: fresh snapshots re-anchor downstream books before resumed deltas.
- User isolation: forged `userId`, reconnects, and multiple tabs never deliver one user's history or live executions to another.

---

### Task 1: Make WebSocket Authentication Optional

**Files:**
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/HttpAuthHandler.java`
- Modify: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/auth/HttpAuthHandlerTest.java`

**Interfaces:**
- Consumes: existing `AuthenticationProvider.authenticate(Headers)` and `HttpAuthHandler.PRINCIPAL`.
- Produces: `/ws/v1` requests always forwarded; valid token sets `PRINCIPAL`, missing/invalid token leaves it unset; REST authentication remains unchanged.

- [ ] **Step 1: Write failing optional-auth tests**

Add tests named `wsUpgradeWithoutCredentialsPassesAsAnonymous`,
`wsUpgradeWithInvalidBearerPassesAsAnonymous`, and
`wsUpgradeWithValidBearerAttachesPrincipal`. Retain the assertion that query-string
tokens are never authenticated.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `mvn -pl oms-api -am -Dtest=HttpAuthHandlerTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: anonymous and invalid-token upgrade tests fail with HTTP 401 under current behavior.

- [ ] **Step 3: Implement optional authentication for `/ws/v1`**

Add path-specific handling in `channelRead`: attempt authentication when credentials
are present, attach the principal only on success, and forward the upgrade on missing
or invalid credentials. Keep `sendUnauthorized` for protected REST requests.

- [ ] **Step 4: Run the focused and REST-auth regression tests**

Run: `mvn -pl oms-api -am -Dtest=HttpAuthHandlerTest,RestApiHandlerAuthTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: PASS; unauthenticated REST remains 401.

- [ ] **Step 5: Commit**

```bash
git add openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/auth/HttpAuthHandler.java \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/auth/HttpAuthHandlerTest.java
git commit -m "feat: allow anonymous OMS websocket upgrades"
```

### Task 2: Add the Shared Match-Gateway Market-Data Bridge

**Files:**
- Create: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket/MarketDataSource.java`
- Create: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket/GatewayMarketDataSource.java`
- Create: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/websocket/GatewayMarketDataSourceTest.java`
- Modify: `openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsConfig.java`
- Modify: `openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/OmsConfigTest.java`

**Interfaces:**
- Produces: `MarketDataSource.start(IntStream marketIds, Consumer<String> liveFrameConsumer)`, `CompletionStage<List<String>> loadInitialFrames(int marketId, String interval, int tradeLimit, int candleLimit)`, and `close()`.
- Produces: `OmsConfig.marketGatewayUrl()` loaded from `OMS_MARKET_GATEWAY_URL`, default `http://127.0.0.1:8081`.
- Consumes: match-gateway `/ws`, `/api/orderbook`, `/api/trades`, and `/api/candles` without changing their payloads.

- [ ] **Step 1: Write failing bridge tests**

Cover URL construction, one shared upstream WebSocket per market, the upstream
`{"action":"subscribe","marketId":N}` message, REST snapshot paths, frame delivery,
bounded exponential reconnect, and clean shutdown. Use a local test HTTP/WebSocket
server; no external network dependency.

- [ ] **Step 2: Run the bridge tests and verify failure**

Run: `mvn -pl oms-api,oms-app -am -Dtest=GatewayMarketDataSourceTest,OmsConfigTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: FAIL because the bridge and configuration field do not exist.

- [ ] **Step 3: Define `MarketDataSource` and implement `GatewayMarketDataSource`**

Use one JDK WebSocket connection per configured market. Relay only recognized public
frames: `BOOK_SNAPSHOT`, `BOOK_DELTA`, `TRADES_BATCH`, `TICKER_STATS`,
`CANDLE_HISTORY`, `CANDLE_UPDATE`, `CLUSTER_STATUS`, and `CLUSTER_EVENT`.
Fetch per-subscriber initial book, recent trades, and requested-interval candles via
the existing REST APIs. Bound response size, timeouts, reconnect delay, and per-client
initialization buffers.

- [ ] **Step 4: Add and test `OMS_MARKET_GATEWAY_URL`**

Assert the default is `http://127.0.0.1:8081` and a system-property override
`oms.market.gateway.url` is honored by `OmsConfig.loadDefaults()`.

- [ ] **Step 5: Run focused tests**

Run: `mvn -pl oms-api,oms-app -am -Dtest=GatewayMarketDataSourceTest,OmsConfigTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: PASS with no outbound connection after `close()`.

- [ ] **Step 6: Commit**

```bash
git add openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/websocket/GatewayMarketDataSourceTest.java \
        openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsConfig.java \
        openexch/oms-main/oms-app/src/test/java/com/openexchange/oms/app/OmsConfigTest.java
git commit -m "feat: bridge match gateway data into OMS"
```

### Task 3: Add Anonymous Public Subscriptions to `/ws/v1`

**Files:**
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket/WebSocketHandler.java`
- Create: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/websocket/WebSocketHandlerTest.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/HttpServer.java`

**Interfaces:**
- Consumes: `MarketDataSource` from Task 2.
- Produces: public subscription `{"op":"subscribe","channels":["orderbook","trades","candles"],"marketId":1,"interval":"1m"}`.
- Produces: `SUBSCRIBED`, `AUTH_REQUIRED`, `INVALID_CHANNEL`, `INVALID_MARKET`, `INVALID_INTERVAL`, and `UPSTREAM_UNAVAILABLE` control messages without closing a healthy connection.

- [ ] **Step 1: Write failing public-subscription tests**

Test anonymous subscribe, initial snapshot before live frames, market/interval filters,
all six intervals, invalid inputs, unsubscribe, a market switch while the old snapshot
is pending, and bridge reconnect re-anchoring before deltas.

- [ ] **Step 2: Run tests and verify failure**

Run: `mvn -pl oms-api -am -Dtest=WebSocketHandlerTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: FAIL because the current handler requires a principal and supports only private channels.

- [ ] **Step 3: Split public and private subscription state in `WebSocketHandler`**

Track public subscriptions per connection by market/channel/interval. Register the
connection with `MarketDataSource`, hold live frames while initial snapshots load,
discard superseded initialization state, then flush buffered frames in arrival order.
Filter every relayed frame before writing it to a downstream channel.

- [ ] **Step 4: Keep private-channel rejection non-destructive**

For anonymous `orders`, `executions`, or `balances`, send `AUTH_REQUIRED` and keep the
socket open so existing public subscriptions continue. Never fall back to payload
`userId` when no principal exists.

- [ ] **Step 5: Run public WebSocket tests**

Run: `mvn -pl oms-api -am -Dtest=WebSocketHandlerTest,HttpAuthHandlerTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: PASS, including invalid-token public access and connection survival after `AUTH_REQUIRED`.

- [ ] **Step 6: Commit**

```bash
git add openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket/WebSocketHandler.java \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/websocket/WebSocketHandlerTest.java \
        openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/HttpServer.java
git commit -m "feat: serve public market data on ws v1"
```

### Task 4: Add Authenticated Execution History and Live Fills

**Files:**
- Modify: `openexch/oms-main/oms-core/src/main/java/com/openexchange/oms/core/OmsCoreEngine.java`
- Create: `openexch/oms-main/oms-core/src/test/java/com/openexchange/oms/core/OmsCoreEngineTest.java`
- Modify: `openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket/WebSocketHandler.java`
- Modify: `openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/websocket/WebSocketHandlerTest.java`
- Modify: `openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsApplication.java`

**Interfaces:**
- Produces: `OmsCoreEngine.setExecutionListener(Consumer<ExecutionReport>)` called once per newly applied maker/taker execution report and never for duplicate trade delivery.
- Produces: `EXECUTION_HISTORY` with authenticated user's recent `ExecutionResponse[]` before live `EXECUTION_REPORT` events.
- Preserves: existing bare `OrderResponse` events on the `orders` channel.

- [ ] **Step 1: Write failing execution-listener tests**

Assert maker and taker reports are emitted after a newly applied trade, duplicate trade
delivery emits nothing, and reports carry the correct user, side, price, quantity, and
maker flag.

- [ ] **Step 2: Run core tests and verify failure**

Run: `mvn -pl oms-core -am -Dtest=OmsCoreEngineTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: FAIL because no execution listener exists.

- [ ] **Step 3: Implement and wire the execution listener**

Notify the listener for each created report independently of persistence availability,
after idempotent settlement accepted the trade. In `OmsApplication`, wrap live events as
`{"type":"EXECUTION_REPORT","execution":...}` and call
`WebSocketHandler.pushToUser(userId, "executions", event)`.

- [ ] **Step 4: Write failing authenticated-history tests**

With a stub `OrderService`, assert authenticated execution subscribe returns only that
principal's `EXECUTION_HISTORY`, then live reports; forged `userId` is ignored/rejected;
two users and multiple tabs remain isolated. Assert unavailable persistence returns a
structured history error without inventing an empty history or closing the socket.

- [ ] **Step 5: Implement history delivery in `WebSocketHandler`**

Use `OrderService.getExecutions(principal.userId(), 100, 0)`. Send the snapshot before
registering the connection for live execution delivery, while buffering a fill that races
the query so no execution is lost between snapshot and stream.

- [ ] **Step 6: Run focused private-channel tests**

Run: `mvn -pl oms-core,oms-api,oms-app -am -Dtest=OmsCoreEngineTest,WebSocketHandlerTest -Dsurefire.failIfNoSpecifiedTests=false test`

Expected: PASS with strict per-user isolation and duplicate suppression.

- [ ] **Step 7: Commit**

```bash
git add openexch/oms-main/oms-core/src/main/java/com/openexchange/oms/core/OmsCoreEngine.java \
        openexch/oms-main/oms-core/src/test/java/com/openexchange/oms/core/OmsCoreEngineTest.java \
        openexch/oms-main/oms-api/src/main/java/com/openexchange/oms/api/websocket/WebSocketHandler.java \
        openexch/oms-main/oms-api/src/test/java/com/openexchange/oms/api/websocket/WebSocketHandlerTest.java \
        openexch/oms-main/oms-app/src/main/java/com/openexchange/oms/app/OmsApplication.java
git commit -m "feat: stream authenticated execution history and fills"
```

### Task 5: Migrate the Trading UI to One Optional-Auth Socket

**Files:**
- Modify: `openexch/trading-ui-main/src/hooks/useWebSocket.ts`
- Modify: `openexch/trading-ui-main/src/App.tsx`
- Modify: `openexch/trading-ui-main/src/types/market.ts`
- Modify: `openexch/trading-ui-main/src/__tests__/app.smoke.test.tsx`
- Create: `openexch/trading-ui-main/src/hooks/useWebSocket.test.ts`
- Remove after migration: `openexch/trading-ui-main/src/hooks/useOmsSocket.ts`
- Remove after migration: `openexch/trading-ui-main/src/hooks/useOmsSocket.test.ts`

**Interfaces:**
- Consumes: `/ws/v1` protocol from Tasks 3 and 4.
- Produces: one socket that is always open for public data and reconnects with the bearer subprotocol when session token changes.

- [ ] **Step 1: Write failing hook and smoke tests**

Assert signed-out startup opens `/ws/v1` without protocols and subscribes to public
channels; login reconnects with `['bearer', token]` and adds `orders`/`executions`;
logout reconnects anonymously; public trades and private order/execution events reach
their separate callbacks.

- [ ] **Step 2: Run frontend tests and verify failure**

Run: `npm test -- --run src/hooks/useWebSocket.test.ts src/__tests__/app.smoke.test.tsx`

Expected: FAIL because the UI currently opens separate market and OMS sockets.

- [ ] **Step 3: Extend `useWebSocket` for optional authentication and unified messages**

Change its URL to same-origin `/ws/v1`, accept `token`, public/private callbacks, and
selected candle interval, and use the Task 3 subscription schema. Preserve conflation
for market frames; bypass conflation for user order and execution events.

- [ ] **Step 4: Remove the second OMS socket from `App.tsx`**

Route public market frames through existing handlers, order events through
`handleOrderEvent`, execution history/live events through their account-history state,
and keep REST reseeding only as reconnect fallback.

- [ ] **Step 5: Run all frontend tests and production build**

Run: `npm test -- --run`

Run: `npm run build`

Expected: all tests and TypeScript production build PASS; exactly one WebSocket instance
is active per market page.

- [ ] **Step 6: Commit**

```bash
git add openexch/trading-ui-main/src
git commit -m "feat: use unified public and private websocket"
```

### Task 6: Documentation, Full Verification, and Production Deployment

**Files:**
- Modify: `openexch/oms-main/docs/API.md`
- Modify: `docs/neco-spot-frontend-api.zh-CN.md`
- Modify: production environment only: `OMS_MARKET_GATEWAY_URL=http://127.0.0.1:28082`

**Interfaces:**
- Documents: anonymous/public and authenticated/private subscription examples, response event types, reconnect behavior, and migration compatibility.

- [ ] **Step 1: Update API documentation**

Document exact JSON for public subscribe, authenticated subscribe, `TRADES_BATCH`,
`CANDLE_HISTORY`, `CANDLE_UPDATE`, `EXECUTION_HISTORY`, `EXECUTION_REPORT`, errors,
and browser bearer subprotocol use. Explicitly state that public data never requires login.

- [ ] **Step 2: Run complete local verification**

Run from `openexch/oms-main`:

`mvn test`

Run from `openexch/trading-ui-main`:

`npm test -- --run && npm run build`

Run from repository root:

`git diff --check`

Expected: all available tests/builds PASS; environment-dependent integration tests may
skip only when their declared local service is absent.

- [ ] **Step 3: Build final deployable artifacts**

Build the OMS application JAR and trading UI from the verified tree. Record SHA-256,
artifact size, and current production rollback hashes before uploading.

- [ ] **Step 4: Verify production prerequisites before restart**

Confirm match-gateway health on `127.0.0.1:28082`, OMS database availability, current
Nginx `/ws/v1` route, service-manager state, disk space, Java version, and rollback copy.
Set `OMS_MARKET_GATEWAY_URL=http://127.0.0.1:28082` in the existing OMS environment
without changing unrelated services.

- [ ] **Step 5: Deploy and verify anonymously**

Restart only the OMS/UI processes required by the final artifacts. Connect to public
`wss://testapp.flydex.ai/ws/v1` without a token and verify book snapshot/deltas,
`TRADES_BATCH` initial/live data, requested candle history/live updates, ping/pong, and
that `/spot/ws` still works.

- [ ] **Step 6: Verify authenticated isolation**

Using a real session without printing its token, verify current-user order status,
`EXECUTION_HISTORY`, and a small controlled fill's `EXECUTION_REPORT`; verify an
anonymous private subscription returns `AUTH_REQUIRED` while public streaming continues.

- [ ] **Step 7: Commit documentation and record deployment evidence**

```bash
git add openexch/oms-main/docs/API.md docs/neco-spot-frontend-api.zh-CN.md
git commit -m "docs: document unified spot websocket"
```

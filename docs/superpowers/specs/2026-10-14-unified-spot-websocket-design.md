# Unified Spot WebSocket Design

## Goal

Expose one browser-facing WebSocket endpoint, `/ws/v1`, for both public spot
market data and authenticated user events.

Connecting to `/ws/v1` must never require login. Authentication is optional and
only unlocks private channels belonging to the authenticated user.

## Required Behavior

### Anonymous connection

An anonymous client can connect and subscribe to:

- candle history and live candle updates;
- order-book snapshots and deltas;
- latest public market trades.

No account, balance, order, execution, or private history data is sent to an
anonymous client.

### Authenticated connection

A client may provide its existing session token during the WebSocket upgrade.
After successful authentication, the same connection can subscribe to all
public data plus:

- current-user order acceptance, rejection, cancellation, and status updates;
- current-user execution/fill events;
- a recent current-user execution-history snapshot after subscription.

The server derives user identity exclusively from the authenticated session.
It must ignore or reject a client-supplied `userId` for private subscriptions.

### Invalid or expired token

An invalid or expired token does not prevent the WebSocket connection from
opening. The connection remains anonymous and public subscriptions continue to
work. A private subscription request receives a structured authentication
error without closing the connection.

## Endpoint and Compatibility

The canonical endpoint is:

```text
WS /ws/v1
```

The existing `/spot/ws` public market-data endpoint remains available during
frontend migration. It can be removed only after production clients have moved
to `/ws/v1`; removal is outside this change.

## Architecture

The OMS API continues to own `/ws/v1` and private user events. It gains an
internal market-data bridge to the existing match gateway, which remains the
source of order books, trades, tickers, and candles.

The bridge connects internally to the match-gateway WebSocket, maintains the
required market subscriptions, and republishes public events through the OMS
WebSocket registry. It does not implement a second order book or candle engine.

This keeps one browser connection while preserving the existing ownership
boundaries:

- match gateway: public market-data production;
- OMS API: browser connection, optional authentication, private event routing;
- `/ws/v1`: unified delivery surface.

## Authentication Flow

1. The browser opens `/ws/v1`, optionally providing the existing session token
   through the currently supported WebSocket token mechanism.
2. The upgrade always succeeds when the network request itself is valid.
3. If the token is valid, the channel stores the resolved authenticated user.
4. If the token is absent, invalid, or expired, the channel is anonymous.
5. Public subscription authorization does not inspect authentication state.
6. Private subscription authorization requires the authenticated user stored on
   the channel.

Authentication failure must not be treated as a connection failure.

## Subscription Model

Public market data is selected by `marketId` and optional candle interval.
Private channels do not accept a target user ID.

Example anonymous subscription:

```json
{
  "op": "subscribe",
  "channels": ["orderbook", "trades", "candles"],
  "marketId": 1,
  "interval": "1m"
}
```

Example authenticated subscription:

```json
{
  "op": "subscribe",
  "channels": ["orderbook", "trades", "candles", "orders", "executions"],
  "marketId": 1,
  "interval": "1m"
}
```

Supported candle intervals remain `1m`, `5m`, `15m`, `1h`, `4h`, and `1d`.

## Initial Snapshots and Live Events

After a successful public subscription, the server sends the relevant initial
snapshot before subsequent live events:

- current order-book snapshot;
- requested candle history;
- latest available market trades.

After a successful authenticated execution subscription, the server sends the
recent execution-history snapshot for the authenticated user, then continues
with live execution events. Order subscriptions continue with live order state
events. Snapshot messages and live messages must have distinct event types so
the frontend can merge them without guessing.

## Error Handling

Subscription errors are returned as structured WebSocket messages and do not
close a healthy connection. Errors include:

- `AUTH_REQUIRED` for a private channel on an anonymous connection;
- `INVALID_CHANNEL` for an unknown channel;
- `INVALID_MARKET` for an unsupported market;
- `INVALID_INTERVAL` for an unsupported candle interval;
- `UPSTREAM_UNAVAILABLE` when a public snapshot cannot currently be obtained.

If the internal market-data bridge reconnects, it restores upstream
subscriptions and resumes public delivery. Private OMS events remain
independent of that reconnect.

## Security Rules

- Public market data is available without authentication.
- Private events are filtered by the server-resolved authenticated user.
- A request field can never select another user's private data.
- Execution-history queries use the authenticated user identity.
- Logs must not contain session tokens.
- Backpressure or a slow client must not block OMS event processing for other
  clients.

## Frontend Migration

The spot frontend will use `/ws/v1` before and after login:

- before login, subscribe to public market channels;
- after login, reconnect or reauthenticate using the session token and add
  private channels;
- on logout, reconnect anonymously and retain public subscriptions.

The frontend no longer needs separate browser connections to `/spot/ws` and
`/ws/v1` after migration.

## Verification

Automated tests must prove:

1. anonymous `/ws/v1` upgrade succeeds;
2. anonymous public subscriptions receive snapshots and live updates;
3. anonymous private subscriptions return `AUTH_REQUIRED` without disconnect;
4. an invalid token still permits public subscriptions;
5. a valid token receives only that user's orders and executions;
6. a forged `userId` cannot expose another user's data;
7. execution history is sent before subsequent live execution events;
8. all supported candle intervals work through `/ws/v1`;
9. bridge reconnect restores public subscriptions;
10. `/spot/ws` remains functional for compatibility.

Production verification must test both anonymous and authenticated clients,
confirm message ownership, and confirm that public REST and WebSocket behavior
still works through Nginx.

## Out of Scope

- Removing `/spot/ws`.
- Changing matching, order-book, trade, or candle calculations.
- Changing session creation or wallet login.
- Adding public access to balances, orders, executions, or account history.

# dYdX Trading Sidecar

`dydxtrading` is the Tyche-facing trading proxy for dYdX native order and
cancel transactions.

Runtime ownership is split as follows:

- `protocol`: dYdX chain node and reusable dYdX protocol libraries.
- `dydxtrading`: standalone Tyche trading proxy.
- `tyche`: business backend for NECO deposits, withdrawals, Ledger, bindings,
  trading intent APIs, and sync workers.

This service is intentionally not part of the `dydxprotocold` startup path.
Restarting or upgrading it must not restart the chain node.

## Wallet Model

Tyche never sends a dYdX private key to this service. On NECO signature login,
Tyche calls `POST /wallet/create`; `dydxtrading` creates or reuses a per-user
dYdX proxy wallet, encrypts the private key into the local keystore, and returns
only the dYdX address to Tyche for binding.

Trading requests must use that bound proxy wallet address as `account`. The
sidecar loads the matching private key from the keystore and signs the native
dYdX transaction. If the request account equals the configured system wallet, or
the message owner does not match the request account, the sidecar rejects it.

## Configuration

Required:

- `DYDX_TRADING_BEARER_TOKEN`: Tyche-to-sidecar API token, at least 32 chars.
- `DYDX_PROXY_WALLET_KEYSTORE_PATH`: encrypted user proxy wallet keystore path.
- `DYDX_PROXY_WALLET_KEYSTORE_SECRET`: keystore encryption secret, at least 32 chars.

Common:

- `DYDX_TRADING_LISTEN`: defaults to `127.0.0.1:8090`.
- `DYDX_CHAIN_ID`: required when gas top-up or subaccount deposit is enabled.
- `DYDX_REST_URL`: defaults to `http://127.0.0.1:1317`.
- `DYDX_GRPC_URL`: defaults to `127.0.0.1:9090`. Used by the internal
  reconcile endpoint to query long-term stateful orders.
- `DYDX_BROADCAST_URL`: defaults to `${DYDX_REST_URL}/cosmos/tx/v1beta1/txs`.
- `DYDX_TX_GAS_LIMIT`: defaults to `500000`; used for user orders,
  subaccount deposits, and gas top-up transactions.
- `DYDX_TX_FEE_AMOUNT`: defaults to `12500000000000000adedx`; Cosmos fee
  coins attached to user orders, subaccount deposits, and gas top-up
  transactions. Increase or replace this if the chain's min gas prices change.
- `DYDX_SYSTEM_PRIVATE_KEY_HEX`: optional system funding wallet private key hex.
- `DYDX_SYSTEM_MNEMONIC`: optional system funding wallet mnemonic. Use this
  instead of `DYDX_SYSTEM_PRIVATE_KEY_HEX` when you only have the mnemonic.
- `DYDX_SYSTEM_HD_PATH`: optional mnemonic derivation path, defaults to
  `m/44'/118'/0'/0/0`.
- `DYDX_SYSTEM_BIP39_PASSPHRASE`: optional BIP39 passphrase, normally empty.
  The system funding wallet is used for gas top-up and for depositing USDC
  collateral into user dYdX subaccounts after Tyche has collected NECO-side
  funds. It is never used for user trading orders. Configure either hex or
  mnemonic, not both.
- `DYDX_GAS_TOP_UP_ENABLED`: set to `true` to fund user proxy wallets with gas.
- `DYDX_GAS_DENOM`: defaults to `adedx`.
- `DYDX_GAS_MIN_BALANCE`: minimum user proxy wallet gas balance in base units.
- `DYDX_GAS_TOP_UP_AMOUNT`: amount sent from the system wallet when below min.
- `DYDX_GAS_CONFIRM_ATTEMPTS`: confirmation polling attempts after top-up.
- `DYDX_GAS_CONFIRM_INTERVAL`: confirmation polling interval, defaults to `500ms`.
- `DYDX_SUBACCOUNT_DEPOSIT_BROADCAST_MODE`: defaults to `BROADCAST_MODE_SYNC`.
- `DYDX_EVENT_API_KEY`: event query token, defaults to bearer token when empty.
- `DYDX_EVENT_INGEST_KEY`: event ingest token, defaults to event API key when empty.
- `TYCHE_URL`: optional Tyche callback/ingest base URL.
- `TYCHE_INTERNAL_API_KEY`: optional Tyche internal API key.

The process rejects non-loopback listen addresses unless
`DYDX_ALLOW_REMOTE_LISTEN=true` is explicitly set. Keep port `8090` private.

## Internal Price Endpoint

`dydxtrading` exposes `GET /prices?market=ETH-USD&marketId=1` for Tyche market
orders. The handler reads the dYdX chain REST price endpoint
`/dydxprotocol/prices/market/{marketId}` through `DYDX_REST_URL`, converts the
chain `price` plus `exponent` into a decimal USD price, and returns it to Tyche.

The endpoint uses the same internal auth as event reads:

```http
X-API-Key: <DYDX_EVENT_API_KEY or DYDX_TRADING_BEARER_TOKEN>
```

This endpoint is read-only and must stay on the private loopback sidecar port.
Tyche uses it to turn a frontend `orderType=market` request into a protected
IOC order before submitting the native dYdX transaction.

## Internal Subaccount Deposit Endpoint

`POST /subaccount/deposit` signs a native dYdX `MsgDepositToSubaccount` with
the configured system wallet and deposits collateral into the user's bound dYdX
subaccount. Tyche calls this after it has already collected/credited the
NECO-side margin asset. A successful response means the deposit transaction was
broadcast; order state still needs indexer/chain confirmation.

```http
Authorization: Bearer <DYDX_TRADING_BEARER_TOKEN>
```

This endpoint must stay private on `127.0.0.1:8090`.

## Internal Reconcile Endpoint

`POST /reconcile/order` is used by Tyche after an order has been broadcast. It
queries the dYdX chain directly and returns:

- tx status from `DYDX_REST_URL`;
- stateful-order status from `DYDX_GRPC_URL`;
- subaccount positions from `DYDX_REST_URL`.

Tyche uses this to move local orders from `broadcasted` to `submitted`, and to
backfill `filled` plus a position snapshot when the stateful order is already
gone but the subaccount position shows the order took effect. This endpoint is
also private and uses:

```http
X-API-Key: <DYDX_EVENT_API_KEY or DYDX_TRADING_BEARER_TOKEN>
```

## Local Build

```sh
go test -mod=readonly -tags "test_ledger_mock netgo" ./...
go build -mod=readonly -tags "netgo" ./cmd/dydxtrading
```

Or:

```sh
make test
make build
```

## Docker

Build from this directory:

```sh
make image
```

Start only this service. The default compose file uses Linux `host` networking
and keeps `dydxtrading` bound to `127.0.0.1:8090`, so it can reach the local
chain REST port without publishing the sidecar API to the public network:

```sh
DYDX_TRADING_BEARER_TOKEN=replace-with-32-plus-chars \
DYDX_PROXY_WALLET_KEYSTORE_PATH=/data/dydx/dydxtrading/proxy-wallets.json \
DYDX_PROXY_WALLET_KEYSTORE_SECRET=replace-with-32-plus-chars \
DYDX_REST_URL=http://127.0.0.1:1317 \
DYDX_GRPC_URL=127.0.0.1:9090 \
DYDX_BROADCAST_URL=http://127.0.0.1:1317/cosmos/tx/v1beta1/txs \
DYDX_TX_GAS_LIMIT=500000 \
DYDX_TX_FEE_AMOUNT=12500000000000000adedx \
make compose-upd
```

Enable gas top-up only after the system wallet has funds. If you have a private
key hex:

```sh
DYDX_SYSTEM_PRIVATE_KEY_HEX=replace-with-system-funding-key \
DYDX_CHAIN_ID=flydex-mainnet-1 \
DYDX_GAS_TOP_UP_ENABLED=true \
DYDX_GAS_DENOM=adedx \
DYDX_GAS_MIN_BALANCE=1000000000000000 \
DYDX_GAS_TOP_UP_AMOUNT=10000000000000000 \
DYDX_GAS_CONFIRM_ATTEMPTS=10 \
DYDX_GAS_CONFIRM_INTERVAL=500ms \
DYDX_TX_GAS_LIMIT=500000 \
DYDX_TX_FEE_AMOUNT=12500000000000000adedx \
DYDX_SUBACCOUNT_DEPOSIT_BROADCAST_MODE=BROADCAST_MODE_SYNC \
make compose-upd
```

If you only have the mnemonic:

```sh
DYDX_SYSTEM_MNEMONIC="replace with system wallet mnemonic" \
DYDX_SYSTEM_HD_PATH="m/44'/118'/0'/0/0" \
DYDX_CHAIN_ID=flydex-mainnet-1 \
DYDX_GAS_TOP_UP_ENABLED=true \
DYDX_GAS_DENOM=adedx \
DYDX_GAS_MIN_BALANCE=1000000000000000 \
DYDX_GAS_TOP_UP_AMOUNT=10000000000000000 \
DYDX_GAS_CONFIRM_ATTEMPTS=10 \
DYDX_GAS_CONFIRM_INTERVAL=500ms \
DYDX_TX_GAS_LIMIT=500000 \
DYDX_TX_FEE_AMOUNT=12500000000000000adedx \
DYDX_SUBACCOUNT_DEPOSIT_BROADCAST_MODE=BROADCAST_MODE_SYNC \
make compose-upd
```

Stop without touching `dydxprotocold` or Tyche:

```sh
make compose-stop
```

The compose file has no host `ports:` mapping. With the default
`DYDX_TRADING_LISTEN=127.0.0.1:8090`, the sidecar API is reachable only from the
same server. Do not set `DYDX_ALLOW_REMOTE_LISTEN=true` unless the host firewall
and reverse proxy rules are already locked down.

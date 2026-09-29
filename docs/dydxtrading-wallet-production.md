# dYdX Trading Wallet Production Notes

Current ownership boundary:

- Tyche handles NECO login, Ledger, order APIs, and wallet binding records.
- dydxtrading creates and stores per-user dYdX proxy wallets.
- The system dYdX wallet is only a gas/funding wallet. It must not sign user orders.

## Required Flow

1. Frontend asks Tyche for a NECO login challenge.
2. User signs the challenge with the NECO wallet.
3. Tyche verifies the NECO signature and creates or loads the Tyche user.
4. Tyche calls dydxtrading `POST /wallet/create`.
5. dydxtrading creates or reuses the user's dYdX proxy wallet and stores the private key encrypted in its keystore.
6. Tyche saves the returned `dydxAddress` as the user's active binding.
7. When the user places a dYdX order, Tyche resolves the active binding from its store and injects `dydxAccount`.
8. Tyche builds the native dYdX Cosmos tx with that account as both tx account and order owner.
9. dydxtrading loads the matching private key from the keystore and signs the tx.
10. If gas top-up is enabled, dydxtrading may fund the proxy wallet from the system wallet before signing.

## dydxtrading Required Config

```sh
DYDX_TRADING_BEARER_TOKEN=replace-with-32-plus-chars
DYDX_PROXY_WALLET_KEYSTORE_PATH=/data/dydx/dydxtrading/proxy-wallets.json
DYDX_PROXY_WALLET_KEYSTORE_SECRET=replace-with-32-plus-chars
DYDX_TRADING_LISTEN=127.0.0.1:8090
DYDX_REST_URL=http://127.0.0.1:1317
DYDX_BROADCAST_URL=http://127.0.0.1:1317/cosmos/tx/v1beta1/txs
```

For the single-server Docker deployment, run `dydxtrading` with the provided
compose file's host network mode. Keep `DYDX_TRADING_LISTEN` on loopback so
port `8090` is not reachable from the public network.

Optional gas top-up:

```sh
DYDX_SYSTEM_MNEMONIC="replace with system gas wallet mnemonic"
DYDX_SYSTEM_HD_PATH="m/44'/118'/0'/0/0"
DYDX_CHAIN_ID=flydex-mainnet-1
DYDX_GAS_TOP_UP_ENABLED=true
DYDX_GAS_DENOM=adedx
DYDX_GAS_MIN_BALANCE=1000000000000000
DYDX_GAS_TOP_UP_AMOUNT=10000000000000000
DYDX_GAS_CONFIRM_ATTEMPTS=10
DYDX_GAS_CONFIRM_INTERVAL=500ms
```

`DYDX_SYSTEM_PRIVATE_KEY_HEX` is still supported when you already have a raw hex
private key. Configure either `DYDX_SYSTEM_PRIVATE_KEY_HEX` or
`DYDX_SYSTEM_MNEMONIC`, not both. The default mnemonic path matches the Cosmos
SDK key path normally used by `dydxprotocold keys add`.

Do not configure one global user trading private key. User trades must be signed by the bound user proxy wallet.

## Tyche Required Config

```toml
dydxTradingURL = "http://127.0.0.1:8090/dydx/trading"
dydxWalletCreateURL = "http://127.0.0.1:8090/wallet/create"
dydxEventSourceURL = "http://127.0.0.1:8090"
internalAPIKey = "same-or-rotated-sidecar-token-at-least-32-chars"
```

Tyche `account` and `privateKey` should stay empty in dYdX mode. Tyche does not sign native dYdX orders.

Do not set these legacy fields in dYdX mode:

```toml
dydxBroadcastURL = ""
dydxChainBroadcastURL = ""
dydxAutoAddressPrefix = ""
```

`internalAPIKey` is mandatory in dYdX mode and must be at least 32 characters.
Use the same value as the sidecar bearer token only when the services are on the
same private host or private network. Rotate it separately when Tyche and
dydxtrading are maintained by different operators.

## Safety Checks Now Enforced

- dydxtrading rejects orders for unknown proxy wallet addresses.
- dydxtrading rejects orders whose account is the system gas wallet.
- dydxtrading rejects orders when the outer request account does not match the native dYdX message owner.
- Tyche production App resolves `dydxAccount` from the active wallet binding; frontend-provided account is not trusted.
- Order records persist `dydxAccount`, so later cancel and sync paths keep the same wallet context.
- Tyche dYdX mode requires `dydxTradingURL`; the old Redis pending queue and `/dydx/broadcast` path are disabled when the sidecar URL is configured.
- Tyche dYdX mode rejects `dydxAutoAddressPrefix`, `dydxBroadcastURL`, and `dydxChainBroadcastURL` so a deployment cannot silently fall back to test address generation or legacy broadcasting.
- Legacy EVM transaction, wallet, and rule endpoints return an error in dYdX mode.
- Order creation is atomic by `userId + clientOrderId`, so a repeated frontend submit resumes or returns the existing order instead of double charging.
- Ledger deposit, refund, and trading settlement writes use the Ledger `refId` as an atomic idempotency gate.
- dYdX sync fills, order-status events, and position events use `eventId` as an atomic idempotency gate.

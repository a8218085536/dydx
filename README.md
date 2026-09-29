# FlyDEX dYdX and Spot Exchange

This repository groups the FlyDEX blockchain, trading services, and spot
exchange source code for development and deployment reference.

## Directory map

### dYdX chain and services

- `protocol/` - dYdX protocol and chain node (`dydxprotocold`).
- `dydxtrading/` - Tyche-facing dYdX trading sidecar.
- `tyche/` - Tyche business service, chain event consumers, and price workers.

### Spot exchange

- `openexch/match-main/` - Aeron matching engine and market gateway.
- `openexch/oms-main/` - order management, risk, ledger, and account APIs.
- `openexch/assets-main/` - asset and balance services.
- `openexch/admin-main/` - cluster administration and operations gateway.
- `openexch/trading-ui-main/` - React and TypeScript trading UI.

### Operations and documentation

- `scripts/` - packaging, deployment, and server checks.
- `docs/` - deployment notes and project documentation.

## Security

Runtime configuration, credentials, mnemonics, private keys, certificates,
archives, binaries, logs, and dependency directories are intentionally
excluded by `.gitignore`. Copy an appropriate example configuration and inject
secrets through the deployment environment before starting a service.

Do not commit production credentials or wallet material to this repository.

# Tyche dYdX Deployment

This package is server-neutral. It does not hardcode a deployment user.

## Layout

- `deploy/tyche.dydx.single-node.toml`: Tyche dYdX + NECO config template.
- `deploy/render-tyche-dydx-config.sh`: renders `tyche.toml` and copies the `dydxtrading` internal token.
- `deploy/build-tyche.sh`: builds `build/tyche`.
- `deploy/start-tyche-dydx.sh`: starts Tyche from `build/tyche`, or falls back to `go run`.
- `deploy/install-tyche-dydx-systemd.sh`: installs the systemd service on the target server.
- `deploy/build-neco-price-cache.sh`: builds the standalone NECO swap price cache worker.
- `deploy/start-neco-price-cache.sh`: starts the price cache worker from `build/neco-price-cache`.
- `deploy/install-neco-price-cache-systemd.sh`: installs `neco-price-cache.service`.
- `deploy/update-tyche-dydx-from-tmp.sh`: updates an existing Tyche deployment from `/tmp/tyche-dydx-deploy-20260815-final.zip`.

## Before Installing

On the target server, make sure these are already running:

- Redis on `127.0.0.1:6379`.
- dYdX chain node.
- `dydxtrading` with `../dydxtrading/.env` next to the `tyche` directory.

The renderer reads `DYDX_TRADING_BEARER_TOKEN` from that file and writes it into
Tyche's internal API key fields.

## Manual Start

```bash
cd /home/ec2-user/flydex/dydx/tyche
bash deploy/render-tyche-dydx-config.sh
bash deploy/build-tyche.sh
bash deploy/start-tyche-dydx.sh
```

## Systemd Install

Run as root or with sudo from `/home/ec2-user/flydex/dydx/tyche`:

```bash
sudo TYCHE_SERVICE_USER=ec2-user bash deploy/install-tyche-dydx-systemd.sh
sudo systemctl restart tyche-dydx.service
sudo systemctl status tyche-dydx.service
```

For temporary frontend debugging through `server-domain:8081`:

```bash
sudo TYCHE_SERVICE_USER=ec2-user TYCHE_LISTEN=0.0.0.0:8081 bash deploy/install-tyche-dydx-systemd.sh
sudo systemctl restart tyche-dydx.service
```

If `sudo` cannot find Go during install, pass the Go binary path explicitly:

```bash
sudo TYCHE_SERVICE_USER=ec2-user TYCHE_GO_BIN=/usr/local/go/bin/go TYCHE_LISTEN=0.0.0.0:8081 bash deploy/install-tyche-dydx-systemd.sh
```

Only expose `8081` to trusted IPs. Do not expose Redis, `dydxtrading` port
`8090`, or dYdX internal node ports publicly.

## Standalone NECO Price Cache

The price cache worker is independent from Tyche's HTTP service. It reads the
same `tyche.toml`, polls the configured NECO swap routes, and writes the latest
asset/USD prices to Redis every second by default.

Install it after `tyche.toml` exists:

```bash
cd /home/ec2-user/flydex/dydx/tyche
sudo TYCHE_SERVICE_USER=ec2-user bash deploy/install-neco-price-cache-systemd.sh
sudo systemctl restart neco-price-cache.service
sudo systemctl status neco-price-cache.service --no-pager -l
```

Run one manual sync without installing systemd:

```bash
cd /home/ec2-user/flydex/dydx/tyche
bash deploy/build-neco-price-cache.sh
./build/neco-price-cache -config ./tyche.toml -once
```

Useful Redis keys:

```text
<namespace>:price:asset:FDEX:USD
<namespace>:price:asset:FDEX:USD:json
<namespace>:price:asset:USDT:USD
<namespace>:price:route:FDEX-USDT
<namespace>:price:assets
<namespace>:price:routes
```

On the current single-node deployment the namespace is usually:

```text
flydex:tyche:flydex-mainnet-1
```

Example checks:

```bash
redis-cli -n 2 HGETALL 'flydex:tyche:flydex-mainnet-1:price:asset:FDEX:USD'
redis-cli -n 2 GET 'flydex:tyche:flydex-mainnet-1:price:asset:FDEX:USD:json'
redis-cli -n 2 SMEMBERS 'flydex:tyche:flydex-mainnet-1:price:assets'
```

The hash and JSON both include `updatedAt` and `expiresAt`. Consumers must reject
the price when current time is greater than `expiresAt`.

## Update From Uploaded Package

Upload the new package to `/tmp/tyche-dydx-deploy-20260815-final.zip`, then run this from the existing Tyche directory:

```bash
cd /home/ec2-user/flydex/dydx/tyche
sudo TYCHE_SERVICE_USER=ec2-user TYCHE_LISTEN=0.0.0.0:8081 bash deploy/update-tyche-dydx-from-tmp.sh
```

The update script stops only `tyche-dydx.service`, keeps the current `tyche.toml`, moves the uploaded zip from `/tmp` into `/home/ec2-user/flydex/dydx`, backs up the old `tyche` directory, extracts the new package, restores the config, reinstalls the systemd unit, and restarts Tyche.

It does not stop or modify the dYdX chain node, Redis, or `dydxtrading`.

The update script also keeps old configs compatible by adding
`dydxRetainedCollateralUsd = "1"` when missing. This is a small
system-retained dYdX collateral buffer per account/subaccount; it is not shown
as user available balance and does not reduce the NECO asset charged for new
orders. Override it with `TYCHE_DYDX_RETAINED_COLLATERAL_USD=0` to disable.

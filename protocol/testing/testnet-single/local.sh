#!/bin/bash
set -exo pipefail

# This initializes a single-validator local dYdX chain for lightweight Tyche
# integration testing. It intentionally does not start Tyche or dydxtrading.

source "./genesis.sh"

CHAIN_ID="localdydxprotocol"

MONIKER="alice"
MNEMONIC="merge panther lobster crazy road hollow amused security before critic about cliff exhibit cause coyote talent happy where lion river tobacco option coconut small"
NODE_KEY="8EGQBxfGMcRfH0C45UTedEG5Xi3XAcukuInLUqFPpskjp1Ny0c5XvwlKevAwtVvkwoeYYQSe0geQG/cF3GAcUA=="

TEST_ACCOUNTS=(
	"dydx199tqg4wdlnu4qjlxchpd7seg454937hjrknju4" # alice
	"dydx10fx7sy6ywd5senxae9dwytf8jxek3t2gcen2vs" # bob
	"dydx1fjg6zp6vv8t9wvy4lps03r5l4g7tkjw9wvmh70" # carl
	"dydx1wau5mja7j7zdavtfq9lu7ejef05hm6ffenlcsn" # dave
)

FAUCET_ACCOUNTS=(
	"dydx1nzuttarf5k2j0nug5yzhr6p74t9avehn9hlh8m" # main faucet
)

VAULT_ACCOUNTS=(
	"dydx1c0m5x87llaunl5sgv3q5vd7j5uha26d2q2r2q0" # BTC vault
	"dydx14rplxdyycc6wxmgl8fggppgq4774l70zt6phkw" # ETH vault
)

VAULT_NUMBERS=(
	0 # BTC clob pair ID
	1 # ETH clob pair ID
)

install_prerequisites() {
	apk add dasel jq
}

create_validator() {
	VAL_HOME_DIR="$HOME/chain/.${MONIKER}"
	VAL_CONFIG_DIR="$VAL_HOME_DIR/config"

	dydxprotocold init "${MONIKER}" -o --chain-id="$CHAIN_ID" --home "$VAL_HOME_DIR"
	dydxprotocold tendermint gen-priv-key --home "$VAL_HOME_DIR" --mnemonic "$MNEMONIC"

	new_file=$(jq ".priv_key.value = \"${NODE_KEY}\"" "$VAL_CONFIG_DIR"/node_key.json)
	cat <<<"$new_file" >"$VAL_CONFIG_DIR"/node_key.json

	edit_config "$VAL_CONFIG_DIR"
	use_slinky "$VAL_CONFIG_DIR"

	edit_genesis "$VAL_CONFIG_DIR" "" "${FAUCET_ACCOUNTS[*]}" "${VAULT_ACCOUNTS[*]}" "${VAULT_NUMBERS[*]}" "" "" "" ""
	update_genesis_use_test_volatile_market "$VAL_CONFIG_DIR"
	update_genesis_complete_bridge_delay "$VAL_CONFIG_DIR" "30"

	echo "$MNEMONIC" | dydxprotocold keys add "${MONIKER}" --recover --keyring-backend=test --home "$VAL_HOME_DIR"

	for acct in "${TEST_ACCOUNTS[@]}"; do
		dydxprotocold add-genesis-account "$acct" 100000000000000000$USDC_DENOM,$TESTNET_VALIDATOR_NATIVE_TOKEN_BALANCE$NATIVE_TOKEN --home "$VAL_HOME_DIR"
	done
	for acct in "${FAUCET_ACCOUNTS[@]}"; do
		dydxprotocold add-genesis-account "$acct" 900000000000000000$USDC_DENOM,$TESTNET_VALIDATOR_NATIVE_TOKEN_BALANCE$NATIVE_TOKEN --home "$VAL_HOME_DIR"
	done

	dydxprotocold gentx "${MONIKER}" $TESTNET_VALIDATOR_SELF_DELEGATE_AMOUNT$NATIVE_TOKEN --moniker="${MONIKER}" --keyring-backend=test --chain-id="$CHAIN_ID" --home "$VAL_HOME_DIR"
	dydxprotocold collect-gentxs --home "$VAL_HOME_DIR"
}

setup_cosmovisor() {
	VAL_HOME_DIR="$HOME/chain/.${MONIKER}"
	export DAEMON_NAME=dydxprotocold
	export DAEMON_HOME="$VAL_HOME_DIR"

	cosmovisor init /bin/dydxprotocold
}

use_slinky() {
	CONFIG_FOLDER=$1
	dasel put -t bool -f "$CONFIG_FOLDER"/app.toml 'oracle.enabled' -v true
	dasel put -t string -f "$CONFIG_FOLDER"/app.toml 'oracle.oracle_address' -v 'slinky0:8080'
}

edit_config() {
	CONFIG_FOLDER=$1

	dasel put -t bool -f "$CONFIG_FOLDER"/config.toml '.p2p.pex' -v 'false'
	dasel put -t string -f "$CONFIG_FOLDER"/config.toml '.consensus.timeout_commit' -v '5s'

	dasel put -t bool -f "$CONFIG_FOLDER"/app.toml '.oracle.metrics_enabled' -v 'true'
	dasel put -t string -f "$CONFIG_FOLDER"/app.toml '.oracle.prometheus_server_address' -v 'localhost:8001'
}

install_prerequisites
create_validator
setup_cosmovisor

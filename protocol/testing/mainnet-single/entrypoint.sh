#!/bin/bash
set -eo pipefail

source "/dydxprotocol/genesis.sh"

CHAIN_ID="${DYDX_MAINNET_SINGLE_CHAIN_ID:-flydex-mainnet-1}"
MONIKER="${DYDX_MAINNET_SINGLE_MONIKER:-flydex-validator-1}"
BRIDGE_DAEMON_ENABLED="${DYDX_BRIDGE_DAEMON_ENABLED:-false}"
WALLET_NAMES=(
	"validator"
	"treasury"
	"market-maker-1"
	"market-maker-2"
	"tyche-custody"
)
VAULT_NUMBERS=(
	0 # BTC clob pair ID
	1 # ETH clob pair ID
)
VAULT_ACCOUNTS=(
	"dydx1c0m5x87llaunl5sgv3q5vd7j5uha26d2q2r2q0" # vault-VAULT_TYPE_CLOB-0
	"dydx14rplxdyycc6wxmgl8fggppgq4774l70zt6phkw" # vault-VAULT_TYPE_CLOB-1
)

VAL_HOME_DIR="$HOME/chain/.validator"
VAL_CONFIG_DIR="$VAL_HOME_DIR/config"
KEY_HOME_DIR="$HOME/keys"
INIT_MARKER="$VAL_HOME_DIR/.mainnet-single-initialized"
VALIDATOR_MNEMONIC=""

DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT="${DYDX_MAINNET_SINGLE_GENESIS_ACCOUNT_USDC_AMOUNT:-80000000000000}"
DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT="${DYDX_MAINNET_SINGLE_GENESIS_ACCOUNT_NATIVE_AMOUNT:-80000000000000000000000000}"
FAUCET_ACCOUNTS=()

ensure_clean_first_init() {
	if [ -d "$KEY_HOME_DIR/keyring-test" ] || [ -d "$VAL_CONFIG_DIR" ]; then
		cat >&2 <<EOF
Refusing to initialize because partial mainnet-single data already exists.

This protects previously generated wallets and chain state from accidental
overwrite. Inspect ./localnet-mainnet-single on the host. Only remove it if
you intentionally want to discard this not-yet-started chain initialization.
EOF
		exit 1
	fi
}

print_wallet_warning() {
	cat <<'EOF'

============================================================
IMPORTANT: SAVE THESE MNEMONICS NOW
These wallets were generated inside this Docker volume during
the first chain initialization. They are printed for operator
backup and will not be regenerated on normal restarts.
============================================================

EOF
}

generate_wallets() {
	mkdir -p "$KEY_HOME_DIR"

	print_wallet_warning

	GENESIS_ACCOUNT_NAMES=()
	GENESIS_ACCOUNTS=()
	for name in "${WALLET_NAMES[@]}"; do
		output=$(dydxprotocold keys add "$name" --keyring-backend=test --home "$KEY_HOME_DIR" 2>&1)
		address=$(printf "%s\n" "$output" | awk '/address:/ {for (i = 1; i <= NF; i++) if ($i == "address:") {print $(i + 1); exit}}')
		mnemonic=$(printf "%s\n" "$output" | tail -n 1)

		if [ -z "$address" ] || [ -z "$mnemonic" ]; then
			echo "Failed to generate wallet $name" >&2
			printf "%s\n" "$output" >&2
			exit 1
		fi

		GENESIS_ACCOUNT_NAMES+=("$name")
		GENESIS_ACCOUNTS+=("$address")
		if [ "$name" = "validator" ]; then
			VALIDATOR_MNEMONIC="$mnemonic"
		fi
		{
			echo "$name"
			echo "address: $address"
			echo "mnemonic: $mnemonic"
			echo
		}
	done
}

genesis_usdc_amount_for_wallet() {
	case "$1" in
		validator)
			echo "${DYDX_MAINNET_SINGLE_VALIDATOR_USDC_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT}"
			;;
		treasury)
			echo "${DYDX_MAINNET_SINGLE_TREASURY_USDC_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT}"
			;;
		market-maker-1)
			echo "${DYDX_MAINNET_SINGLE_MARKET_MAKER_1_USDC_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT}"
			;;
		market-maker-2)
			echo "${DYDX_MAINNET_SINGLE_MARKET_MAKER_2_USDC_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT}"
			;;
		tyche-custody)
			echo "${DYDX_MAINNET_SINGLE_TYCHE_CUSTODY_USDC_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT}"
			;;
		*)
			echo "$DEFAULT_GENESIS_ACCOUNT_USDC_AMOUNT"
			;;
	esac
}

genesis_native_amount_for_wallet() {
	case "$1" in
		validator)
			echo "${DYDX_MAINNET_SINGLE_VALIDATOR_NATIVE_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT}"
			;;
		treasury)
			echo "${DYDX_MAINNET_SINGLE_TREASURY_NATIVE_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT}"
			;;
		market-maker-1)
			echo "${DYDX_MAINNET_SINGLE_MARKET_MAKER_1_NATIVE_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT}"
			;;
		market-maker-2)
			echo "${DYDX_MAINNET_SINGLE_MARKET_MAKER_2_NATIVE_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT}"
			;;
		tyche-custody)
			echo "${DYDX_MAINNET_SINGLE_TYCHE_CUSTODY_NATIVE_AMOUNT:-$DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT}"
			;;
		*)
			echo "$DEFAULT_GENESIS_ACCOUNT_NATIVE_AMOUNT"
			;;
	esac
}

validate_amount() {
	if [[ ! "$2" =~ ^[0-9]+$ ]]; then
		echo "Invalid amount for $1: $2" >&2
		exit 1
	fi
}

total_genesis_native_amount() {
	local total=0
	local name
	local native_amount
	for name in "${GENESIS_ACCOUNT_NAMES[@]}"; do
		native_amount=$(genesis_native_amount_for_wallet "$name")
		validate_amount "$name native token" "$native_amount"
		total=$(echo "$total + $native_amount" | bc)
	done
	echo "$total"
}

create_validator() {
	if [ -z "$VALIDATOR_MNEMONIC" ]; then
		echo "validator mnemonic was not generated" >&2
		exit 1
	fi

	dydxprotocold init "$MONIKER" -o --chain-id="$CHAIN_ID" --home "$VAL_HOME_DIR"
	dydxprotocold tendermint gen-priv-key --home "$VAL_HOME_DIR" --mnemonic "$VALIDATOR_MNEMONIC"

	edit_config "$VAL_CONFIG_DIR"
	use_slinky "$VAL_CONFIG_DIR"

	total_native_amount=$(total_genesis_native_amount)
	edit_genesis "$VAL_CONFIG_DIR" "${GENESIS_ACCOUNTS[*]}" "${FAUCET_ACCOUNTS[*]}" "${VAULT_ACCOUNTS[*]}" "${VAULT_NUMBERS[*]}" "" "" "" "" "$total_native_amount"
	if [ "$BRIDGE_DAEMON_ENABLED" = "true" ]; then
		update_genesis_complete_bridge_delay "$VAL_CONFIG_DIR" "${DYDX_MAINNET_SINGLE_BRIDGE_DELAY_BLOCKS:-600}"
	else
		echo "Bridge daemon disabled; keeping default bridge genesis safety parameters."
	fi

	for idx in "${!GENESIS_ACCOUNTS[@]}"; do
		name="${GENESIS_ACCOUNT_NAMES[$idx]}"
		acct="${GENESIS_ACCOUNTS[$idx]}"
		usdc_amount=$(genesis_usdc_amount_for_wallet "$name")
		native_amount=$(genesis_native_amount_for_wallet "$name")
		validate_amount "$name USDC" "$usdc_amount"
		validate_amount "$name native token" "$native_amount"
		echo "Genesis allocation for $name ($acct): ${usdc_amount}${USDC_DENOM},${native_amount}${NATIVE_TOKEN}"
		dydxprotocold add-genesis-account "$acct" "${usdc_amount}${USDC_DENOM},${native_amount}${NATIVE_TOKEN}" --home "$VAL_HOME_DIR"
	done

	echo "$VALIDATOR_MNEMONIC" | dydxprotocold keys add "$MONIKER" --recover --keyring-backend=test --home "$VAL_HOME_DIR"
	dydxprotocold gentx "$MONIKER" "${TESTNET_VALIDATOR_SELF_DELEGATE_AMOUNT}${NATIVE_TOKEN}" --moniker="$MONIKER" --keyring-backend=test --chain-id="$CHAIN_ID" --home "$VAL_HOME_DIR"
	dydxprotocold collect-gentxs --home "$VAL_HOME_DIR"
}

setup_cosmovisor() {
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
	dasel put -t string -f "$CONFIG_FOLDER"/config.toml '.consensus.timeout_commit' -v "${DYDX_MAINNET_SINGLE_TIMEOUT_COMMIT:-300ms}"

	dasel put -t bool -f "$CONFIG_FOLDER"/app.toml '.oracle.metrics_enabled' -v 'true'
	dasel put -t string -f "$CONFIG_FOLDER"/app.toml '.oracle.prometheus_server_address' -v 'localhost:8001'
}

if [ ! -f "$INIT_MARKER" ]; then
	echo "Initializing single-node mainnet chain $CHAIN_ID..."
	ensure_clean_first_init
	generate_wallets
	create_validator
	setup_cosmovisor
	date -u +"%Y-%m-%dT%H:%M:%SZ" > "$INIT_MARKER"
	echo "Single-node mainnet chain initialized."
else
	echo "Single-node mainnet chain already initialized. Starting existing state."
fi

exec cosmovisor run start \
	--log_level "${DYDX_MAINNET_SINGLE_LOG_LEVEL:-info}" \
	--home "$VAL_HOME_DIR" \
	--bridge-daemon-enabled "$BRIDGE_DAEMON_ENABLED" \
	--bridge-daemon-eth-rpc-endpoint "${ETH_RPC_ENDPOINT:-}" \
	--dd-error-tracking-format true \
	--max-daemon-unhealthy-seconds 4294967295 \
	--grpc-streaming-enabled true \
	--indexer-kafka-conn-str "${DYDX_INDEXER_KAFKA_CONN_STR:-kafka:9092}" \
	--indexer-kafka-max-retry "${DYDX_INDEXER_KAFKA_MAX_RETRY:-20}" \
	--indexer-send-offchain-data "${DYDX_INDEXER_SEND_OFFCHAIN_DATA:-true}" \
	"$@"

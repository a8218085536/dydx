package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChainNodeDeploymentDoesNotStartDYDXTrading(t *testing.T) {
	root := protocolRoot(t)
	files := []string{
		"Dockerfile",
		"docker-compose.yml",
		"docker-compose.single.yml",
		"docker-compose.mainnet-single.yml",
		filepath.Join("testing", "testnet-local", "Dockerfile"),
		filepath.Join("testing", "mainnet-single", "Dockerfile"),
		filepath.Join("testing", "mainnet-single", "entrypoint.sh"),
	}
	for _, file := range files {
		body := readProtocolFile(t, root, file)
		if strings.Contains(body, "cmd/dydxtrading") || strings.Contains(body, "dydxtrading") {
			t.Fatalf("%s references dydxtrading; chain node deployment must stay independent", file)
		}
	}
}

func TestDYDXTradingHasIndependentPrivateDeployment(t *testing.T) {
	for _, file := range []string{
		"Dockerfile.dydxtrading",
		"docker-compose.dydxtrading.yml",
		filepath.Join("cmd", "dydxtrading"),
		filepath.Join("sidecar", "dydxtrading"),
	} {
		if _, err := os.Stat(filepath.Join(protocolRoot(t), file)); err == nil {
			t.Fatalf("%s should live in the top-level dydxtrading service, not protocol", file)
		} else if !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) error = %v", file, err)
		}
	}
}

func TestDEDXNativeTokenConfigIsConsistent(t *testing.T) {
	root := protocolRoot(t)
	constants := readProtocolFile(t, root, filepath.Join("lib", "constants.go"))
	if !strings.Contains(constants, `DefaultBaseDenom  = "adedx"`) {
		t.Fatal("lib/constants.go does not set DefaultBaseDenom to adedx")
	}

	genesis := readProtocolFile(t, root, filepath.Join("testing", "genesis.sh"))
	required := []string{
		`REWARD_TOKEN="adedx"`,
		`NATIVE_TOKEN="adedx"`,
		`NATIVE_TOKEN_WHOLE_COIN="dedx"`,
		`COIN_NAME="DEDX"`,
		`set_denom_metadata "$NATIVE_TOKEN" "$NATIVE_TOKEN_WHOLE_COIN" "$COIN_NAME"`,
	}
	for _, want := range required {
		if !strings.Contains(genesis, want) {
			t.Fatalf("testing/genesis.sh does not contain %q", want)
		}
	}

	config := readProtocolFile(t, root, filepath.Join("cmd", "dydxprotocold", "cmd", "config.go"))
	if !strings.Contains(config, "25000000000adedx") {
		t.Fatal("cmd/dydxprotocold/cmd/config.go does not set min gas price in adedx")
	}

	fixedPrice := readProtocolFile(t, root, filepath.Join(
		"daemons",
		"pricefeed",
		"client",
		"price_function",
		"test_fixed_price_exchange",
		"fixed_price_function.go",
	))
	if !strings.Contains(fixedPrice, `Pair:  "FDEX-USD"`) {
		t.Fatal("fixed price exchange does not include FDEX-USD")
	}
}

func TestDEDXUSDMarketIsConfiguredForTrading(t *testing.T) {
	genesis := readProtocolFile(t, protocolRoot(t), filepath.Join("testing", "genesis.sh"))
	required := []string{
		"# Marketmap: FDEX-USD",
		".app_state.marketmap.market_map.markets.FDEX/USD",
		".app_state.marketmap.market_map.markets.FDEX/USD.ticker.currency_pair.Base' -v 'FDEX'",
		".app_state.marketmap.market_map.markets.FDEX/USD.ticker.currency_pair.Quote' -v 'USD'",
		".app_state.marketmap.market_map.markets.FDEX/USD.provider_configs.[]",
		"# Market: FDEX-USD",
		".app_state.prices.market_params.[37].pair' -v 'FDEX-USD'",
		".app_state.prices.market_params.[37].id' -v '302'",
		".app_state.prices.market_prices.[37].id' -v '302'",
		"# Perpetual (Isolated): FDEX-USD",
		".app_state.perpetuals.perpetuals.[35].params.ticker' -v 'FDEX-USD'",
		".app_state.perpetuals.perpetuals.[35].params.id' -v '302'",
		".app_state.perpetuals.perpetuals.[35].params.market_id' -v '302'",
		"# Clob: FDEX-USD",
		".app_state.clob.clob_pairs.[35].id' -v '302'",
		".app_state.clob.clob_pairs.[35].status' -v \"$INITIAL_CLOB_PAIR_STATUS\"",
		".app_state.clob.clob_pairs.[35].perpetual_clob_metadata.perpetual_id' -v '302'",
	}
	for _, want := range required {
		if !strings.Contains(genesis, want) {
			t.Fatalf("testing/genesis.sh does not contain %q", want)
		}
	}
	if strings.Contains(genesis, ".app_state.clob.clob_pairs.[35].status' -v 'STATUS_FINAL_SETTLEMENT'") {
		t.Fatal("FDEX-USD CLOB pair must be active at genesis, not final settlement")
	}
}

func TestMainnetSingleDoesNotEnableTestUSDMarket(t *testing.T) {
	genesis := readProtocolFile(t, protocolRoot(t), filepath.Join("testing", "genesis.sh"))
	mainnetSingleDefaultGenesis := strings.Split(genesis, "# Modify the genesis file to add test volatile market.")[0]
	forbidden := []string{
		"# Marketmap: TEST-USD",
		".app_state.marketmap.market_map.markets.TEST/USD",
		`"off_chain_ticker": "TEST-USD"`,
	}
	for _, wantAbsent := range forbidden {
		if strings.Contains(mainnetSingleDefaultGenesis, wantAbsent) {
			t.Fatalf("testing/genesis.sh still contains disabled test market fragment %q", wantAbsent)
		}
	}
}

func TestMainnetSingleDefaultBlockCommitIntervalIsFast(t *testing.T) {
	root := protocolRoot(t)
	entrypoint := readProtocolFile(t, root, filepath.Join("testing", "mainnet-single", "entrypoint.sh"))
	if !strings.Contains(entrypoint, `DYDX_MAINNET_SINGLE_TIMEOUT_COMMIT:-300ms`) {
		t.Fatal("testing/mainnet-single/entrypoint.sh should default timeout_commit to 300ms")
	}

	compose := readProtocolFile(t, root, "docker-compose.mainnet-single.yml")
	if !strings.Contains(compose, `DYDX_MAINNET_SINGLE_TIMEOUT_COMMIT=${DYDX_MAINNET_SINGLE_TIMEOUT_COMMIT:-300ms}`) {
		t.Fatal("docker-compose.mainnet-single.yml should default DYDX_MAINNET_SINGLE_TIMEOUT_COMMIT to 300ms")
	}
}

func TestDockerBuildsUseResilientGoModuleNetworkConfig(t *testing.T) {
	root := protocolRoot(t)
	files := []string{
		"Dockerfile",
		filepath.Join("testing", "mainnet-single", "Dockerfile"),
	}
	required := []string{
		`ENV GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct`,
		`ENV GOSUMDB=sum.golang.google.cn`,
	}
	for _, file := range files {
		body := readProtocolFile(t, root, file)
		for _, want := range required {
			if !strings.Contains(body, want) {
				t.Fatalf("%s does not contain %q", file, want)
			}
		}
	}
}

func TestMainnetSingleGenesisUsesActualNativeAllocation(t *testing.T) {
	root := protocolRoot(t)
	genesis := readProtocolFile(t, root, filepath.Join("testing", "genesis.sh"))
	entrypoint := readProtocolFile(t, root, filepath.Join("testing", "mainnet-single", "entrypoint.sh"))

	requiredGenesis := []string{
		`TOTAL_TEST_ACCOUNT_NATIVE_TOKEN_BALANCE="${10}"`,
		`if [ -z "$TOTAL_TEST_ACCOUNT_NATIVE_TOKEN_BALANCE" ]; then`,
		`if [ -n "$TOTAL_TEST_ACCOUNT_NATIVE_TOKEN_BALANCE" ]; then`,
		`bridge_module_account_balance=$(echo "$bridge_module_account_balance - $TOTAL_TEST_ACCOUNT_NATIVE_TOKEN_BALANCE" | bc)`,
		`Invalid native token genesis allocation: bridge module account balance would be negative.`,
	}
	for _, want := range requiredGenesis {
		if !strings.Contains(genesis, want) {
			t.Fatalf("testing/genesis.sh does not contain %q", want)
		}
	}

	requiredEntrypoint := []string{
		`total_genesis_native_amount()`,
		`total_native_amount=$(total_genesis_native_amount)`,
		`edit_genesis "$VAL_CONFIG_DIR" "${GENESIS_ACCOUNTS[*]}" "${FAUCET_ACCOUNTS[*]}" "${VAULT_ACCOUNTS[*]}" "${VAULT_NUMBERS[*]}" "" "" "" "" "$total_native_amount"`,
	}
	for _, want := range requiredEntrypoint {
		if !strings.Contains(entrypoint, want) {
			t.Fatalf("testing/mainnet-single/entrypoint.sh does not contain %q", want)
		}
	}
}

func TestMainnetSinglePreflightScriptExists(t *testing.T) {
	script := readProtocolFile(t, protocolRoot(t), filepath.Join("scripts", "check-mainnet-single-env.sh"))
	required := []string{
		"docker-compose.mainnet-single.yml",
		"docker network inspect flydex_backend",
		"make mainnet-single-start",
		"for port in 26656 26657 9090 9092 1317 8080 8002; do",
		"DYDX_INDEXER_KAFKA_CONN_STR defaults to kafka:9092",
	}
	for _, want := range required {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/check-mainnet-single-env.sh does not contain %q", want)
		}
	}
}

func protocolRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func readProtocolFile(t *testing.T, root, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return string(body)
}

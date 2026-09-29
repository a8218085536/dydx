//go:build cgo

package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"flydex/dydxtrading/sidecar/dydxtrading"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/dydxprotocol/v4-chain/protocol/app"
	appconfig "github.com/dydxprotocol/v4-chain/protocol/app/config"
)

func main() {
	appconfig.SetupConfig()

	listenAddr := envOrDefault("DYDX_TRADING_LISTEN", "127.0.0.1:8090")
	if err := validateListenAddress(listenAddr); err != nil {
		log.Fatal(err)
	}
	authToken := os.Getenv("DYDX_TRADING_BEARER_TOKEN")
	if len(authToken) < 32 {
		log.Fatal("DYDX_TRADING_BEARER_TOKEN must be at least 32 characters")
	}
	eventAPIKey := envOrDefault("DYDX_EVENT_API_KEY", authToken)
	ingestEventKey := envOrDefault("DYDX_EVENT_INGEST_KEY", eventAPIKey)
	eventUpstreamURL := os.Getenv("DYDX_EVENT_UPSTREAM_URL")
	eventUpstreamAPIKey := os.Getenv("DYDX_EVENT_UPSTREAM_API_KEY")
	rawIndexerURL := os.Getenv("DYDX_RAW_INDEXER_EVENT_SOURCE_URL")
	rawIndexerAPIKey := os.Getenv("DYDX_RAW_INDEXER_EVENT_SOURCE_API_KEY")
	indexerKafkaBrokers := csvEnv("DYDX_INDEXER_KAFKA_BROKERS")
	indexerKafkaTopic := envOrDefault("DYDX_INDEXER_KAFKA_TOPIC", dydxtrading.DefaultIndexerKafkaTopic)
	indexerKafkaPartition := int32EnvOrDefault("DYDX_INDEXER_KAFKA_PARTITION", 0)
	indexerKafkaMaxMessages := intEnvOrDefault("DYDX_INDEXER_KAFKA_MAX_MESSAGES", 100)
	indexerKafkaReadTimeout := durationEnvOrDefault("DYDX_INDEXER_KAFKA_READ_TIMEOUT", time.Second)
	tycheURL := os.Getenv("TYCHE_URL")
	tycheInternalAPIKey := os.Getenv("TYCHE_INTERNAL_API_KEY")
	indexerPollInterval := durationEnvOrDefault("DYDX_INDEXER_CONSUMER_INTERVAL", 2*time.Second)
	restURL := strings.TrimRight(envOrDefault("DYDX_REST_URL", "http://127.0.0.1:1317"), "/")
	grpcURL := envOrDefault("DYDX_GRPC_URL", "127.0.0.1:9090")
	broadcastURL := envOrDefault("DYDX_BROADCAST_URL", restURL+"/cosmos/tx/v1beta1/txs")
	txGasLimit := uint64EnvOrDefault("DYDX_TX_GAS_LIMIT", dydxtrading.DefaultTxGasLimit)
	txFeeAmount, err := dydxtrading.ParseTxFeeAmount(os.Getenv("DYDX_TX_FEE_AMOUNT"))
	if err != nil {
		log.Fatalf("invalid DYDX_TX_FEE_AMOUNT: %v", err)
	}
	walletStore, err := dydxtrading.NewEncryptedFileProxyWalletStore(dydxtrading.EncryptedFileProxyWalletStoreConfig{
		Path:          os.Getenv("DYDX_PROXY_WALLET_KEYSTORE_PATH"),
		Secret:        os.Getenv("DYDX_PROXY_WALLET_KEYSTORE_SECRET"),
		AddressPrefix: envOrDefault("DYDX_ADDRESS_PREFIX", appconfig.Bech32PrefixAccAddr),
	})
	if err != nil {
		log.Fatalf("invalid proxy wallet keystore config: %v", err)
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	accountProvider := dydxtrading.NewSequenceManager(
		dydxtrading.NewRESTAccountProvider(restURL, httpClient),
	)
	signer := dydxtrading.NewDirectSigner(app.GetEncodingConfig().TxConfig)
	broadcaster := dydxtrading.NewRESTBroadcaster(broadcastURL, httpClient)
	systemAddress, systemKey, gasTopUpper, err := newGasTopUpper(restURL, httpClient, signer, accountProvider, broadcaster, txGasLimit, txFeeAmount)
	if err != nil {
		log.Fatalf("invalid dYdX gas top-up config: %v", err)
	}
	service := dydxtrading.NewService(dydxtrading.ServiceConfig{
		Signer:             signer,
		AccountProvider:    accountProvider,
		Broadcaster:        broadcaster,
		PrivateKeyProvider: walletStore,
		SystemAddress:      systemAddress,
		GasTopUpper:        gasTopUpper,
		GasLimit:           txGasLimit,
		FeeAmount:          txFeeAmount,
	})
	eventBuffer := dydxtrading.NewMemoryTradingEventBuffer()
	eventProvider := dydxtrading.TradingEventProvider(eventBuffer)
	if eventUpstreamURL != "" {
		eventProvider = dydxtrading.NewRESTTradingEventProvider(dydxtrading.RESTTradingEventProviderConfig{
			URL:    eventUpstreamURL,
			APIKey: eventUpstreamAPIKey,
			Client: httpClient,
		})
	}
	indexerSource, indexerSourceName, err := newIndexerEventSource(rawIndexerURL, rawIndexerAPIKey, indexerKafkaBrokers, indexerKafkaTopic, indexerKafkaPartition, indexerKafkaMaxMessages, indexerKafkaReadTimeout)
	if err != nil {
		log.Fatalf("invalid dYdX indexer source config: %v", err)
	}
	if rawIndexerWorker, enabled, err := dydxtrading.NewRawIndexerConsumerWorker(dydxtrading.RawIndexerConsumerWorkerConfig{
		Source:              indexerSource,
		RawSourceURL:        rawIndexerURL,
		RawSourceAPIKey:     rawIndexerAPIKey,
		TycheURL:            tycheURL,
		TycheInternalAPIKey: tycheInternalAPIKey,
		Client:              httpClient,
		Ingestor:            eventBuffer,
	}); err != nil {
		log.Fatalf("invalid dYdX raw indexer consumer config: %v", err)
	} else if enabled {
		log.Printf("dYdX raw indexer consumer enabled, polling %s", indexerSourceName)
		go dydxtrading.RunIndexerConsumerPollLoop(context.Background(), rawIndexerWorker, indexerPollInterval)
	}
	var subaccountDepositor dydxtrading.SubaccountDepositService
	if systemKey != nil && os.Getenv("DYDX_CHAIN_ID") != "" {
		subaccountDepositor = dydxtrading.NewSubaccountDepositor(dydxtrading.SubaccountDepositorConfig{
			ChainID:          os.Getenv("DYDX_CHAIN_ID"),
			SystemAddress:    systemAddress,
			SystemPrivateKey: systemKey,
			Signer:           signer,
			AccountProvider:  accountProvider,
			Broadcaster:      broadcaster,
			BroadcastMode:    envOrDefault("DYDX_SUBACCOUNT_DEPOSIT_BROADCAST_MODE", dydxtrading.BroadcastModeSync),
			GasLimit:         txGasLimit,
			FeeAmount:        txFeeAmount,
		})
	}

	log.Printf("dYdX trading sidecar listening on %s", listenAddr)
	handler := dydxtrading.NewHTTPHandler(service, dydxtrading.HTTPConfig{
		BearerToken:         authToken,
		EventAPIKey:         eventAPIKey,
		EventProvider:       eventProvider,
		EventIngestor:       eventBuffer,
		IngestEventKey:      ingestEventKey,
		WalletProvisioner:   walletStore,
		MarketPriceProvider: dydxtrading.NewRESTMarketPriceProvider(restURL, httpClient),
		SubaccountDepositor: subaccountDepositor,
		ReconcileProvider:   dydxtrading.NewChainReconcileProvider(restURL, grpcURL, httpClient),
	})
	if err := http.ListenAndServe(listenAddr, handler); err != nil {
		log.Fatal(err)
	}
}

func newIndexerEventSource(rawIndexerURL, rawIndexerAPIKey string, kafkaBrokers []string, kafkaTopic string, kafkaPartition int32, kafkaMaxMessages int, kafkaReadTimeout time.Duration) (dydxtrading.NormalizedIndexerEventSource, string, error) {
	if len(kafkaBrokers) == 0 {
		return nil, rawIndexerURL, nil
	}
	messageSource, err := dydxtrading.NewKafkaIndexerBlockMessageSource(dydxtrading.KafkaIndexerBlockMessageSourceConfig{
		Brokers:     kafkaBrokers,
		Topic:       kafkaTopic,
		Partition:   kafkaPartition,
		MaxMessages: kafkaMaxMessages,
		ReadTimeout: kafkaReadTimeout,
	})
	if err != nil {
		return nil, "", err
	}
	return dydxtrading.NewProtobufIndexerBlockSource(messageSource), "kafka://" + strings.Join(kafkaBrokers, ",") + "/" + kafkaTopic, nil
}

func newGasTopUpper(restURL string, httpClient *http.Client, signer dydxtrading.Signer, accountProvider dydxtrading.AccountProvider, broadcaster dydxtrading.Broadcaster, gasLimit uint64, feeAmount sdk.Coins) (string, cryptotypes.PrivKey, dydxtrading.GasTopUpper, error) {
	systemKey, configured, err := dydxtrading.SystemPrivateKeyFromConfig(dydxtrading.SystemPrivateKeyConfig{
		Hex:             os.Getenv("DYDX_SYSTEM_PRIVATE_KEY_HEX"),
		Mnemonic:        os.Getenv("DYDX_SYSTEM_MNEMONIC"),
		HDPath:          os.Getenv("DYDX_SYSTEM_HD_PATH"),
		BIP39Passphrase: os.Getenv("DYDX_SYSTEM_BIP39_PASSPHRASE"),
	})
	if err != nil {
		return "", nil, nil, err
	}
	if !configured {
		if boolEnv("DYDX_GAS_TOP_UP_ENABLED") {
			return "", nil, nil, dydxtrading.ErrGasTopUpNotConfigured
		}
		return "", nil, nil, nil
	}
	systemAddress, err := dydxtrading.AddressFromPrivateKey(systemKey, envOrDefault("DYDX_ADDRESS_PREFIX", appconfig.Bech32PrefixAccAddr))
	if err != nil {
		return "", nil, nil, err
	}
	if !boolEnv("DYDX_GAS_TOP_UP_ENABLED") {
		return systemAddress, systemKey, nil, nil
	}
	chainID := os.Getenv("DYDX_CHAIN_ID")
	if chainID == "" {
		return "", nil, nil, dydxtrading.ErrGasTopUpNotConfigured
	}
	topUpper, err := dydxtrading.NewBankGasTopUpper(dydxtrading.BankGasTopUpperConfig{
		ChainID:          chainID,
		Denom:            envOrDefault("DYDX_GAS_DENOM", "adedx"),
		MinBalance:       os.Getenv("DYDX_GAS_MIN_BALANCE"),
		TopUpAmount:      os.Getenv("DYDX_GAS_TOP_UP_AMOUNT"),
		SystemAddress:    systemAddress,
		SystemPrivateKey: systemKey,
		Signer:           signer,
		AccountProvider:  accountProvider,
		Broadcaster:      broadcaster,
		BalanceProvider:  dydxtrading.NewRESTBalanceProvider(restURL, httpClient),
		BroadcastMode:    envOrDefault("DYDX_GAS_BROADCAST_MODE", dydxtrading.BroadcastModeSync),
		GasLimit:         gasLimit,
		FeeAmount:        feeAmount,
		ConfirmAttempts:  intEnvOrDefault("DYDX_GAS_CONFIRM_ATTEMPTS", 10),
		ConfirmInterval:  durationEnvOrDefault("DYDX_GAS_CONFIRM_INTERVAL", 500*time.Millisecond),
	})
	if err != nil {
		return "", nil, nil, err
	}
	return systemAddress, systemKey, topUpper, nil
}

func validateListenAddress(listenAddr string) error {
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return err
	}
	if os.Getenv("DYDX_ALLOW_REMOTE_LISTEN") == "true" {
		return nil
	}
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return &listenAddressError{addr: listenAddr}
}

type listenAddressError struct {
	addr string
}

func (e *listenAddressError) Error() string {
	return "remote listen address " + e.addr + " requires DYDX_ALLOW_REMOTE_LISTEN=true"
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func boolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func csvEnv(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func intEnvOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("%s must be an integer: %v", key, err)
	}
	return parsed
}

func int32EnvOrDefault(key string, fallback int32) int32 {
	return int32(intEnvOrDefault(key, int(fallback)))
}

func uint64EnvOrDefault(key string, fallback uint64) uint64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return fallback
	}
	return parsed
}

func durationEnvOrDefault(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf("%s must be a Go duration such as 2s or 500ms: %v", key, err)
	}
	return duration
}

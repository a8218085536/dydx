package main

import (
	"context"
	"encoding/json"
	"errors"
	"ethgo/cmd/tyche/app"
	"ethgo/eth"
	"ethgo/model"
	"ethgo/model/prices"
	"ethgo/tyche"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "./tyche.toml", "Tyche config file")
	assetsFlag := flag.String("assets", "", "comma-separated assets to sync; defaults to configured trading assets and price routes")
	interval := flag.Duration("interval", time.Second, "price sync interval")
	ttl := flag.Duration("ttl", 30*time.Second, "cached price TTL")
	once := flag.Bool("once", false, "sync prices once and exit")
	flag.Parse()

	if err := run(*configPath, *assetsFlag, *interval, *ttl, *once); err != nil {
		log.Fatal(err)
	}
}

func run(configPath, assetsFlag string, interval, ttl time.Duration, once bool) error {
	conf, err := app.NewConfig(configPath)
	if err != nil {
		return err
	}
	if err := model.Init(conf.Redis); err != nil {
		return err
	}
	defer model.Dispose()

	backend, err := newNECOBackend(conf)
	if err != nil {
		return err
	}
	defer backend.Close()

	oracle := newAssetUSDOracle(backend, conf.Tyche)
	if oracle == nil {
		return tyche.ErrNECOPriceOracleNotConfigured
	}
	assets := priceCacheAssetsFromConfig(conf.Tyche, assetsFlag)
	if len(assets) == 0 {
		return errors.New("no price cache assets configured")
	}

	worker := tyche.NewNECOPriceCacheWorker(oracle, prices.NewRedisStore(), assets, interval, ttl)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
	defer stop()

	if once {
		entries, err := worker.PollOnce(ctx)
		if err != nil {
			return err
		}
		encoded, _ := json.MarshalIndent(entries, "", "  ")
		fmt.Println(string(encoded))
		return nil
	}

	log.Printf("NECO price cache syncing assets=%s interval=%s ttl=%s", strings.Join(assets, ","), interval, ttl)
	for {
		entries, err := worker.PollOnce(ctx)
		if err != nil {
			log.Printf("NECO price cache sync failed: %v", err)
		} else {
			log.Printf("NECO price cache synced %d asset(s)", len(entries))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func newNECOBackend(conf *app.Config) (eth.Backend, error) {
	if conf == nil || conf.Tyche == nil || conf.Tyche.NECORPCURL == "" {
		return nil, errors.New("tyche.necoRpcURL is required")
	}
	necoConf := &eth.Config{
		Addr:                  conf.Tyche.NECORPCURL,
		TransactionsPerSecond: 1,
	}
	if conf.Backend != nil {
		cloned := *conf.Backend
		cloned.Addr = conf.Tyche.NECORPCURL
		necoConf = &cloned
	}
	if err := necoConf.Init(); err != nil {
		return nil, err
	}
	return eth.New(necoConf)
}

func newAssetUSDOracle(backend eth.Backend, conf *tyche.Config) tyche.AssetUSDOracle {
	if backend == nil || conf == nil || !conf.NECOPriceOracle.Enabled {
		return nil
	}
	if len(conf.PriceRoutes) > 0 {
		return tyche.NewNECOV2RoutePriceOracle(backend, conf.NECOPriceOracle, conf.PriceRoutes)
	}
	return tyche.NewNECOV2PairPriceOracle(backend, conf.NECOPriceOracle)
}

func priceCacheAssetsFromConfig(conf *tyche.Config, assetsFlag string) []string {
	if assetsFlag != "" {
		return normalizeUniqueAssets(strings.Split(assetsFlag, ","))
	}
	if conf == nil {
		return nil
	}
	var assets []string
	for _, mapping := range conf.AssetMappings {
		if mapping.EnabledForTrading {
			assets = append(assets, mapping.NECOAsset)
		}
	}
	for _, route := range conf.PriceRoutes {
		assets = append(assets, route.Asset)
	}
	return normalizeUniqueAssets(assets)
}

func normalizeUniqueAssets(assets []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(assets))
	for _, asset := range assets {
		normalized := strings.ToUpper(strings.TrimSpace(asset))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

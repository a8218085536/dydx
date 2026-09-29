package tyche

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

func TestNECOV2PairPriceOracleQuotesDEDXUSDThroughConfiguredPairs(t *testing.T) {
	dedx := "0x0000000000000000000000000000000000000ded"
	neco := "0x0000000000000000000000000000000000000ec0"
	usd := "0x0000000000000000000000000000000000000ddd"
	dedxNecoPair := "0x0000000000000000000000000000000000001001"
	necoUSDPair := "0x0000000000000000000000000000000000001002"
	backend := newFakeV2PairBackend(map[string]fakeV2Pair{
		dedxNecoPair: {
			token0:   dedx,
			token1:   neco,
			reserve0: big.NewInt(100),
			reserve1: big.NewInt(200),
		},
		necoUSDPair: {
			token0:   neco,
			token1:   usd,
			reserve0: big.NewInt(200),
			reserve1: big.NewInt(100),
		},
	})
	oracle := NewNECOV2PairPriceOracle(backend, NECOPriceOracleConfig{
		Enabled:         true,
		Mode:            NECOPriceOracleModeV2Pair,
		DEDXNECOPair:    dedxNecoPair,
		NECOUSDPair:     necoUSDPair,
		DEDXToken:       dedx,
		NECOToken:       neco,
		USDToken:        usd,
		DEDXDecimals:    0,
		NECODecimals:    0,
		USDDecimals:     0,
		MinLiquidityUSD: "100",
	})

	snapshot, err := oracle.DEDXUSDPrice(context.Background())

	if err != nil {
		t.Fatalf("DEDXUSDPrice() error = %v", err)
	}
	if snapshot.DEDXUSDPrice != "1" {
		t.Fatalf("DEDXUSDPrice = %q, want 1", snapshot.DEDXUSDPrice)
	}
	if snapshot.Source != "v2_pair:DEDX-NECO+NECO-USD" {
		t.Fatalf("source = %q", snapshot.Source)
	}
}

func TestNECOV2PairPriceOracleRejectsStalePairReserves(t *testing.T) {
	dedx := "0x0000000000000000000000000000000000000ded"
	neco := "0x0000000000000000000000000000000000000ec0"
	usd := "0x0000000000000000000000000000000000000ddd"
	dedxNecoPair := "0x0000000000000000000000000000000000001001"
	necoUSDPair := "0x0000000000000000000000000000000000001002"
	backend := newFakeV2PairBackend(map[string]fakeV2Pair{
		dedxNecoPair: {
			token0:    dedx,
			token1:    neco,
			reserve0:  big.NewInt(100),
			reserve1:  big.NewInt(200),
			timestamp: 10,
		},
		necoUSDPair: {
			token0:    neco,
			token1:    usd,
			reserve0:  big.NewInt(200),
			reserve1:  big.NewInt(100),
			timestamp: 10,
		},
	})
	oracle := NewNECOV2PairPriceOracle(backend, NECOPriceOracleConfig{
		Enabled:            true,
		Mode:               NECOPriceOracleModeV2Pair,
		DEDXNECOPair:       dedxNecoPair,
		NECOUSDPair:        necoUSDPair,
		DEDXToken:          dedx,
		NECOToken:          neco,
		USDToken:           usd,
		MaxPriceAgeSeconds: 60,
	})
	oracle.now = func() time.Time { return time.Unix(1000, 0) }

	_, err := oracle.DEDXUSDPrice(context.Background())

	if !errors.Is(err, ErrNECOPriceStale) {
		t.Fatalf("DEDXUSDPrice() error = %v, want %v", err, ErrNECOPriceStale)
	}
}

func TestNECOV2PairPriceOracleRejectsRuntimePriceDeviation(t *testing.T) {
	dedx := "0x0000000000000000000000000000000000000ded"
	neco := "0x0000000000000000000000000000000000000ec0"
	usd := "0x0000000000000000000000000000000000000ddd"
	dedxNecoPair := "0x0000000000000000000000000000000000001001"
	necoUSDPair := "0x0000000000000000000000000000000000001002"
	backend := newFakeV2PairBackend(map[string]fakeV2Pair{
		dedxNecoPair: {
			token0:   dedx,
			token1:   neco,
			reserve0: big.NewInt(100),
			reserve1: big.NewInt(200),
		},
		necoUSDPair: {
			token0:   neco,
			token1:   usd,
			reserve0: big.NewInt(200),
			reserve1: big.NewInt(100),
		},
	})
	oracle := NewNECOV2PairPriceOracle(backend, NECOPriceOracleConfig{
		Enabled:              true,
		Mode:                 NECOPriceOracleModeV2Pair,
		DEDXNECOPair:         dedxNecoPair,
		NECOUSDPair:          necoUSDPair,
		DEDXToken:            dedx,
		NECOToken:            neco,
		USDToken:             usd,
		MaxPriceDeviationBps: 100,
	})
	if _, err := oracle.DEDXUSDPrice(context.Background()); err != nil {
		t.Fatalf("first DEDXUSDPrice() error = %v", err)
	}
	backend.pairs[common.HexToAddress(dedxNecoPair)] = fakeV2Pair{
		token0:   dedx,
		token1:   neco,
		reserve0: big.NewInt(100),
		reserve1: big.NewInt(300),
	}

	_, err := oracle.DEDXUSDPrice(context.Background())

	if !errors.Is(err, ErrNECOPriceDeviationTooHigh) {
		t.Fatalf("second DEDXUSDPrice() error = %v, want %v", err, ErrNECOPriceDeviationTooHigh)
	}
}

func TestDEDXMarginQuoterUsesOraclePriceToComputeChargeAmount(t *testing.T) {
	quoter := NewDEDXMarginQuoter(staticDEDXUSDOracle{
		snapshot: NECOPriceSnapshot{
			ID:           "snapshot-1",
			DEDXUSDPrice: "0.5",
			Source:       "test",
		},
	}, 0)

	quote, err := quoter.QuoteMargin(context.Background(), TradingMarginQuoteRequest{
		Price: "5",
		Size:  "2",
	})

	if err != nil {
		t.Fatalf("QuoteMargin() error = %v", err)
	}
	if quote.MarginUSD != "10" {
		t.Fatalf("MarginUSD = %q, want 10", quote.MarginUSD)
	}
	if quote.ChargeAsset != "DEDX" || quote.ChargeAmount != "20" {
		t.Fatalf("quote = %+v, want 20 DEDX", quote)
	}
	if quote.PriceSnapshotID != "snapshot-1" {
		t.Fatalf("PriceSnapshotID = %q", quote.PriceSnapshotID)
	}
}

func TestNECOV2RoutePriceOracleQuotesMultipleAssetsThroughConfiguredRoutes(t *testing.T) {
	dedx := "0x0000000000000000000000000000000000000ded"
	neco := "0x0000000000000000000000000000000000000ec0"
	usd := "0x0000000000000000000000000000000000000ddd"
	dedxNecoPair := "0x0000000000000000000000000000000000001001"
	necoUSDPair := "0x0000000000000000000000000000000000001002"
	backend := newFakeV2PairBackend(map[string]fakeV2Pair{
		dedxNecoPair: {
			token0:   dedx,
			token1:   neco,
			reserve0: big.NewInt(100),
			reserve1: big.NewInt(200),
		},
		necoUSDPair: {
			token0:   neco,
			token1:   usd,
			reserve0: big.NewInt(200),
			reserve1: big.NewInt(100),
		},
	})
	oracle := NewNECOV2RoutePriceOracle(backend, NECOPriceOracleConfig{
		Enabled:         true,
		Mode:            NECOPriceOracleModeV2Pair,
		MinLiquidityUSD: "100",
	}, []PriceRouteConfig{
		{
			Name:     "DEDX-NECO-USD",
			Asset:    "DEDX",
			USDAsset: "USD",
			Hops: []PriceRouteHopConfig{
				{Pair: dedxNecoPair, FromToken: dedx, ToToken: neco, FromDecimals: 0, ToDecimals: 0},
				{Pair: necoUSDPair, FromToken: neco, ToToken: usd, FromDecimals: 0, ToDecimals: 0},
			},
		},
		{
			Name:     "NECO-USD",
			Asset:    "NECO",
			USDAsset: "USD",
			Hops: []PriceRouteHopConfig{
				{Pair: necoUSDPair, FromToken: neco, ToToken: usd, FromDecimals: 0, ToDecimals: 0},
			},
		},
	})

	dedxSnapshot, err := oracle.AssetUSDPrice(context.Background(), "DEDX")
	if err != nil {
		t.Fatalf("DEDX AssetUSDPrice() error = %v", err)
	}
	necoSnapshot, err := oracle.AssetUSDPrice(context.Background(), "NECO")
	if err != nil {
		t.Fatalf("NECO AssetUSDPrice() error = %v", err)
	}

	if dedxSnapshot.Asset != "DEDX" || dedxSnapshot.USDPrice != "1" || dedxSnapshot.RouteName != "DEDX-NECO-USD" {
		t.Fatalf("dedx snapshot = %+v, want 1 USD through DEDX route", dedxSnapshot)
	}
	if necoSnapshot.Asset != "NECO" || necoSnapshot.USDPrice != "0.5" || necoSnapshot.RouteName != "NECO-USD" {
		t.Fatalf("neco snapshot = %+v, want 0.5 USD through NECO route", necoSnapshot)
	}
}

func TestNECOV2RoutePriceOracleQuotesStableUSDRouteWithoutSwapHop(t *testing.T) {
	oracle := NewNECOV2RoutePriceOracle(newFakeV2PairBackend(nil), NECOPriceOracleConfig{}, []PriceRouteConfig{{
		Name:     "USDT-USD",
		Asset:    "USDT",
		USDAsset: "USD",
	}})

	snapshot, err := oracle.AssetUSDPrice(context.Background(), "USDT")

	if err != nil {
		t.Fatalf("AssetUSDPrice() error = %v", err)
	}
	if snapshot.Asset != "USDT" || snapshot.USDPrice != "1" || snapshot.RouteName != "USDT-USD" || snapshot.Source != "stable_usd:USDT-USD" {
		t.Fatalf("snapshot = %+v, want direct stable USD price", snapshot)
	}
}

func TestAssetMarginQuoterChargesRequestedTradableAsset(t *testing.T) {
	oracle := staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"NECO": {
				ID:        "snapshot-neco",
				Asset:     "NECO",
				USDPrice:  "0.5",
				RouteName: "NECO-USD",
			},
		},
	}
	quoter := NewAssetMarginQuoter([]AssetMapping{
		{
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
		{
			NECOAsset:         "NECO",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "NECO-USD",
			EnabledForTrading: true,
		},
	}, oracle)

	quote, err := quoter.QuoteMargin(context.Background(), TradingMarginQuoteRequest{
		MarginAsset: "NECO",
		Price:       "5",
		Size:        "2",
	})

	if err != nil {
		t.Fatalf("QuoteMargin() error = %v", err)
	}
	if quote.ChargeAsset != "NECO" || quote.ChargeAmount != "20000000000000000000" {
		t.Fatalf("quote = %+v, want 20 NECO in atomic units", quote)
	}
	if quote.MarginUSD != "10" || quote.AssetUSDPrice != "0.5" || quote.PriceRoute != "NECO-USD" {
		t.Fatalf("quote price fields = %+v", quote)
	}
}

func TestAssetMarginQuoterQuotesUSDTAsDirectUSD(t *testing.T) {
	quoter := NewAssetMarginQuoter([]AssetMapping{{
		NECOAsset:         "USDT",
		DYDXAsset:         "USD",
		Decimals:          18,
		PriceRoute:        "USDT-USD",
		EnabledForTrading: true,
	}}, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"USDT": {
				ID:        "stable-usdt",
				Asset:     "USDT",
				USDPrice:  "1",
				RouteName: "USDT-USD",
			},
		},
	})

	quote, err := quoter.QuoteMargin(context.Background(), TradingMarginQuoteRequest{
		MarginAsset: "USDT",
		Price:       "1892",
		Size:        "0.001",
	})

	if err != nil {
		t.Fatalf("QuoteMargin() error = %v", err)
	}
	if quote.ChargeAsset != "USDT" || quote.ChargeAmount != "1892000000000000000" {
		t.Fatalf("quote = %+v, want 1.892 USDT in atomic units", quote)
	}
	if quote.MarginUSD != "1.892" || quote.AssetUSDPrice != "1" || quote.PriceRoute != "USDT-USD" {
		t.Fatalf("quote price fields = %+v", quote)
	}
}

func TestAssetMarginQuoterQuotesFDEXThroughDirectUSDTPriceRoute(t *testing.T) {
	quoter := NewAssetMarginQuoter([]AssetMapping{{
		NECOAsset:         "FDEX",
		DYDXAsset:         "USD",
		Decimals:          18,
		PriceRoute:        "FDEX-USDT",
		EnabledForTrading: true,
	}}, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"FDEX": {
				ID:        "fdex-usdt-snapshot",
				Asset:     "FDEX",
				USDPrice:  "4",
				RouteName: "FDEX-USDT",
			},
		},
	})

	quote, err := quoter.QuoteMargin(context.Background(), TradingMarginQuoteRequest{
		MarginAsset: "fdex",
		Price:       "20",
		Size:        "2",
	})

	if err != nil {
		t.Fatalf("QuoteMargin() error = %v", err)
	}
	if quote.ChargeAsset != "FDEX" || quote.ChargeAmount != "10000000000000000000" {
		t.Fatalf("quote = %+v, want 10 FDEX in atomic units", quote)
	}
	if quote.MarginUSD != "40" || quote.AssetUSDPrice != "4" || quote.PriceRoute != "FDEX-USDT" || quote.DEDXUSDPrice != "" {
		t.Fatalf("quote price fields = %+v", quote)
	}
}

func TestAssetMarginQuoterRaisesSmallOrderToMinimumMarginUSD(t *testing.T) {
	quoter := NewAssetMarginQuoter([]AssetMapping{
		{
			NECOAsset:         "DEDX",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "DEDX-NECO-USD",
			EnabledForTrading: true,
		},
	}, staticAssetUSDOracle{
		snapshots: map[string]AssetPriceSnapshot{
			"DEDX": {
				ID:        "snapshot-dedx",
				Asset:     "DEDX",
				USDPrice:  "4",
				RouteName: "DEDX-NECO-USD",
			},
		},
	})

	quote, err := quoter.QuoteMargin(context.Background(), TradingMarginQuoteRequest{
		MarginAsset:  "DEDX",
		Price:        "1892",
		Size:         "0.001",
		MinMarginUSD: "20",
	})

	if err != nil {
		t.Fatalf("QuoteMargin() error = %v", err)
	}
	if quote.MarginUSD != "20" {
		t.Fatalf("MarginUSD = %q, want 20", quote.MarginUSD)
	}
	if quote.ChargeAsset != "DEDX" || quote.ChargeAmount != "5000000000000000000" {
		t.Fatalf("quote = %+v, want 5 DEDX in atomic units", quote)
	}
}

func TestAssetMarginQuoterRejectsAssetThatIsNotEnabledForTrading(t *testing.T) {
	quoter := NewAssetMarginQuoter([]AssetMapping{
		{
			NECOAsset:         "NECO",
			DYDXAsset:         "USD",
			Decimals:          18,
			PriceRoute:        "NECO-USD",
			EnabledForTrading: false,
		},
	}, staticAssetUSDOracle{})

	_, err := quoter.QuoteMargin(context.Background(), TradingMarginQuoteRequest{
		MarginAsset: "NECO",
		Price:       "5",
		Size:        "2",
	})

	if !errors.Is(err, ErrTradingBalanceAssetNotConfigured) {
		t.Fatalf("QuoteMargin() error = %v, want %v", err, ErrTradingBalanceAssetNotConfigured)
	}
}

type staticDEDXUSDOracle struct {
	snapshot NECOPriceSnapshot
	err      error
}

func (o staticDEDXUSDOracle) DEDXUSDPrice(context.Context) (NECOPriceSnapshot, error) {
	return o.snapshot, o.err
}

type staticAssetUSDOracle struct {
	snapshots map[string]AssetPriceSnapshot
	err       error
}

func (o staticAssetUSDOracle) AssetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if o.err != nil {
		return AssetPriceSnapshot{}, o.err
	}
	snapshot, ok := o.snapshots[asset]
	if !ok {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	return snapshot, nil
}

type fakeV2Pair struct {
	token0    string
	token1    string
	reserve0  *big.Int
	reserve1  *big.Int
	timestamp uint32
}

type fakeV2PairBackend struct {
	pairs map[common.Address]fakeV2Pair
	abi   abi.ABI
}

func newFakeV2PairBackend(pairs map[string]fakeV2Pair) *fakeV2PairBackend {
	normalized := map[common.Address]fakeV2Pair{}
	for address, pair := range pairs {
		normalized[common.HexToAddress(address)] = pair
	}
	parsed, err := abi.JSON(strings.NewReader(necoV2PairABIJSON))
	if err != nil {
		panic(err)
	}
	return &fakeV2PairBackend{pairs: normalized, abi: parsed}
}

func (b *fakeV2PairBackend) BlockNumber(context.Context) (uint64, error) {
	return 123, nil
}

func (b *fakeV2PairBackend) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if msg.To == nil {
		return nil, errors.New("missing call target")
	}
	pair, ok := b.pairs[*msg.To]
	if !ok {
		return nil, errors.New("unknown pair")
	}
	for name, method := range b.abi.Methods {
		if len(msg.Data) >= 4 && string(msg.Data[:4]) == string(method.ID) {
			switch name {
			case "token0":
				return method.Outputs.Pack(common.HexToAddress(pair.token0))
			case "token1":
				return method.Outputs.Pack(common.HexToAddress(pair.token1))
			case "getReserves":
				timestamp := pair.timestamp
				if timestamp == 0 {
					timestamp = uint32(time.Now().Unix())
				}
				return method.Outputs.Pack(pair.reserve0, pair.reserve1, timestamp)
			}
		}
	}
	return nil, errors.New("unknown method")
}

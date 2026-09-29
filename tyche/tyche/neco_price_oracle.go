package tyche

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const NECOPriceOracleModeV2Pair = "v2_pair"

const necoV2PairABIJSON = `[
	{"inputs":[],"name":"token0","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
	{"inputs":[],"name":"token1","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
	{"inputs":[],"name":"getReserves","outputs":[{"internalType":"uint112","name":"reserve0","type":"uint112"},{"internalType":"uint112","name":"reserve1","type":"uint112"},{"internalType":"uint32","name":"blockTimestampLast","type":"uint32"}],"stateMutability":"view","type":"function"}
]`

var (
	ErrNECOPriceOracleNotConfigured = errors.New("neco price oracle is not configured")
	ErrNECOPriceInvalidPair         = errors.New("neco price oracle invalid pair")
	ErrNECOPriceInvalidReserve      = errors.New("neco price oracle invalid reserve")
	ErrNECOPriceLiquidityTooLow     = errors.New("neco price oracle liquidity too low")
	ErrNECOPriceStale               = errors.New("neco price oracle stale price")
	ErrNECOPriceDeviationTooHigh    = errors.New("neco price oracle price deviation too high")
	ErrTradingQuoteInvalidAmount    = errors.New("trading quote invalid amount")
)

type NECOPriceOracleConfig struct {
	Enabled              bool   `json:"enabled" toml:"enabled"`
	Mode                 string `json:"mode" toml:"mode"`
	DEDXNECOPair         string `json:"dedxNecoPair" toml:"dedxNecoPair"`
	NECOUSDPair          string `json:"necoUsdPair" toml:"necoUsdPair"`
	DEDXToken            string `json:"dedxToken" toml:"dedxToken"`
	NECOToken            string `json:"necoToken" toml:"necoToken"`
	USDToken             string `json:"usdToken" toml:"usdToken"`
	DEDXDecimals         uint8  `json:"dedxDecimals" toml:"dedxDecimals"`
	NECODecimals         uint8  `json:"necoDecimals" toml:"necoDecimals"`
	USDDecimals          uint8  `json:"usdDecimals" toml:"usdDecimals"`
	MinLiquidityUSD      string `json:"minLiquidityUsd" toml:"minLiquidityUsd"`
	MaxPriceDeviationBps uint32 `json:"maxPriceDeviationBps" toml:"maxPriceDeviationBps"`
	MaxPriceAgeSeconds   uint32 `json:"maxPriceAgeSeconds" toml:"maxPriceAgeSeconds"`
}

type PriceRouteConfig struct {
	Name     string                `json:"name" toml:"name"`
	Asset    string                `json:"asset" toml:"asset"`
	USDAsset string                `json:"usdAsset" toml:"usdAsset"`
	Hops     []PriceRouteHopConfig `json:"hops" toml:"hops"`
}

type PriceRouteHopConfig struct {
	Pair         string `json:"pair" toml:"pair"`
	FromToken    string `json:"fromToken" toml:"fromToken"`
	ToToken      string `json:"toToken" toml:"toToken"`
	FromDecimals uint8  `json:"fromDecimals" toml:"fromDecimals"`
	ToDecimals   uint8  `json:"toDecimals" toml:"toDecimals"`
}

type NECOPriceSnapshot struct {
	ID           string `json:"id"`
	DEDXUSDPrice string `json:"dedxUsdPrice"`
	Source       string `json:"source"`
	BlockNumber  uint64 `json:"blockNumber"`
	MinLiquidity string `json:"minLiquidityUsd,omitempty"`
	DEDXNECOPair string `json:"dedxNecoPair"`
	NECOUSDPair  string `json:"necoUsdPair"`
}

type AssetPriceSnapshot struct {
	ID           string   `json:"id"`
	Asset        string   `json:"asset"`
	USDAsset     string   `json:"usdAsset"`
	USDPrice     string   `json:"usdPrice"`
	RouteName    string   `json:"routeName"`
	Source       string   `json:"source"`
	BlockNumber  uint64   `json:"blockNumber"`
	MinLiquidity string   `json:"minLiquidityUsd,omitempty"`
	Pairs        []string `json:"pairs"`
}

type NECOV2PairCaller interface {
	BlockNumber(context.Context) (uint64, error)
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
}

type DEDXUSDOracle interface {
	DEDXUSDPrice(context.Context) (NECOPriceSnapshot, error)
}

type AssetUSDOracle interface {
	AssetUSDPrice(context.Context, string) (AssetPriceSnapshot, error)
}

type NECOV2PairPriceOracle struct {
	caller NECOV2PairCaller
	conf   NECOPriceOracleConfig
	abi    abi.ABI
	now    func() time.Time

	mu               sync.Mutex
	lastDEDXUSDPrice *big.Rat
}

type NECOV2RoutePriceOracle struct {
	caller NECOV2PairCaller
	conf   NECOPriceOracleConfig
	routes map[string]PriceRouteConfig
	abi    abi.ABI
	now    func() time.Time

	mu         sync.Mutex
	lastPrices map[string]*big.Rat
}

func NewNECOV2PairPriceOracle(caller NECOV2PairCaller, conf NECOPriceOracleConfig) *NECOV2PairPriceOracle {
	parsed, err := abi.JSON(strings.NewReader(necoV2PairABIJSON))
	if err != nil {
		panic(err)
	}
	if conf.Mode == "" {
		conf.Mode = NECOPriceOracleModeV2Pair
	}
	return &NECOV2PairPriceOracle{caller: caller, conf: conf, abi: parsed, now: time.Now}
}

func NewNECOV2RoutePriceOracle(caller NECOV2PairCaller, conf NECOPriceOracleConfig, routes []PriceRouteConfig) *NECOV2RoutePriceOracle {
	parsed, err := abi.JSON(strings.NewReader(necoV2PairABIJSON))
	if err != nil {
		panic(err)
	}
	if conf.Mode == "" {
		conf.Mode = NECOPriceOracleModeV2Pair
	}
	oracle := &NECOV2RoutePriceOracle{
		caller:     caller,
		conf:       conf,
		routes:     map[string]PriceRouteConfig{},
		abi:        parsed,
		now:        time.Now,
		lastPrices: map[string]*big.Rat{},
	}
	for _, route := range routes {
		if route.Asset != "" {
			oracle.routes[normalizeAsset(route.Asset)] = route
		}
		if route.Name != "" {
			oracle.routes[normalizeAsset(route.Name)] = route
		}
	}
	return oracle
}

func (o *NECOV2RoutePriceOracle) AssetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if o == nil {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	route, ok := o.routes[normalizeAsset(asset)]
	if !ok {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	if len(route.Hops) == 0 {
		return stableUSDRoutePrice(route)
	}
	if o.caller == nil {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}

	hopPrices := make([]*big.Rat, 0, len(route.Hops))
	fromAmounts := make([]*big.Rat, 0, len(route.Hops))
	toAmounts := make([]*big.Rat, 0, len(route.Hops))
	timestamps := make([]uint32, 0, len(route.Hops))
	pairs := make([]string, 0, len(route.Hops))
	for _, hop := range route.Hops {
		pair, err := o.readPair(ctx, hop.Pair)
		if err != nil {
			return AssetPriceSnapshot{}, err
		}
		fromReserve, toReserve, err := pair.reservesFor(hop.FromToken, hop.ToToken)
		if err != nil {
			return AssetPriceSnapshot{}, err
		}
		fromAmount := scaleAtomicAmount(fromReserve, hop.FromDecimals)
		toAmount := scaleAtomicAmount(toReserve, hop.ToDecimals)
		if !positiveRat(fromAmount) || !positiveRat(toAmount) {
			return AssetPriceSnapshot{}, ErrNECOPriceInvalidReserve
		}
		hopPrices = append(hopPrices, new(big.Rat).Quo(toAmount, fromAmount))
		fromAmounts = append(fromAmounts, fromAmount)
		toAmounts = append(toAmounts, toAmount)
		timestamps = append(timestamps, pair.timestamp)
		pairs = append(pairs, hop.Pair)
	}

	price := big.NewRat(1, 1)
	for _, hopPrice := range hopPrices {
		price.Mul(price, hopPrice)
	}
	if !positiveRat(price) {
		return AssetPriceSnapshot{}, ErrTradingQuoteInvalidAmount
	}
	if err := o.validateFreshness(timestamps...); err != nil {
		return AssetPriceSnapshot{}, err
	}
	minLiquidity, err := routeMinLiquidityUSD(fromAmounts, toAmounts, hopPrices)
	if err != nil {
		return AssetPriceSnapshot{}, err
	}
	if err := o.validateLiquidity(minLiquidity); err != nil {
		return AssetPriceSnapshot{}, err
	}
	if err := o.validateDeviation(normalizeAsset(route.Asset), price); err != nil {
		return AssetPriceSnapshot{}, err
	}

	blockNumber, err := o.caller.BlockNumber(ctx)
	if err != nil {
		return AssetPriceSnapshot{}, err
	}
	name := defaultString(route.Name, normalizeAsset(route.Asset)+"-USD")
	return AssetPriceSnapshot{
		ID:           fmt.Sprintf("neco-v2-route:%d:%s", blockNumber, strings.Join(normalizePairAddresses(pairs), ":")),
		Asset:        normalizeAsset(route.Asset),
		USDAsset:     defaultString(route.USDAsset, "USD"),
		USDPrice:     ratToDecimalString(price, 18),
		RouteName:    name,
		Source:       "v2_pair:" + name,
		BlockNumber:  blockNumber,
		MinLiquidity: ratToDecimalString(minLiquidity, 18),
		Pairs:        pairs,
	}, nil
}

func stableUSDRoutePrice(route PriceRouteConfig) (AssetPriceSnapshot, error) {
	asset := normalizeAsset(route.Asset)
	usdAsset := normalizeAsset(defaultString(route.USDAsset, "USD"))
	if asset == "" || usdAsset != "USD" {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	name := defaultString(route.Name, asset+"-USD")
	return AssetPriceSnapshot{
		ID:        "neco-stable-route:" + name,
		Asset:     asset,
		USDAsset:  usdAsset,
		USDPrice:  "1",
		RouteName: name,
		Source:    "stable_usd:" + name,
	}, nil
}

func (o *NECOV2PairPriceOracle) AssetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if normalizeAsset(asset) != "DEDX" {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	snapshot, err := o.DEDXUSDPrice(ctx)
	if err != nil {
		return AssetPriceSnapshot{}, err
	}
	return AssetPriceSnapshot{
		ID:           snapshot.ID,
		Asset:        "DEDX",
		USDAsset:     "USD",
		USDPrice:     snapshot.DEDXUSDPrice,
		RouteName:    "DEDX-NECO-USD",
		Source:       snapshot.Source,
		BlockNumber:  snapshot.BlockNumber,
		MinLiquidity: snapshot.MinLiquidity,
		Pairs:        []string{snapshot.DEDXNECOPair, snapshot.NECOUSDPair},
	}, nil
}

func (o *NECOV2PairPriceOracle) DEDXUSDPrice(ctx context.Context) (NECOPriceSnapshot, error) {
	if o == nil || o.caller == nil {
		return NECOPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	dedxNeco, err := o.readPair(ctx, o.conf.DEDXNECOPair)
	if err != nil {
		return NECOPriceSnapshot{}, err
	}
	necoUSD, err := o.readPair(ctx, o.conf.NECOUSDPair)
	if err != nil {
		return NECOPriceSnapshot{}, err
	}
	dedxReserve, necoReserve, err := dedxNeco.reservesFor(o.conf.DEDXToken, o.conf.NECOToken)
	if err != nil {
		return NECOPriceSnapshot{}, err
	}
	necoReserveForUSD, usdReserve, err := necoUSD.reservesFor(o.conf.NECOToken, o.conf.USDToken)
	if err != nil {
		return NECOPriceSnapshot{}, err
	}

	dedxAmount := scaleAtomicAmount(dedxReserve, o.conf.DEDXDecimals)
	necoAmount := scaleAtomicAmount(necoReserve, o.conf.NECODecimals)
	necoAmountForUSD := scaleAtomicAmount(necoReserveForUSD, o.conf.NECODecimals)
	usdAmount := scaleAtomicAmount(usdReserve, o.conf.USDDecimals)
	if !positiveRat(dedxAmount) || !positiveRat(necoAmount) || !positiveRat(necoAmountForUSD) || !positiveRat(usdAmount) {
		return NECOPriceSnapshot{}, ErrNECOPriceInvalidReserve
	}

	dedxNECOPrice := new(big.Rat).Quo(necoAmount, dedxAmount)
	necoUSDPrice := new(big.Rat).Quo(usdAmount, necoAmountForUSD)
	dedxUSDPrice := new(big.Rat).Mul(dedxNECOPrice, necoUSDPrice)

	dedxNecoLiquidityUSD := new(big.Rat).Mul(necoAmount, necoUSDPrice)
	dedxNecoLiquidityUSD.Mul(dedxNecoLiquidityUSD, big.NewRat(2, 1))
	necoUSDLiquidityUSD := new(big.Rat).Mul(usdAmount, big.NewRat(2, 1))
	minLiquidity := minRat(dedxNecoLiquidityUSD, necoUSDLiquidityUSD)
	if err := o.validateFreshness(dedxNeco.timestamp, necoUSD.timestamp); err != nil {
		return NECOPriceSnapshot{}, err
	}
	if err := o.validateLiquidity(minLiquidity); err != nil {
		return NECOPriceSnapshot{}, err
	}
	if err := o.validateDeviation(dedxUSDPrice); err != nil {
		return NECOPriceSnapshot{}, err
	}

	blockNumber, err := o.caller.BlockNumber(ctx)
	if err != nil {
		return NECOPriceSnapshot{}, err
	}
	return NECOPriceSnapshot{
		ID:           fmt.Sprintf("neco-v2:%d:%s:%s", blockNumber, normalizeAddress(o.conf.DEDXNECOPair), normalizeAddress(o.conf.NECOUSDPair)),
		DEDXUSDPrice: ratToDecimalString(dedxUSDPrice, 18),
		Source:       "v2_pair:DEDX-NECO+NECO-USD",
		BlockNumber:  blockNumber,
		MinLiquidity: ratToDecimalString(minLiquidity, 18),
		DEDXNECOPair: o.conf.DEDXNECOPair,
		NECOUSDPair:  o.conf.NECOUSDPair,
	}, nil
}

func (o *NECOV2RoutePriceOracle) validateLiquidity(liquidity *big.Rat) error {
	if o.conf.MinLiquidityUSD == "" {
		return nil
	}
	minLiquidity, ok := parseDecimalRat(o.conf.MinLiquidityUSD)
	if !ok || minLiquidity.Sign() < 0 {
		return ErrTradingQuoteInvalidAmount
	}
	if liquidity.Cmp(minLiquidity) < 0 {
		return ErrNECOPriceLiquidityTooLow
	}
	return nil
}

func (o *NECOV2RoutePriceOracle) validateFreshness(timestamps ...uint32) error {
	if o.conf.MaxPriceAgeSeconds == 0 {
		return nil
	}
	now := time.Now
	if o.now != nil {
		now = o.now
	}
	current := now().Unix()
	maxAge := int64(o.conf.MaxPriceAgeSeconds)
	for _, timestamp := range timestamps {
		if timestamp == 0 {
			return ErrNECOPriceStale
		}
		observed := int64(timestamp)
		if observed > current+60 || current-observed > maxAge {
			return ErrNECOPriceStale
		}
	}
	return nil
}

func (o *NECOV2RoutePriceOracle) validateDeviation(asset string, price *big.Rat) error {
	if !positiveRat(price) {
		return ErrTradingQuoteInvalidAmount
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if previous := o.lastPrices[asset]; previous != nil && o.conf.MaxPriceDeviationBps > 0 {
		diff := absRat(new(big.Rat).Sub(price, previous))
		deviationBps := diff.Quo(diff, previous)
		deviationBps.Mul(deviationBps, big.NewRat(10000, 1))
		if deviationBps.Cmp(big.NewRat(int64(o.conf.MaxPriceDeviationBps), 1)) > 0 {
			return ErrNECOPriceDeviationTooHigh
		}
	}
	o.lastPrices[asset] = new(big.Rat).Set(price)
	return nil
}

func (o *NECOV2RoutePriceOracle) readPair(ctx context.Context, pairAddress string) (necoV2PairState, error) {
	if !common.IsHexAddress(pairAddress) {
		return necoV2PairState{}, ErrNECOPriceInvalidPair
	}
	token0, err := o.callAddress(ctx, pairAddress, "token0")
	if err != nil {
		return necoV2PairState{}, err
	}
	token1, err := o.callAddress(ctx, pairAddress, "token1")
	if err != nil {
		return necoV2PairState{}, err
	}
	reserves, err := o.call(ctx, pairAddress, "getReserves")
	if err != nil {
		return necoV2PairState{}, err
	}
	if len(reserves) < 2 {
		return necoV2PairState{}, ErrNECOPriceInvalidReserve
	}
	reserve0, ok := reserves[0].(*big.Int)
	if !ok {
		return necoV2PairState{}, ErrNECOPriceInvalidReserve
	}
	reserve1, ok := reserves[1].(*big.Int)
	if !ok {
		return necoV2PairState{}, ErrNECOPriceInvalidReserve
	}
	return necoV2PairState{
		token0:    normalizeAddress(token0.Hex()),
		token1:    normalizeAddress(token1.Hex()),
		reserve0:  reserve0,
		reserve1:  reserve1,
		timestamp: reserveTimestamp(reserves),
	}, nil
}

func (o *NECOV2RoutePriceOracle) callAddress(ctx context.Context, pairAddress, method string) (common.Address, error) {
	output, err := o.call(ctx, pairAddress, method)
	if err != nil {
		return common.Address{}, err
	}
	if len(output) != 1 {
		return common.Address{}, ErrNECOPriceInvalidPair
	}
	address, ok := output[0].(common.Address)
	if !ok {
		return common.Address{}, ErrNECOPriceInvalidPair
	}
	return address, nil
}

func (o *NECOV2RoutePriceOracle) call(ctx context.Context, pairAddress, method string) ([]interface{}, error) {
	input, err := o.abi.Pack(method)
	if err != nil {
		return nil, err
	}
	to := common.HexToAddress(pairAddress)
	output, err := o.caller.CallContract(ctx, ethereum.CallMsg{
		To:   &to,
		Data: input,
	}, nil)
	if err != nil {
		return nil, err
	}
	return o.abi.Unpack(method, output)
}

func (o *NECOV2PairPriceOracle) validateLiquidity(liquidity *big.Rat) error {
	if o.conf.MinLiquidityUSD == "" {
		return nil
	}
	minLiquidity, ok := parseDecimalRat(o.conf.MinLiquidityUSD)
	if !ok || minLiquidity.Sign() < 0 {
		return ErrTradingQuoteInvalidAmount
	}
	if liquidity.Cmp(minLiquidity) < 0 {
		return ErrNECOPriceLiquidityTooLow
	}
	return nil
}

func (o *NECOV2PairPriceOracle) validateFreshness(timestamps ...uint32) error {
	if o.conf.MaxPriceAgeSeconds == 0 {
		return nil
	}
	now := time.Now
	if o.now != nil {
		now = o.now
	}
	current := now().Unix()
	maxAge := int64(o.conf.MaxPriceAgeSeconds)
	for _, timestamp := range timestamps {
		if timestamp == 0 {
			return ErrNECOPriceStale
		}
		observed := int64(timestamp)
		if observed > current+60 || current-observed > maxAge {
			return ErrNECOPriceStale
		}
	}
	return nil
}

func (o *NECOV2PairPriceOracle) validateDeviation(price *big.Rat) error {
	if !positiveRat(price) {
		return ErrTradingQuoteInvalidAmount
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastDEDXUSDPrice != nil && o.conf.MaxPriceDeviationBps > 0 {
		diff := absRat(new(big.Rat).Sub(price, o.lastDEDXUSDPrice))
		deviationBps := diff.Quo(diff, o.lastDEDXUSDPrice)
		deviationBps.Mul(deviationBps, big.NewRat(10000, 1))
		if deviationBps.Cmp(big.NewRat(int64(o.conf.MaxPriceDeviationBps), 1)) > 0 {
			return ErrNECOPriceDeviationTooHigh
		}
	}
	o.lastDEDXUSDPrice = new(big.Rat).Set(price)
	return nil
}

func (o *NECOV2PairPriceOracle) readPair(ctx context.Context, pairAddress string) (necoV2PairState, error) {
	if !common.IsHexAddress(pairAddress) {
		return necoV2PairState{}, ErrNECOPriceInvalidPair
	}
	token0, err := o.callAddress(ctx, pairAddress, "token0")
	if err != nil {
		return necoV2PairState{}, err
	}
	token1, err := o.callAddress(ctx, pairAddress, "token1")
	if err != nil {
		return necoV2PairState{}, err
	}
	reserves, err := o.call(ctx, pairAddress, "getReserves")
	if err != nil {
		return necoV2PairState{}, err
	}
	if len(reserves) < 2 {
		return necoV2PairState{}, ErrNECOPriceInvalidReserve
	}
	reserve0, ok := reserves[0].(*big.Int)
	if !ok {
		return necoV2PairState{}, ErrNECOPriceInvalidReserve
	}
	reserve1, ok := reserves[1].(*big.Int)
	if !ok {
		return necoV2PairState{}, ErrNECOPriceInvalidReserve
	}
	return necoV2PairState{
		token0:    normalizeAddress(token0.Hex()),
		token1:    normalizeAddress(token1.Hex()),
		reserve0:  reserve0,
		reserve1:  reserve1,
		timestamp: reserveTimestamp(reserves),
	}, nil
}

func reserveTimestamp(reserves []interface{}) uint32 {
	if len(reserves) < 3 {
		return 0
	}
	timestamp, ok := reserves[2].(uint32)
	if !ok {
		return 0
	}
	return timestamp
}

func (o *NECOV2PairPriceOracle) callAddress(ctx context.Context, pairAddress, method string) (common.Address, error) {
	output, err := o.call(ctx, pairAddress, method)
	if err != nil {
		return common.Address{}, err
	}
	if len(output) != 1 {
		return common.Address{}, ErrNECOPriceInvalidPair
	}
	address, ok := output[0].(common.Address)
	if !ok {
		return common.Address{}, ErrNECOPriceInvalidPair
	}
	return address, nil
}

func (o *NECOV2PairPriceOracle) call(ctx context.Context, pairAddress, method string) ([]interface{}, error) {
	input, err := o.abi.Pack(method)
	if err != nil {
		return nil, err
	}
	to := common.HexToAddress(pairAddress)
	output, err := o.caller.CallContract(ctx, ethereum.CallMsg{
		To:   &to,
		Data: input,
	}, nil)
	if err != nil {
		return nil, err
	}
	return o.abi.Unpack(method, output)
}

type necoV2PairState struct {
	token0    string
	token1    string
	reserve0  *big.Int
	reserve1  *big.Int
	timestamp uint32
}

func (p necoV2PairState) reservesFor(firstToken, secondToken string) (*big.Int, *big.Int, error) {
	first := normalizeAddress(firstToken)
	second := normalizeAddress(secondToken)
	switch {
	case p.token0 == first && p.token1 == second:
		return new(big.Int).Set(p.reserve0), new(big.Int).Set(p.reserve1), nil
	case p.token0 == second && p.token1 == first:
		return new(big.Int).Set(p.reserve1), new(big.Int).Set(p.reserve0), nil
	default:
		return nil, nil, ErrNECOPriceInvalidPair
	}
}

type TradingMarginQuoteRequest struct {
	UserID                   string `json:"userId"`
	ClientOrderID            string `json:"clientOrderId"`
	Market                   string `json:"market"`
	MarginAsset              string `json:"marginAsset"`
	Price                    string `json:"price"`
	Size                     string `json:"size"`
	MinMarginUSD             string `json:"minMarginUsd,omitempty"`
	CollateralQuantumsPerUSD string `json:"collateralQuantumsPerUsd,omitempty"`
}

type TradingMarginQuote struct {
	ChargeAsset     string `json:"chargeAsset"`
	ChargeAmount    string `json:"chargeAmount"`
	MarginUSD       string `json:"marginUsd"`
	PriceSnapshotID string `json:"priceSnapshotId"`
	DEDXUSDPrice    string `json:"dedxUsdPrice"`
	AssetUSDPrice   string `json:"assetUsdPrice,omitempty"`
	PriceRoute      string `json:"priceRoute,omitempty"`
}

type TradingMarginQuoter interface {
	QuoteMargin(context.Context, TradingMarginQuoteRequest) (TradingMarginQuote, error)
}

type DEDXMarginQuoter struct {
	oracle       DEDXUSDOracle
	dedxDecimals uint8
}

func NewDEDXMarginQuoter(oracle DEDXUSDOracle, dedxDecimals uint8) *DEDXMarginQuoter {
	return &DEDXMarginQuoter{oracle: oracle, dedxDecimals: dedxDecimals}
}

type AssetMarginQuoter struct {
	mapper   *AssetMapper
	mappings []AssetMapping
	oracle   AssetUSDOracle
}

func NewAssetMarginQuoter(mappings []AssetMapping, oracle AssetUSDOracle) *AssetMarginQuoter {
	return &AssetMarginQuoter{
		mapper:   NewAssetMapper(mappings),
		mappings: mappings,
		oracle:   oracle,
	}
}

func (q *AssetMarginQuoter) QuoteMargin(ctx context.Context, request TradingMarginQuoteRequest) (TradingMarginQuote, error) {
	if q == nil || q.oracle == nil {
		return TradingMarginQuote{}, ErrNECOPriceOracleNotConfigured
	}
	mapping, ok := q.tradableMapping(request.MarginAsset)
	if !ok {
		return TradingMarginQuote{}, ErrTradingBalanceAssetNotConfigured
	}
	price, ok := parseDecimalRat(request.Price)
	if !ok || price.Sign() <= 0 {
		return TradingMarginQuote{}, ErrTradingQuoteInvalidAmount
	}
	size, ok := parseDecimalRat(request.Size)
	if !ok || size.Sign() <= 0 {
		return TradingMarginQuote{}, ErrTradingQuoteInvalidAmount
	}
	marginUSD := new(big.Rat).Mul(price, size)
	marginUSD, err := applyMinimumMarginUSD(marginUSD, request.MinMarginUSD)
	if err != nil {
		return TradingMarginQuote{}, err
	}
	marginUSD, err = applyCollateralQuantumMarginUSD(marginUSD, request.CollateralQuantumsPerUSD)
	if err != nil {
		return TradingMarginQuote{}, err
	}
	snapshot, err := q.oracle.AssetUSDPrice(ctx, mapping.NECOAsset)
	if err != nil {
		return TradingMarginQuote{}, err
	}
	assetUSDPrice, ok := parseDecimalRat(snapshot.USDPrice)
	if !ok || assetUSDPrice.Sign() <= 0 {
		return TradingMarginQuote{}, ErrTradingQuoteInvalidAmount
	}
	chargeAmount := new(big.Rat).Quo(marginUSD, assetUSDPrice)
	chargeAmount.Mul(chargeAmount, new(big.Rat).SetInt(pow10(mapping.Decimals)))
	quote := TradingMarginQuote{
		ChargeAsset:     mapping.NECOAsset,
		ChargeAmount:    ceilRatToInt(chargeAmount).String(),
		MarginUSD:       ratToDecimalString(marginUSD, 18),
		PriceSnapshotID: snapshot.ID,
		AssetUSDPrice:   snapshot.USDPrice,
		PriceRoute:      defaultString(snapshot.RouteName, mapping.PriceRoute),
	}
	if normalizeAsset(mapping.NECOAsset) == "DEDX" {
		quote.DEDXUSDPrice = snapshot.USDPrice
	}
	return quote, nil
}

func (q *AssetMarginQuoter) tradableMapping(asset string) (AssetMapping, bool) {
	if strings.TrimSpace(asset) != "" {
		mapping, ok := q.mapper.ByNECOAsset(asset)
		return mapping, ok && mapping.EnabledForTrading && mapping.PriceRoute != ""
	}
	for _, mapping := range q.mappings {
		if mapping.EnabledForTrading && mapping.PriceRoute != "" {
			return mapping, true
		}
	}
	return AssetMapping{}, false
}

func (q *DEDXMarginQuoter) QuoteMargin(ctx context.Context, request TradingMarginQuoteRequest) (TradingMarginQuote, error) {
	if q == nil || q.oracle == nil {
		return TradingMarginQuote{}, ErrNECOPriceOracleNotConfigured
	}
	price, ok := parseDecimalRat(request.Price)
	if !ok || price.Sign() <= 0 {
		return TradingMarginQuote{}, ErrTradingQuoteInvalidAmount
	}
	size, ok := parseDecimalRat(request.Size)
	if !ok || size.Sign() <= 0 {
		return TradingMarginQuote{}, ErrTradingQuoteInvalidAmount
	}
	marginUSD := new(big.Rat).Mul(price, size)
	marginUSD, err := applyMinimumMarginUSD(marginUSD, request.MinMarginUSD)
	if err != nil {
		return TradingMarginQuote{}, err
	}
	marginUSD, err = applyCollateralQuantumMarginUSD(marginUSD, request.CollateralQuantumsPerUSD)
	if err != nil {
		return TradingMarginQuote{}, err
	}
	snapshot, err := q.oracle.DEDXUSDPrice(ctx)
	if err != nil {
		return TradingMarginQuote{}, err
	}
	dedxUSDPrice, ok := parseDecimalRat(snapshot.DEDXUSDPrice)
	if !ok || dedxUSDPrice.Sign() <= 0 {
		return TradingMarginQuote{}, ErrTradingQuoteInvalidAmount
	}
	dedxAmount := new(big.Rat).Quo(marginUSD, dedxUSDPrice)
	dedxAmount.Mul(dedxAmount, new(big.Rat).SetInt(pow10(q.dedxDecimals)))
	return TradingMarginQuote{
		ChargeAsset:     "DEDX",
		ChargeAmount:    ceilRatToInt(dedxAmount).String(),
		MarginUSD:       ratToDecimalString(marginUSD, 18),
		PriceSnapshotID: snapshot.ID,
		DEDXUSDPrice:    snapshot.DEDXUSDPrice,
		AssetUSDPrice:   snapshot.DEDXUSDPrice,
		PriceRoute:      "DEDX-NECO-USD",
	}, nil
}

func routeMinLiquidityUSD(fromAmounts, toAmounts, hopPrices []*big.Rat) (*big.Rat, error) {
	if len(fromAmounts) == 0 || len(fromAmounts) != len(toAmounts) || len(fromAmounts) != len(hopPrices) {
		return nil, ErrNECOPriceInvalidReserve
	}
	suffixPrices := make([]*big.Rat, len(hopPrices)+1)
	suffixPrices[len(hopPrices)] = big.NewRat(1, 1)
	for i := len(hopPrices) - 1; i >= 0; i-- {
		suffixPrices[i] = new(big.Rat).Mul(hopPrices[i], suffixPrices[i+1])
	}
	var minLiquidity *big.Rat
	for i := range hopPrices {
		fromUSD := new(big.Rat).Mul(fromAmounts[i], suffixPrices[i])
		toUSD := new(big.Rat).Mul(toAmounts[i], suffixPrices[i+1])
		liquidity := new(big.Rat).Add(fromUSD, toUSD)
		if !positiveRat(liquidity) {
			return nil, ErrNECOPriceInvalidReserve
		}
		if minLiquidity == nil || liquidity.Cmp(minLiquidity) < 0 {
			minLiquidity = liquidity
		}
	}
	return minLiquidity, nil
}

func normalizePairAddresses(pairs []string) []string {
	out := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, normalizeAddress(pair))
	}
	return out
}

func normalizeAsset(asset string) string {
	return strings.ToUpper(strings.TrimSpace(asset))
}

func scaleAtomicAmount(amount *big.Int, decimals uint8) *big.Rat {
	return new(big.Rat).SetFrac(new(big.Int).Set(amount), pow10(decimals))
}

func pow10(decimals uint8) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
}

func positiveRat(value *big.Rat) bool {
	return value != nil && value.Sign() > 0
}

func minRat(left, right *big.Rat) *big.Rat {
	if left.Cmp(right) <= 0 {
		return new(big.Rat).Set(left)
	}
	return new(big.Rat).Set(right)
}

func absRat(value *big.Rat) *big.Rat {
	if value.Sign() < 0 {
		return value.Neg(value)
	}
	return value
}

func parseDecimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if strings.HasPrefix(value, "+") {
		value = strings.TrimPrefix(value, "+")
	}
	if strings.HasPrefix(value, "-") {
		rat, ok := parseDecimalRat(strings.TrimPrefix(value, "-"))
		if !ok {
			return nil, false
		}
		return rat.Neg(rat), true
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return nil, false
	}
	whole := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if whole == "" {
		whole = "0"
	}
	if whole == "" && fraction == "" {
		return nil, false
	}
	digits := whole + fraction
	if digits == "" {
		return nil, false
	}
	numerator, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, false
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(fraction))), nil)
	return new(big.Rat).SetFrac(numerator, denominator), true
}

func ratToDecimalString(value *big.Rat, precision int) string {
	if value == nil {
		return "0"
	}
	out := value.FloatString(precision)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" || out == "-0" {
		return "0"
	}
	return out
}

func applyMinimumMarginUSD(marginUSD *big.Rat, minMarginUSD string) (*big.Rat, error) {
	if marginUSD == nil {
		return big.NewRat(0, 1), nil
	}
	minMarginUSD = strings.TrimSpace(minMarginUSD)
	if minMarginUSD == "" {
		return marginUSD, nil
	}
	minimum, ok := parseDecimalRat(minMarginUSD)
	if !ok || minimum.Sign() < 0 {
		return nil, ErrTradingQuoteInvalidAmount
	}
	if marginUSD.Cmp(minimum) < 0 {
		return minimum, nil
	}
	return marginUSD, nil
}

func applyCollateralQuantumMarginUSD(marginUSD *big.Rat, collateralQuantumsPerUSD string) (*big.Rat, error) {
	if marginUSD == nil {
		return big.NewRat(0, 1), nil
	}
	collateralQuantumsPerUSD = strings.TrimSpace(collateralQuantumsPerUSD)
	if collateralQuantumsPerUSD == "" {
		return marginUSD, nil
	}
	scale, ok := parseDecimalRat(collateralQuantumsPerUSD)
	if !ok || scale.Sign() <= 0 {
		return nil, ErrTradingQuoteInvalidAmount
	}
	rawQuantums := new(big.Rat).Mul(marginUSD, scale)
	quantums := ceilRatToInt(rawQuantums)
	if quantums.Sign() <= 0 || quantums.BitLen() > 64 {
		return nil, ErrTradingQuoteInvalidAmount
	}
	return new(big.Rat).Quo(new(big.Rat).SetInt(quantums), scale), nil
}

func marginUSDAlignedToCollateralQuantum(marginUSD *big.Rat, collateralQuantumsPerUSD string) (bool, error) {
	aligned, err := applyCollateralQuantumMarginUSD(marginUSD, collateralQuantumsPerUSD)
	if err != nil {
		return false, err
	}
	return aligned.Cmp(marginUSD) == 0, nil
}

func ceilRatToInt(value *big.Rat) *big.Int {
	num := new(big.Int).Set(value.Num())
	den := new(big.Int).Set(value.Denom())
	if value.Sign() <= 0 {
		return big.NewInt(0)
	}
	quo, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() > 0 {
		quo.Add(quo, big.NewInt(1))
	}
	return quo
}

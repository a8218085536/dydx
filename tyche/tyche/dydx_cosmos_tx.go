package tyche

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DYDXCosmosTxParamKey            = "cosmosTx"
	DYDXCosmosSignModeDirect        = "SIGN_MODE_DIRECT"
	DYDXCosmosBroadcastModeSync     = "BROADCAST_MODE_SYNC"
	DYDXCosmosMsgPlaceOrderTypeURL  = "/dydxprotocol.clob.MsgPlaceOrder"
	DYDXCosmosMsgCancelOrderTypeURL = "/dydxprotocol.clob.MsgCancelOrder"

	DYDXOrderFlagsShortTerm = uint32(0)
	DYDXOrderFlagsLongTerm  = uint32(64)

	DYDXOrderSideBuy  = uint32(1)
	DYDXOrderSideSell = uint32(2)

	DYDXTimeInForceUnspecified = uint32(0)
	DYDXTimeInForceIOC         = uint32(1)
	DYDXTimeInForcePostOnly    = uint32(2)

	dydxStatefulOrderTimeWindowSeconds = uint32(95 * 24 * 60 * 60)
	defaultDYDXGoodTilSeconds          = uint32(7 * 24 * 60 * 60)
	dydxShortTermOrderBlockWindow      = uint32(40)
	defaultDYDXGoodTilBlockDelta       = uint32(20)
)

var (
	ErrDYDXOrderBuilderNotConfigured      = errors.New("dydx order builder is not configured")
	ErrDYDXHeightProviderNotConfigured    = errors.New("dydx height provider is not configured")
	ErrDYDXChainIDNotConfigured           = errors.New("dydx chain id is not configured")
	ErrDYDXAccountNotConfigured           = errors.New("dydx account is not configured")
	ErrDYDXMarketNotConfigured            = errors.New("dydx market is not configured")
	ErrInvalidDYDXClientOrderID           = errors.New("invalid dydx client order id")
	ErrInvalidDYDXOrderSide               = errors.New("invalid dydx order side")
	ErrInvalidDYDXOrderType               = errors.New("invalid dydx order type")
	ErrInvalidDYDXAmount                  = errors.New("invalid dydx amount")
	ErrDYDXGoodTilBlockDeltaTooLarge      = errors.New("dydx goodTilBlock delta exceeds short-term window")
	ErrDYDXGoodTilSecondsTooLarge         = errors.New("dydx goodTil seconds exceeds stateful order time window")
	ErrDYDXBlockHeightTooLarge            = errors.New("dydx block height exceeds uint32")
	ErrDYDXBlockTimeProviderNotConfigured = errors.New("dydx block time provider is not configured")
	ErrDYDXBlockTimeTooLarge              = errors.New("dydx block time exceeds uint32")
)

type DYDXMarketConfig struct {
	Market           string `json:"market" toml:"market"`
	ClobPairID       uint32 `json:"clobPairId" toml:"clobPairId"`
	QuantumsPerUnit  string `json:"quantumsPerUnit" toml:"quantumsPerUnit"`
	StepBaseQuantums string `json:"stepBaseQuantums" toml:"stepBaseQuantums"`
	SubticksPerUnit  string `json:"subticksPerUnit" toml:"subticksPerUnit"`
	SubticksPerTick  string `json:"subticksPerTick" toml:"subticksPerTick"`
}

func (m DYDXMarketConfig) subticksPerTick() (uint64, error) {
	value := strings.TrimSpace(m.SubticksPerTick)
	if value == "" {
		return defaultSubticksPerTickForClobPair(m.ClobPairID), nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, ErrInvalidDYDXAmount
	}
	return parsed, nil
}

func (m DYDXMarketConfig) stepBaseQuantums() (uint64, error) {
	value := strings.TrimSpace(m.StepBaseQuantums)
	if value == "" {
		return 1, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, ErrInvalidDYDXAmount
	}
	return parsed, nil
}

func defaultSubticksPerTickForClobPair(clobPairID uint32) uint64 {
	switch clobPairID {
	case 0, 1:
		return 100000
	default:
		return 1000000
	}
}

type DYDXCosmosOrderBuilderConfig struct {
	ChainID          string `json:"chainId" toml:"chainId"`
	Account          string `json:"account" toml:"account"`
	SubaccountNumber uint32 `json:"subaccountNumber" toml:"subaccountNumber"`
	// GoodTilBlockDelta is retained for legacy config compatibility. Long-term orders use GoodTilSeconds.
	GoodTilBlockDelta uint32             `json:"goodTilBlockDelta" toml:"goodTilBlockDelta"`
	GoodTilSeconds    uint32             `json:"goodTilSeconds" toml:"goodTilSeconds"`
	Markets           []DYDXMarketConfig `json:"markets" toml:"markets"`
}

type DYDXCosmosOrderBuilder struct {
	conf    DYDXCosmosOrderBuilderConfig
	markets map[string]DYDXMarketConfig
}

func NewDYDXCosmosOrderBuilder(conf DYDXCosmosOrderBuilderConfig) *DYDXCosmosOrderBuilder {
	if conf.GoodTilSeconds == 0 {
		conf.GoodTilSeconds = defaultDYDXGoodTilSeconds
	}
	if conf.GoodTilBlockDelta == 0 {
		conf.GoodTilBlockDelta = defaultDYDXGoodTilBlockDelta
	}
	builder := &DYDXCosmosOrderBuilder{
		conf:    conf,
		markets: map[string]DYDXMarketConfig{},
	}
	for _, market := range conf.Markets {
		builder.markets[normalizeDYDXMarket(market.Market)] = market
	}
	return builder
}

type DYDXCosmosTxRequest struct {
	ID            string        `json:"id"`
	Action        string        `json:"action"`
	Account       string        `json:"account"`
	ChainID       string        `json:"chainId"`
	SignMode      string        `json:"signMode"`
	BroadcastMode string        `json:"broadcastMode"`
	Message       DYDXCosmosAny `json:"message"`
}

type DYDXCosmosAny struct {
	TypeURL string      `json:"typeUrl"`
	Value   interface{} `json:"value"`
}

type DYDXCosmosMsgPlaceOrder struct {
	Order DYDXCosmosOrder `json:"order"`
}

type DYDXCosmosMsgCancelOrder struct {
	OrderID          DYDXCosmosOrderID `json:"orderId"`
	GoodTilBlock     uint32            `json:"goodTilBlock"`
	GoodTilBlockTime uint32            `json:"goodTilBlockTime"`
}

type DYDXCosmosOrder struct {
	OrderID          DYDXCosmosOrderID `json:"orderId"`
	Side             uint32            `json:"side"`
	Quantums         uint64            `json:"quantums"`
	Subticks         uint64            `json:"subticks"`
	GoodTilBlock     uint32            `json:"goodTilBlock"`
	GoodTilBlockTime uint32            `json:"goodTilBlockTime"`
	TimeInForce      uint32            `json:"timeInForce"`
	ReduceOnly       bool              `json:"reduceOnly"`
	ClientMetadata   uint32            `json:"clientMetadata"`
}

type DYDXCosmosOrderID struct {
	SubaccountID DYDXCosmosSubaccountID `json:"subaccountId"`
	ClientID     uint32                 `json:"clientId"`
	OrderFlags   uint32                 `json:"orderFlags"`
	ClobPairID   uint32                 `json:"clobPairId"`
}

type DYDXCosmosSubaccountID struct {
	Owner  string `json:"owner"`
	Number uint32 `json:"number"`
}

func (b *DYDXCosmosOrderBuilder) BuildPlaceOrder(order DYDXPlaceOrder, currentBlockValues ...uint64) (DYDXCosmosTxRequest, error) {
	if err := b.validateBaseConfig(); err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	currentBlockHeight, currentBlockTimeUnix := dydxBlockInputs(currentBlockValues)
	account := b.accountForOrder(order.DYDXAccount)
	if account == "" {
		return DYDXCosmosTxRequest{}, ErrDYDXAccountNotConfigured
	}
	market, err := b.market(order.Market)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	subaccountNumber := b.subaccountNumberForOrder(order.SubaccountNumber, order.Market, market.ClobPairID)
	clientID, err := parseDYDXClientID(order.ClientOrderID)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	side, err := parseDYDXSide(order.Side)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	timeInForce, err := parseDYDXTimeInForce(order.OrderType)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	if order.ReduceOnly {
		timeInForce = DYDXTimeInForceIOC
	}
	quantums, err := parseScaledUint64(order.Size, market.QuantumsPerUnit)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	stepBaseQuantums, err := market.stepBaseQuantums()
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	if stepBaseQuantums == 0 || quantums%stepBaseQuantums != 0 {
		return DYDXCosmosTxRequest{}, ErrInvalidDYDXAmount
	}
	subticks, err := parseScaledUint64(order.Price, market.SubticksPerUnit)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	subticksPerTick, err := market.subticksPerTick()
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	if subticksPerTick == 0 || subticks%subticksPerTick != 0 {
		return DYDXCosmosTxRequest{}, ErrInvalidDYDXAmount
	}
	orderFlags := DYDXOrderFlagsLongTerm
	var goodTilBlock uint32
	var goodTilBlockTime uint32
	if timeInForce == DYDXTimeInForceIOC {
		orderFlags = DYDXOrderFlagsShortTerm
		goodTilBlock, err = b.goodTilBlock(currentBlockHeight)
		if err != nil {
			return DYDXCosmosTxRequest{}, err
		}
	} else {
		goodTilBlockTime, err = b.goodTilBlockTime(currentBlockTimeUnix)
		if err != nil {
			return DYDXCosmosTxRequest{}, err
		}
	}
	message := DYDXCosmosMsgPlaceOrder{
		Order: DYDXCosmosOrder{
			OrderID: DYDXCosmosOrderID{
				SubaccountID: DYDXCosmosSubaccountID{
					Owner:  account,
					Number: subaccountNumber,
				},
				ClientID:   clientID,
				OrderFlags: orderFlags,
				ClobPairID: market.ClobPairID,
			},
			Side:             side,
			Quantums:         quantums,
			Subticks:         subticks,
			GoodTilBlock:     goodTilBlock,
			GoodTilBlockTime: goodTilBlockTime,
			TimeInForce:      timeInForce,
			ReduceOnly:       order.ReduceOnly,
		},
	}
	return b.tx(order.ID, DYDXActionPlaceOrder, account, DYDXCosmosMsgPlaceOrderTypeURL, message), nil
}

func (b *DYDXCosmosOrderBuilder) BuildCancelOrder(order DYDXCancelOrder, currentBlockTimeUnix uint64) (DYDXCosmosTxRequest, error) {
	if err := b.validateBaseConfig(); err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	account := b.accountForOrder(order.DYDXAccount)
	if account == "" {
		return DYDXCosmosTxRequest{}, ErrDYDXAccountNotConfigured
	}
	market, err := b.market(order.Market)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	subaccountNumber := b.subaccountNumberForOrder(order.SubaccountNumber, order.Market, market.ClobPairID)
	clientID, err := parseDYDXClientID(order.ClientOrderID)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	goodTilBlockTime, err := b.goodTilBlockTime(currentBlockTimeUnix)
	if err != nil {
		return DYDXCosmosTxRequest{}, err
	}
	message := DYDXCosmosMsgCancelOrder{
		OrderID: DYDXCosmosOrderID{
			SubaccountID: DYDXCosmosSubaccountID{
				Owner:  account,
				Number: subaccountNumber,
			},
			ClientID:   clientID,
			OrderFlags: DYDXOrderFlagsLongTerm,
			ClobPairID: market.ClobPairID,
		},
		GoodTilBlockTime: goodTilBlockTime,
	}
	return b.tx(order.ID, DYDXActionCancelOrder, account, DYDXCosmosMsgCancelOrderTypeURL, message), nil
}

func (b *DYDXCosmosOrderBuilder) tx(id, action, account, typeURL string, message interface{}) DYDXCosmosTxRequest {
	return DYDXCosmosTxRequest{
		ID:            id,
		Action:        action,
		Account:       account,
		ChainID:       b.conf.ChainID,
		SignMode:      DYDXCosmosSignModeDirect,
		BroadcastMode: DYDXCosmosBroadcastModeSync,
		Message: DYDXCosmosAny{
			TypeURL: typeURL,
			Value:   message,
		},
	}
}

func (b *DYDXCosmosOrderBuilder) validateBaseConfig() error {
	if b == nil {
		return ErrDYDXOrderBuilderNotConfigured
	}
	if b.conf.ChainID == "" {
		return ErrDYDXChainIDNotConfigured
	}
	if b.conf.GoodTilSeconds > dydxStatefulOrderTimeWindowSeconds {
		return ErrDYDXGoodTilSecondsTooLarge
	}
	if b.conf.GoodTilBlockDelta > dydxShortTermOrderBlockWindow {
		return ErrDYDXGoodTilBlockDeltaTooLarge
	}
	return nil
}

func (b *DYDXCosmosOrderBuilder) accountForOrder(account string) string {
	return firstNonEmpty(account, b.conf.Account)
}

func (b *DYDXCosmosOrderBuilder) market(marketName string) (DYDXMarketConfig, error) {
	market, ok := b.markets[normalizeDYDXMarket(marketName)]
	if !ok {
		return DYDXMarketConfig{}, ErrDYDXMarketNotConfigured
	}
	if market.QuantumsPerUnit == "" || market.SubticksPerUnit == "" {
		return DYDXMarketConfig{}, ErrInvalidDYDXAmount
	}
	return market, nil
}

func (b *DYDXCosmosOrderBuilder) subaccountNumberForOrder(explicit uint32, market string, clobPairID uint32) uint32 {
	fallback := defaultDYDXTradingSubaccountNumber
	if b != nil {
		fallback = b.conf.SubaccountNumber
	}
	return explicitOrBuiltInDYDXSubaccountNumber(explicit, market, clobPairID, fallback)
}

func (b *DYDXCosmosOrderBuilder) goodTilBlockTime(currentBlockTimeUnix uint64) (uint32, error) {
	if b.conf.GoodTilSeconds > dydxStatefulOrderTimeWindowSeconds {
		return 0, ErrDYDXGoodTilSecondsTooLarge
	}
	if currentBlockTimeUnix == 0 {
		return 0, ErrDYDXBlockTimeTooLarge
	}
	goodTilBlockTime := currentBlockTimeUnix + uint64(b.conf.GoodTilSeconds)
	if goodTilBlockTime > math.MaxUint32 {
		return 0, ErrDYDXBlockTimeTooLarge
	}
	return uint32(goodTilBlockTime), nil
}

func (b *DYDXCosmosOrderBuilder) goodTilBlock(currentBlockHeight uint64) (uint32, error) {
	if currentBlockHeight == 0 {
		return 0, ErrDYDXHeightProviderNotConfigured
	}
	if b.conf.GoodTilBlockDelta > dydxShortTermOrderBlockWindow {
		return 0, ErrDYDXGoodTilBlockDeltaTooLarge
	}
	goodTilBlock := currentBlockHeight + uint64(b.conf.GoodTilBlockDelta)
	if goodTilBlock > math.MaxUint32 {
		return 0, ErrDYDXBlockHeightTooLarge
	}
	return uint32(goodTilBlock), nil
}

func dydxBlockInputs(values []uint64) (uint64, uint64) {
	switch len(values) {
	case 0:
		return 0, 0
	case 1:
		return 0, values[0]
	default:
		return values[0], values[1]
	}
}

type DYDXBlockHeightProvider interface {
	CurrentBlockHeight(context.Context) (uint64, error)
}

type DYDXBlockTimeProvider interface {
	CurrentBlockTimeUnix(context.Context) (uint64, error)
}

type DYDXCosmosRequestFactoryConfig struct {
	Builder        *DYDXCosmosOrderBuilder
	HeightProvider DYDXBlockHeightProvider
	TimeProvider   DYDXBlockTimeProvider
}

type DYDXCosmosRequestFactory struct {
	builder        *DYDXCosmosOrderBuilder
	heightProvider DYDXBlockHeightProvider
	timeProvider   DYDXBlockTimeProvider
}

func NewDYDXCosmosRequestFactory(conf DYDXCosmosRequestFactoryConfig) *DYDXCosmosRequestFactory {
	return &DYDXCosmosRequestFactory{
		builder:        conf.Builder,
		heightProvider: conf.HeightProvider,
		timeProvider:   firstDYDXBlockTimeProvider(conf.TimeProvider, conf.HeightProvider),
	}
}

func (f *DYDXCosmosRequestFactory) ParamsForIntent(ctx context.Context, intent DYDXIntent) (map[string]interface{}, error) {
	if f == nil || f.builder == nil {
		return nil, ErrDYDXOrderBuilderNotConfigured
	}
	if f.timeProvider == nil {
		return nil, ErrDYDXBlockTimeProviderNotConfigured
	}
	blockTimeUnix, err := f.timeProvider.CurrentBlockTimeUnix(ctx)
	if err != nil {
		return nil, err
	}

	var tx DYDXCosmosTxRequest
	switch intent.Action {
	case DYDXActionPlaceOrder:
		order := dydxPlaceOrderFromIntent(intent)
		if f.builder.placeOrderRequiresShortTerm(order) {
			if f.heightProvider == nil {
				return nil, ErrDYDXHeightProviderNotConfigured
			}
			var blockHeight uint64
			blockHeight, err = f.heightProvider.CurrentBlockHeight(ctx)
			if err != nil {
				return nil, err
			}
			tx, err = f.builder.BuildPlaceOrder(order, blockHeight, blockTimeUnix)
		} else {
			tx, err = f.builder.BuildPlaceOrder(order, blockTimeUnix)
		}
	case DYDXActionCancelOrder:
		tx, err = f.builder.BuildCancelOrder(dydxCancelOrderFromIntent(intent), blockTimeUnix)
	default:
		err = ErrUnsupportedDYDXAction
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{DYDXCosmosTxParamKey: tx}, nil
}

func firstDYDXBlockTimeProvider(timeProvider DYDXBlockTimeProvider, heightProvider DYDXBlockHeightProvider) DYDXBlockTimeProvider {
	if timeProvider != nil {
		return timeProvider
	}
	if provider, ok := heightProvider.(DYDXBlockTimeProvider); ok {
		return provider
	}
	return nil
}

func (b *DYDXCosmosOrderBuilder) placeOrderRequiresShortTerm(order DYDXPlaceOrder) bool {
	if order.ReduceOnly {
		return true
	}
	timeInForce, err := parseDYDXTimeInForce(order.OrderType)
	return err == nil && timeInForce == DYDXTimeInForceIOC
}

type DYDXHTTPBlockHeightProviderConfig struct {
	URL    string
	APIKey string
}

type DYDXHTTPBlockHeightProvider struct {
	conf   DYDXHTTPBlockHeightProviderConfig
	client *http.Client
}

func NewDYDXHTTPBlockHeightProvider(conf DYDXHTTPBlockHeightProviderConfig) *DYDXHTTPBlockHeightProvider {
	return &DYDXHTTPBlockHeightProvider{conf: conf, client: http.DefaultClient}
}

func (p *DYDXHTTPBlockHeightProvider) CurrentBlockHeight(ctx context.Context) (uint64, error) {
	body, err := p.statusBody(ctx)
	if err != nil {
		return 0, err
	}
	return extractDYDXHeight(body)
}

func (p *DYDXHTTPBlockHeightProvider) CurrentBlockTimeUnix(ctx context.Context) (uint64, error) {
	body, err := p.statusBody(ctx)
	if err != nil {
		return 0, err
	}
	return extractDYDXBlockTimeUnix(body)
}

func (p *DYDXHTTPBlockHeightProvider) statusBody(ctx context.Context) (map[string]interface{}, error) {
	if p == nil || p.conf.URL == "" {
		return nil, ErrDYDXHeightProviderNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.conf.URL, nil)
	if err != nil {
		return nil, err
	}
	if p.conf.APIKey != "" {
		req.Header.Set("X-Tyche-Internal-Key", p.conf.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dydx height provider returned status %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body, nil
}

func dydxPlaceOrderFromIntent(intent DYDXIntent) DYDXPlaceOrder {
	return DYDXPlaceOrder{
		ID:               intent.ID,
		UserID:           stringParam(intent.Params, "userId"),
		DYDXAccount:      stringParam(intent.Params, "dydxAccount"),
		SubaccountNumber: uint32Param(intent.Params, "subaccountNumber"),
		ClientOrderID:    stringParam(intent.Params, "clientOrderId"),
		Market:           stringParam(intent.Params, "market"),
		Side:             stringParam(intent.Params, "side"),
		OrderType:        stringParam(intent.Params, "orderType"),
		Price:            stringParam(intent.Params, "price"),
		Size:             stringParam(intent.Params, "size"),
		MarginAsset:      stringParam(intent.Params, "marginAsset"),
		MarginAmount:     stringParam(intent.Params, "marginAmount"),
		ReduceOnly:       boolParam(intent.Params, "reduceOnly"),
	}
}

func dydxCancelOrderFromIntent(intent DYDXIntent) DYDXCancelOrder {
	return DYDXCancelOrder{
		ID:               intent.ID,
		UserID:           stringParam(intent.Params, "userId"),
		DYDXAccount:      stringParam(intent.Params, "dydxAccount"),
		SubaccountNumber: uint32Param(intent.Params, "subaccountNumber"),
		OrderID:          stringParam(intent.Params, "orderId"),
		ClientOrderID:    stringParam(intent.Params, "clientOrderId"),
		Market:           stringParam(intent.Params, "market"),
	}
}

func parseDYDXClientID(value string) (uint32, error) {
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, ErrInvalidDYDXClientOrderID
	}
	return uint32(id), nil
}

func parseDYDXSide(value string) (uint32, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "buy", "bid":
		return DYDXOrderSideBuy, nil
	case "sell", "ask":
		return DYDXOrderSideSell, nil
	default:
		return 0, ErrInvalidDYDXOrderSide
	}
}

func parseDYDXTimeInForce(value string) (uint32, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "limit", "default", "unspecified":
		return DYDXTimeInForceUnspecified, nil
	case "ioc":
		return DYDXTimeInForceIOC, nil
	case "post_only", "post-only", "postonly":
		return DYDXTimeInForcePostOnly, nil
	default:
		return 0, ErrInvalidDYDXOrderType
	}
}

func parseScaledUint64(value, multiplier string) (uint64, error) {
	amount, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || amount.Sign() <= 0 {
		return 0, ErrInvalidDYDXAmount
	}
	scale, ok := new(big.Rat).SetString(strings.TrimSpace(multiplier))
	if !ok || scale.Sign() <= 0 {
		return 0, ErrInvalidDYDXAmount
	}
	scaled := new(big.Rat).Mul(amount, scale)
	if !scaled.IsInt() {
		return 0, ErrInvalidDYDXAmount
	}
	scaledInt := scaled.Num()
	if scaledInt.Sign() <= 0 || scaledInt.BitLen() > 64 {
		return 0, ErrInvalidDYDXAmount
	}
	return scaledInt.Uint64(), nil
}

func extractDYDXHeight(body map[string]interface{}) (uint64, error) {
	if value, ok := body["height"]; ok {
		return parseHeightValue(value)
	}
	if data, ok := body["data"].(map[string]interface{}); ok {
		if value, ok := data["height"]; ok {
			return parseHeightValue(value)
		}
	}
	if result, ok := body["result"].(map[string]interface{}); ok {
		if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
			if value, ok := syncInfo["latest_block_height"]; ok {
				return parseHeightValue(value)
			}
		}
	}
	return 0, errors.New("dydx height provider response missing height")
}

func extractDYDXBlockTimeUnix(body map[string]interface{}) (uint64, error) {
	if value, ok := body["blockTimeUnix"]; ok {
		return parseBlockTimeValue(value)
	}
	if value, ok := body["blockTime"]; ok {
		return parseBlockTimeValue(value)
	}
	if data, ok := body["data"].(map[string]interface{}); ok {
		if value, ok := data["blockTimeUnix"]; ok {
			return parseBlockTimeValue(value)
		}
		if value, ok := data["blockTime"]; ok {
			return parseBlockTimeValue(value)
		}
	}
	if result, ok := body["result"].(map[string]interface{}); ok {
		if syncInfo, ok := result["sync_info"].(map[string]interface{}); ok {
			if value, ok := syncInfo["latest_block_time"]; ok {
				return parseBlockTimeValue(value)
			}
		}
	}
	return 0, errors.New("dydx height provider response missing block time")
}

func parseHeightValue(value interface{}) (uint64, error) {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed != math.Trunc(typed) {
			return 0, errors.New("invalid dydx block height")
		}
		return uint64(typed), nil
	case string:
		height, err := strconv.ParseUint(typed, 10, 64)
		if err != nil || height == 0 {
			return 0, errors.New("invalid dydx block height")
		}
		return height, nil
	default:
		return 0, errors.New("invalid dydx block height")
	}
}

func parseBlockTimeValue(value interface{}) (uint64, error) {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed != math.Trunc(typed) {
			return 0, errors.New("invalid dydx block time")
		}
		return uint64(typed), nil
	case string:
		if unixTime, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64); err == nil && unixTime > 0 {
			return unixTime, nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(typed))
		if err != nil || parsed.Unix() <= 0 {
			return 0, errors.New("invalid dydx block time")
		}
		return uint64(parsed.Unix()), nil
	default:
		return 0, errors.New("invalid dydx block time")
	}
}

func normalizeDYDXMarket(market string) string {
	return strings.ToUpper(strings.TrimSpace(market))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

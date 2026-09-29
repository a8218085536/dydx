package tyche

import (
	"context"
	"encoding/json"
	"errors"
	"ethgo/model/dydxorders"
	"strconv"
	"strings"
	"time"
)

const (
	DYDXActionPlaceOrder  = "place_order"
	DYDXActionCancelOrder = "cancel_order"
)

var (
	ErrDYDXBroadcastNotConfigured = errors.New("dydx broadcast client is not configured")
	ErrUnsupportedDYDXAction      = errors.New("unsupported dydx action")
)

type DYDXIntent struct {
	ID     string                 `json:"id"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params"`
}

type DYDXRunner struct {
	conf                  *Config
	enqueue               func(DYDXIntent) error
	broadcaster           DYDXBroadcaster
	trading               *DYDXTradingHTTPClient
	tradingRequestFactory *DYDXCosmosRequestFactory
}

func NewDYDXRunner(conf *Config) *DYDXRunner {
	return &DYDXRunner{
		conf: conf,
		enqueue: func(intent DYDXIntent) error {
			return dydxorders.Pending(intent.ID, intent.Action, intent.Params)
		},
		broadcaster: NewDYDXHTTPBroadcaster(DYDXHTTPBroadcasterConfig{
			URL:     conf.DYDXBroadcastURL,
			Account: conf.Account,
		}),
		trading: NewDYDXTradingHTTPClient(DYDXTradingHTTPClientConfig{
			URL:         conf.DYDXTradingURL,
			BearerToken: conf.InternalAPIKey,
		}),
		tradingRequestFactory: newDYDXCosmosRequestFactoryFromConfig(conf),
	}
}

func (r *DYDXRunner) Init(ctx context.Context) error {
	return nil
}

func (r *DYDXRunner) Run(ctx context.Context) error {
	if r.conf != nil && r.conf.DYDXTradingURL != "" {
		<-ctx.Done()
		return nil
	}

	reader, err := dydxorders.NewPendingReader("dydx-pending", "dydx-worker")
	if err != nil {
		return err
	}

	processor := NewDYDXProcessor(r.broadcaster, DYDXStatusStoreFunc(func(id, status, txHash, reason string) error {
		return dydxorders.SetStatus(id, status, txHash, reason)
	}))

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		messages, err := reader.ReadLatest(16, 1000)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}

		for _, message := range messages {
			intent, err := DYDXIntentFromMessageData(message.Data)
			if err == nil {
				err = processor.Process(ctx, intent)
			}
			if err == nil {
				_ = message.Ack()
			}
		}
	}
}

func (r *DYDXRunner) Submit(ctx context.Context, intent DYDXIntent) error {
	_, err := r.SubmitWithResult(ctx, intent)
	return err
}

func (r *DYDXRunner) SubmitWithResult(ctx context.Context, intent DYDXIntent) (DYDXTradingResult, error) {
	switch intent.Action {
	case DYDXActionPlaceOrder, DYDXActionCancelOrder:
		if r.conf != nil && r.conf.DYDXTradingURL != "" {
			return r.submitTrading(ctx, intent)
		}
		if err := r.enqueue(intent); err != nil {
			return DYDXTradingResult{}, err
		}
		return DYDXTradingResult{}, nil
	default:
		return DYDXTradingResult{}, ErrUnsupportedDYDXAction
	}
}

func (r *DYDXRunner) submitTrading(ctx context.Context, intent DYDXIntent) (DYDXTradingResult, error) {
	if r.tradingRequestFactory != nil {
		params, err := r.tradingRequestFactory.ParamsForIntent(ctx, intent)
		if err != nil {
			return DYDXTradingResult{}, err
		}
		tx, ok := params[DYDXCosmosTxParamKey].(DYDXCosmosTxRequest)
		if !ok {
			return DYDXTradingResult{}, errors.New("dydx cosmos request factory returned invalid tx")
		}
		return r.trading.SubmitCosmosTx(ctx, tx)
	}

	switch intent.Action {
	case DYDXActionPlaceOrder:
		return r.trading.PlaceOrder(ctx, DYDXPlaceOrder{
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
		})
	case DYDXActionCancelOrder:
		return r.trading.CancelOrder(ctx, DYDXCancelOrder{
			ID:               intent.ID,
			UserID:           stringParam(intent.Params, "userId"),
			DYDXAccount:      stringParam(intent.Params, "dydxAccount"),
			SubaccountNumber: uint32Param(intent.Params, "subaccountNumber"),
			OrderID:          stringParam(intent.Params, "orderId"),
			ClientOrderID:    stringParam(intent.Params, "clientOrderId"),
			Market:           stringParam(intent.Params, "market"),
		})
	default:
		return DYDXTradingResult{}, ErrUnsupportedDYDXAction
	}
}

func newDYDXCosmosRequestFactoryFromConfig(conf *Config) *DYDXCosmosRequestFactory {
	if conf == nil || conf.DYDXTradingURL == "" {
		return nil
	}
	var heightProvider DYDXBlockHeightProvider
	if conf.DYDXHeightURL != "" {
		heightProvider = NewDYDXHTTPBlockHeightProvider(DYDXHTTPBlockHeightProviderConfig{
			URL:    conf.DYDXHeightURL,
			APIKey: conf.InternalAPIKey,
		})
	}
	return NewDYDXCosmosRequestFactory(DYDXCosmosRequestFactoryConfig{
		Builder: NewDYDXCosmosOrderBuilder(DYDXCosmosOrderBuilderConfig{
			ChainID:          conf.DYDXChainID,
			SubaccountNumber: conf.DYDXSubaccountNumber,
			GoodTilSeconds:   conf.DYDXGoodTilSeconds,
			Markets:          conf.DYDXMarkets,
		}),
		HeightProvider: heightProvider,
		TimeProvider:   firstDYDXBlockTimeProvider(nil, heightProvider),
	})
}

func stringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	if value, ok := params[key].(string); ok {
		return value
	}
	return ""
}

func boolParam(params map[string]interface{}, key string) bool {
	if params == nil {
		return false
	}
	value, ok := params[key].(bool)
	return ok && value
}

func uint32Param(params map[string]interface{}, key string) uint32 {
	if params == nil {
		return 0
	}
	switch value := params[key].(type) {
	case uint32:
		return value
	case uint64:
		if value > uint64(^uint32(0)) {
			return 0
		}
		return uint32(value)
	case uint:
		if uint64(value) > uint64(^uint32(0)) {
			return 0
		}
		return uint32(value)
	case int:
		if value < 0 {
			return 0
		}
		return uint32(value)
	case int64:
		if value < 0 || value > int64(^uint32(0)) {
			return 0
		}
		return uint32(value)
	case float64:
		if value < 0 || value > float64(^uint32(0)) || value != float64(uint32(value)) {
			return 0
		}
		return uint32(value)
	case json.Number:
		parsed, err := strconv.ParseUint(value.String(), 10, 32)
		if err != nil {
			return 0
		}
		return uint32(parsed)
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
		if err != nil {
			return 0
		}
		return uint32(parsed)
	default:
		return 0
	}
}

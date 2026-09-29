package tyche

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	TradingOrderStatusPendingSubmit      = "pending_submit"
	TradingOrderStatusBroadcasted        = "broadcasted"
	TradingOrderStatusSubmitted          = "submitted"
	TradingOrderStatusClosePending       = "close_pending"
	TradingOrderStatusCloseBroadcasted   = "close_broadcasted"
	TradingOrderStatusCloseSubmitted     = "close_submitted"
	TradingOrderStatusClosePartialFilled = "close_partial_filled"
	TradingOrderStatusClosed             = "closed"
	TradingOrderStatusCloseFailed        = "close_failed"
	TradingOrderStatusCloseCancelled     = "close_cancelled"
	TradingOrderStatusFundingRequired    = "funding_required"
	TradingOrderStatusDYDXFundingFailed  = "dydx_funding_failed"
	TradingOrderStatusCancelPending      = "cancel_pending"
	TradingOrderStatusSubmitFailed       = "submit_failed"
	TradingOrderStatusEquityTierLimit    = "equity_tier_limit"
	TradingOrderStatusCrossesPosition    = "order_crosses_existing_position"
	TradingOrderStatusRejected           = "rejected"
	TradingOrderStatusPartialFilled      = "partial_filled"
	TradingOrderStatusFilled             = "filled"
	TradingOrderStatusCancelled          = "cancelled"
)

const (
	TradingOrderTypeLimit    = "limit"
	TradingOrderTypeIOC      = "ioc"
	TradingOrderTypePostOnly = "post_only"
	TradingOrderTypeMarket   = "market"

	TradingFundingStatusWalletActionRequired = "wallet_action_required"
	TradingFundingStatusPendingConfirmation  = "pending_confirmation"
	TradingFundingStatusReady                = "ready"

	TradingFundingActionTransfer = "transfer"
	TradingFundingActionApprove  = "approve"
	TradingFundingActionPermit   = "permit"
)

const (
	defaultMarketOrderSlippageBps uint32 = 100
	maxMarketOrderSlippageBps     uint32 = 5000

	defaultDYDXMinStatefulOrderMarginUSD = "20"
	noDYDXRetainedCollateralUSD          = "0"
	defaultDYDXRetainedCollateralUSD     = "1"

	TradingOrderEquityTierLimitReason = "当前权益等级只允许 1 个未成交开仓挂单"
)

var ErrTradingOrderNotFound = errors.New("trading order not found")
var ErrDYDXOrderRejected = errors.New("dydx order rejected")
var ErrTradingDYDXBindingRequired = errors.New("active dydx wallet binding is required")
var ErrTradingPositionNotFound = errors.New("trading position not found")
var ErrTradingPositionSizeInvalid = errors.New("trading position size invalid")
var ErrTradingMarketPriceSourceNotConfigured = errors.New("trading market price source is not configured")
var ErrTradingMarketPriceNotFound = errors.New("trading market price not found")
var ErrTradingMarketPriceInvalid = errors.New("trading market price invalid")
var ErrTradingMarketSlippageInvalid = errors.New("trading market slippage invalid")

const TradingOrderCrossesPositionReason = "order crosses existing position"

type TradingPlaceOrderRequest struct {
	UserID                string `json:"userId"`
	NECOAddress           string `json:"necoAddress,omitempty"`
	ClientOrderID         string `json:"clientOrderId"`
	Market                string `json:"market"`
	Side                  string `json:"side"`
	OrderType             string `json:"orderType"`
	ExecutionOrderType    string `json:"executionOrderType,omitempty"`
	Price                 string `json:"price"`
	ReferencePrice        string `json:"referencePrice,omitempty"`
	SlippageBps           uint32 `json:"slippageBps,omitempty"`
	MarketPriceSource     string `json:"marketPriceSource,omitempty"`
	MarketPriceSnapshotID string `json:"marketPriceSnapshotId,omitempty"`
	Size                  string `json:"size"`
	MarginAsset           string `json:"marginAsset"`
	MarginAmount          string `json:"marginAmount"`
}

type TradingClosePositionRequest struct {
	UserID                string `json:"userId"`
	NECOAddress           string `json:"necoAddress,omitempty"`
	ClientOrderID         string `json:"clientOrderId"`
	Market                string `json:"market"`
	OrderType             string `json:"orderType"`
	ExecutionOrderType    string `json:"executionOrderType,omitempty"`
	Price                 string `json:"price"`
	ReferencePrice        string `json:"referencePrice,omitempty"`
	SlippageBps           uint32 `json:"slippageBps,omitempty"`
	MarketPriceSource     string `json:"marketPriceSource,omitempty"`
	MarketPriceSnapshotID string `json:"marketPriceSnapshotId,omitempty"`
	Size                  string `json:"size,omitempty"`
}

type TradingCancelOrderRequest struct {
	UserID        string `json:"userId"`
	ClientOrderID string `json:"clientOrderId"`
	OrderID       string `json:"orderId"`
}

type TradingOrder struct {
	ID                             string `json:"id" redis:"id"`
	UserID                         string `json:"userId" redis:"userId"`
	NECOAddress                    string `json:"necoAddress,omitempty" redis:"necoAddress"`
	DYDXAccount                    string `json:"dydxAccount,omitempty" redis:"dydxAccount"`
	SubaccountNumber               uint32 `json:"subaccountNumber,omitempty" redis:"subaccountNumber"`
	ClientOrderID                  string `json:"clientOrderId" redis:"clientOrderId"`
	Market                         string `json:"market" redis:"market"`
	Side                           string `json:"side" redis:"side"`
	OrderType                      string `json:"orderType" redis:"orderType"`
	ExecutionOrderType             string `json:"executionOrderType,omitempty" redis:"executionOrderType"`
	Price                          string `json:"price" redis:"price"`
	ReferencePrice                 string `json:"referencePrice,omitempty" redis:"referencePrice"`
	SlippageBps                    uint32 `json:"slippageBps,omitempty" redis:"slippageBps"`
	MarketPriceSource              string `json:"marketPriceSource,omitempty" redis:"marketPriceSource"`
	MarketPriceSnapshotID          string `json:"marketPriceSnapshotId,omitempty" redis:"marketPriceSnapshotId"`
	Size                           string `json:"size" redis:"size"`
	MarginAsset                    string `json:"marginAsset" redis:"marginAsset"`
	MarginAmount                   string `json:"marginAmount" redis:"marginAmount"`
	MarginUSD                      string `json:"marginUsd,omitempty" redis:"marginUsd"`
	PriceSnapshotID                string `json:"priceSnapshotId,omitempty" redis:"priceSnapshotId"`
	DEDXUSDPrice                   string `json:"dedxUsdPrice,omitempty" redis:"dedxUsdPrice"`
	AssetUSDPrice                  string `json:"assetUsdPrice,omitempty" redis:"assetUsdPrice"`
	PriceRoute                     string `json:"priceRoute,omitempty" redis:"priceRoute"`
	FundingStatus                  string `json:"fundingStatus,omitempty" redis:"fundingStatus"`
	FundingAction                  string `json:"fundingAction,omitempty" redis:"fundingAction"`
	FundingAsset                   string `json:"fundingAsset,omitempty" redis:"fundingAsset"`
	FundingAmount                  string `json:"fundingAmount,omitempty" redis:"fundingAmount"`
	FundingTokenAddress            string `json:"fundingTokenAddress,omitempty" redis:"fundingTokenAddress"`
	FundingFromAddress             string `json:"fundingFromAddress,omitempty" redis:"fundingFromAddress"`
	FundingToAddress               string `json:"fundingToAddress,omitempty" redis:"fundingToAddress"`
	FundingSpenderAddress          string `json:"fundingSpenderAddress,omitempty" redis:"fundingSpenderAddress"`
	FundingReason                  string `json:"fundingReason,omitempty" redis:"fundingReason"`
	FundingWalletBalance           string `json:"fundingWalletBalance,omitempty" redis:"fundingWalletBalance"`
	FundingLedgerAvailable         string `json:"fundingLedgerAvailable,omitempty" redis:"fundingLedgerAvailable"`
	DYDXFundingTxHash              string `json:"dydxFundingTxHash,omitempty" redis:"dydxFundingTxHash"`
	DYDXFundingStatus              string `json:"dydxFundingStatus,omitempty" redis:"dydxFundingStatus"`
	DYDXFundingAssetID             uint32 `json:"dydxFundingAssetId,omitempty" redis:"dydxFundingAssetId"`
	DYDXFundingQuantums            string `json:"dydxFundingQuantums,omitempty" redis:"dydxFundingQuantums"`
	DYDXOrderMarginQuantums        string `json:"dydxOrderMarginQuantums,omitempty" redis:"dydxOrderMarginQuantums"`
	DYDXRetainedCollateralQuantums string `json:"dydxRetainedCollateralQuantums,omitempty" redis:"dydxRetainedCollateralQuantums"`
	DYDXOrderTxHash                string `json:"dydxOrderTxHash,omitempty" redis:"dydxOrderTxHash"`
	DYDXCancelTxHash               string `json:"dydxCancelTxHash,omitempty" redis:"dydxCancelTxHash"`
	ReduceOnly                     bool   `json:"reduceOnly,omitempty" redis:"reduceOnly"`
	ClosePosition                  bool   `json:"closePosition,omitempty" redis:"closePosition"`
	Status                         string `json:"status" redis:"status"`
	Reason                         string `json:"reason,omitempty" redis:"reason"`
	CreatedAt                      int64  `json:"createdAt,omitempty" redis:"createdAt"`
	UpdatedAt                      int64  `json:"updatedAt,omitempty" redis:"updatedAt"`
}

type TradingStore interface {
	GetOrderByClientID(ctx context.Context, userID, clientOrderID string) (TradingOrder, bool, error)
	GetOrderByID(ctx context.Context, userID, orderID string) (TradingOrder, bool, error)
	SaveOrder(ctx context.Context, order TradingOrder) error
}

type TradingOrderCreateStore interface {
	CreateOrderIfAbsent(ctx context.Context, order TradingOrder) (TradingOrder, bool, error)
}

type TradingService struct {
	store              TradingStore
	ledger             *LedgerService
	submitter          DYDXSubmitter
	marginQuoter       TradingMarginQuoter
	fundingPreparer    TradingFundingPreparer
	accountResolver    TradingDYDXAccountResolver
	positionProvider   TradingPositionProvider
	positionReconcile  DYDXOrderReconcileSource
	marketPrices       TradingMarketPriceSource
	marketSlippage     uint32
	dydxMarkets        map[string]DYDXMarketConfig
	dydxDepositor      DYDXSubaccountDepositor
	dydxFunding        DYDXSubaccountFundingConfig
	retainedCollateral DYDXRetainedCollateralStore
	failedSettlement   TradingFailedOrderSettlementService
}

type DYDXResultSubmitter interface {
	SubmitWithResult(context.Context, DYDXIntent) (DYDXTradingResult, error)
}

type TradingFundingPrepareRequest struct {
	UserID          string `json:"userId"`
	NECOAddress     string `json:"necoAddress"`
	ClientOrderID   string `json:"clientOrderId"`
	Market          string `json:"market"`
	ChargeAsset     string `json:"chargeAsset"`
	ChargeAmount    string `json:"chargeAmount"`
	LedgerAvailable string `json:"ledgerAvailable"`
	MissingAmount   string `json:"missingAmount"`
	RefID           string `json:"refId"`
}

type TradingFundingPrepareResult struct {
	Status          string `json:"status"`
	Action          string `json:"action"`
	Asset           string `json:"asset"`
	Amount          string `json:"amount"`
	TokenAddress    string `json:"tokenAddress"`
	FromAddress     string `json:"fromAddress"`
	ToAddress       string `json:"toAddress"`
	SpenderAddress  string `json:"spenderAddress"`
	Reason          string `json:"reason"`
	WalletBalance   string `json:"walletBalance"`
	LedgerAvailable string `json:"ledgerAvailable"`
}

type TradingFundingPreparer interface {
	PrepareFunding(context.Context, TradingFundingPrepareRequest) (TradingFundingPrepareResult, error)
}

type TradingMarketPriceRequest struct {
	Market string `json:"market"`
	Side   string `json:"side,omitempty"`
}

type TradingMarketPriceSnapshot struct {
	Market     string `json:"market"`
	MarketID   uint32 `json:"marketId,omitempty"`
	Price      string `json:"price"`
	Source     string `json:"source,omitempty"`
	SnapshotID string `json:"snapshotId,omitempty"`
	RawPrice   string `json:"rawPrice,omitempty"`
	Exponent   int32  `json:"exponent,omitempty"`
}

type TradingMarketPriceSource interface {
	MarketPrice(context.Context, TradingMarketPriceRequest) (TradingMarketPriceSnapshot, error)
}

type TradingDYDXAccountResolver interface {
	DYDXAccountForUser(context.Context, string) (string, error)
}

type TradingPositionProvider interface {
	GetPosition(context.Context, string, string) (TradingPosition, error)
}

type TradingSubaccountPositionProvider interface {
	GetPositionForSubaccount(context.Context, string, string, uint32) (TradingPosition, error)
}

type TradingFailedOrderSettlementService interface {
	SettleFailedOrder(context.Context, FailedOrderSettlementRequest) (TradingSettlement, error)
}

type DYDXSubaccountFundingConfig struct {
	CollateralAssetID         uint32
	CollateralQuantumsPerUSD  string
	SubaccountNumber          uint32
	MinStatefulOrderMarginUSD string
	RetainedCollateralUSD     string
}

type DYDXSubaccountDepositRequest struct {
	ID               string `json:"id"`
	UserID           string `json:"userId,omitempty"`
	DYDXAccount      string `json:"dydxAccount"`
	SubaccountNumber uint32 `json:"subaccountNumber"`
	AssetID          uint32 `json:"assetId"`
	Quantums         string `json:"quantums"`
	MarginUSD        string `json:"marginUsd,omitempty"`
}

type DYDXSubaccountDepositResult struct {
	TxHash           string `json:"txHash"`
	Status           string `json:"status"`
	DYDXAccount      string `json:"dydxAccount,omitempty"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
	AssetID          uint32 `json:"assetId"`
	Quantums         string `json:"quantums"`
}

type DYDXSubaccountDepositor interface {
	DepositToSubaccount(context.Context, DYDXSubaccountDepositRequest) (DYDXSubaccountDepositResult, error)
}

type DYDXRetainedCollateralStore interface {
	RetainedCollateralQuantums(context.Context, string, uint32) (string, error)
	SaveRetainedCollateralQuantums(context.Context, string, uint32, string) error
}

func NewTradingService(store TradingStore, ledger *LedgerService, submitter DYDXSubmitter) *TradingService {
	return &TradingService{
		store:     store,
		ledger:    ledger,
		submitter: submitter,
		dydxFunding: DYDXSubaccountFundingConfig{
			CollateralAssetID:         0,
			CollateralQuantumsPerUSD:  "1000000",
			SubaccountNumber:          0,
			MinStatefulOrderMarginUSD: defaultDYDXMinStatefulOrderMarginUSD,
			RetainedCollateralUSD:     noDYDXRetainedCollateralUSD,
		},
	}
}

func (s *TradingService) SetMarginQuoter(quoter TradingMarginQuoter) {
	s.marginQuoter = quoter
}

func (s *TradingService) SetFundingPreparer(preparer TradingFundingPreparer) {
	s.fundingPreparer = preparer
}

func (s *TradingService) SetDYDXAccountResolver(resolver TradingDYDXAccountResolver) {
	s.accountResolver = resolver
}

func (s *TradingService) SetPositionProvider(provider TradingPositionProvider) {
	s.positionProvider = provider
}

func (s *TradingService) SetDYDXPositionReconcileSource(source DYDXOrderReconcileSource) {
	s.positionReconcile = source
}

func (s *TradingService) SetMarketPriceSource(source TradingMarketPriceSource) {
	s.marketPrices = source
}

func (s *TradingService) SetMarketOrderSlippageBps(slippageBps uint32) {
	s.marketSlippage = slippageBps
}

func (s *TradingService) SetDYDXMarkets(markets []DYDXMarketConfig) {
	s.dydxMarkets = map[string]DYDXMarketConfig{}
	for _, market := range markets {
		s.dydxMarkets[normalizeDYDXMarket(market.Market)] = market
	}
}

func (s *TradingService) SetDYDXSubaccountDepositor(depositor DYDXSubaccountDepositor) {
	s.dydxDepositor = depositor
}

func (s *TradingService) SetDYDXRetainedCollateralStore(store DYDXRetainedCollateralStore) {
	s.retainedCollateral = store
}

func (s *TradingService) SetFailedOrderSettlementService(service TradingFailedOrderSettlementService) {
	s.failedSettlement = service
}

func (s *TradingService) SetDYDXSubaccountFundingConfig(conf DYDXSubaccountFundingConfig) {
	if conf.CollateralQuantumsPerUSD == "" {
		conf.CollateralQuantumsPerUSD = "1000000"
	}
	if conf.MinStatefulOrderMarginUSD == "" {
		conf.MinStatefulOrderMarginUSD = defaultDYDXMinStatefulOrderMarginUSD
	}
	if conf.RetainedCollateralUSD == "" {
		conf.RetainedCollateralUSD = noDYDXRetainedCollateralUSD
	}
	s.dydxFunding = conf
}

func (s *TradingService) subaccountNumberForMarket(market string) uint32 {
	if s == nil {
		return defaultDYDXTradingSubaccountNumber
	}
	return builtInDYDXSubaccountNumberForMarket(
		market,
		clobPairIDForMarket(s.dydxMarkets, market),
		s.dydxFunding.SubaccountNumber,
	)
}

func (s *TradingService) SubaccountNumberForMarket(market string) uint32 {
	return s.subaccountNumberForMarket(market)
}

func (s *TradingService) ensureOrderSubaccountNumber(order TradingOrder) TradingOrder {
	fallback := defaultDYDXTradingSubaccountNumber
	var markets map[string]DYDXMarketConfig
	if s != nil {
		fallback = s.dydxFunding.SubaccountNumber
		markets = s.dydxMarkets
	}
	order.SubaccountNumber = explicitOrBuiltInDYDXSubaccountNumber(
		order.SubaccountNumber,
		order.Market,
		clobPairIDForMarket(markets, order.Market),
		fallback,
	)
	return order
}

func (s *TradingService) QuoteMargin(ctx context.Context, request TradingMarginQuoteRequest) (TradingMarginQuote, error) {
	if s.marginQuoter == nil {
		return TradingMarginQuote{}, ErrNECOPriceOracleNotConfigured
	}
	return s.marginQuoter.QuoteMargin(ctx, request)
}

func (s *TradingService) PlaceOrder(ctx context.Context, request TradingPlaceOrderRequest) (TradingOrder, error) {
	if order, ok, err := s.store.GetOrderByClientID(ctx, request.UserID, request.ClientOrderID); err != nil {
		return TradingOrder{}, err
	} else if ok {
		order = s.ensureOrderSubaccountNumber(order)
		if order.Status == TradingOrderStatusFundingRequired {
			return s.resumeFundingRequiredOrder(ctx, order)
		}
		if order.Status == TradingOrderStatusDYDXFundingFailed {
			order, err = s.ensureMinimumOpenOrderMargin(ctx, order)
			if err != nil {
				return TradingOrder{}, err
			}
			return s.fundSaveAndSubmit(ctx, order)
		}
		if order.Status == TradingOrderStatusSubmitFailed {
			return s.submitOrder(ctx, order)
		}
		return order, nil
	}
	if order, handled, err := s.placeOrderAgainstExistingPosition(ctx, request); err != nil || handled {
		return order, err
	}
	var err error
	request, err = s.prepareOrderRequest(ctx, request)
	if err != nil {
		return TradingOrder{}, err
	}

	marginAsset := request.MarginAsset
	marginAmount := request.MarginAmount
	marginUSD := ""
	priceSnapshotID := ""
	dedxUSDPrice := ""
	assetUSDPrice := ""
	priceRoute := ""
	if s.marginQuoter != nil {
		quote, err := s.marginQuoter.QuoteMargin(ctx, TradingMarginQuoteRequest{
			UserID:                   request.UserID,
			ClientOrderID:            request.ClientOrderID,
			Market:                   request.Market,
			MarginAsset:              request.MarginAsset,
			Price:                    request.Price,
			Size:                     request.Size,
			MinMarginUSD:             s.minimumOpenOrderMarginUSD(),
			CollateralQuantumsPerUSD: s.openOrderCollateralQuantumsPerUSD(),
		})
		if err != nil {
			return TradingOrder{}, err
		}
		marginAsset = quote.ChargeAsset
		marginAmount = quote.ChargeAmount
		marginUSD = quote.MarginUSD
		priceSnapshotID = quote.PriceSnapshotID
		dedxUSDPrice = quote.DEDXUSDPrice
		assetUSDPrice = quote.AssetUSDPrice
		priceRoute = quote.PriceRoute
	}
	dydxAccount, err := s.resolveDYDXAccount(ctx, request.UserID, "")
	if err != nil {
		return TradingOrder{}, err
	}

	order := TradingOrder{
		ID:                    uuid.NewString(),
		UserID:                request.UserID,
		NECOAddress:           request.NECOAddress,
		DYDXAccount:           dydxAccount,
		SubaccountNumber:      s.subaccountNumberForMarket(request.Market),
		ClientOrderID:         request.ClientOrderID,
		Market:                request.Market,
		Side:                  request.Side,
		OrderType:             request.OrderType,
		ExecutionOrderType:    request.ExecutionOrderType,
		Price:                 request.Price,
		ReferencePrice:        request.ReferencePrice,
		SlippageBps:           request.SlippageBps,
		MarketPriceSource:     request.MarketPriceSource,
		MarketPriceSnapshotID: request.MarketPriceSnapshotID,
		Size:                  request.Size,
		MarginAsset:           marginAsset,
		MarginAmount:          marginAmount,
		MarginUSD:             marginUSD,
		PriceSnapshotID:       priceSnapshotID,
		DEDXUSDPrice:          dedxUSDPrice,
		AssetUSDPrice:         assetUSDPrice,
		PriceRoute:            priceRoute,
		Status:                TradingOrderStatusPendingSubmit,
	}
	createdOrder, created, err := s.createOrderIfAbsent(ctx, order)
	if err != nil {
		return TradingOrder{}, err
	}
	if !created {
		createdOrder = s.ensureOrderSubaccountNumber(createdOrder)
		if createdOrder.Status == TradingOrderStatusFundingRequired {
			return s.resumeFundingRequiredOrder(ctx, createdOrder)
		}
		if createdOrder.Status == TradingOrderStatusDYDXFundingFailed {
			return s.fundSaveAndSubmit(ctx, createdOrder)
		}
		if createdOrder.Status == TradingOrderStatusSubmitFailed {
			return s.submitOrder(ctx, createdOrder)
		}
		return createdOrder, nil
	}
	order = createdOrder
	if s.fundingPreparer != nil {
		return s.prepareFullFunding(ctx, order)
	}
	return s.chargeSaveAndSubmit(ctx, order)
}

func (s *TradingService) placeOrderAgainstExistingPosition(ctx context.Context, request TradingPlaceOrderRequest) (TradingOrder, bool, error) {
	if s == nil || s.positionProvider == nil {
		return TradingOrder{}, false, nil
	}
	subaccountNumber := s.subaccountNumberForMarket(request.Market)
	position, err := s.positionForSubaccount(ctx, request.UserID, request.Market, subaccountNumber)
	if err != nil {
		if errors.Is(err, ErrTradingSyncEventNotFound) || errors.Is(err, ErrTradingPositionNotFound) {
			return TradingOrder{}, false, nil
		}
		return TradingOrder{}, false, err
	}
	var cleared bool
	position, cleared, err = s.reconcileCachedPosition(ctx, request, subaccountNumber, position)
	if err != nil {
		return TradingOrder{}, false, err
	}
	if cleared {
		return TradingOrder{}, false, nil
	}
	closeSide, positionSize, err := closeOrderSideAndSize(position.Size, "")
	if err != nil {
		if errors.Is(err, ErrTradingPositionNotFound) {
			return TradingOrder{}, false, nil
		}
		return TradingOrder{}, false, err
	}
	if !strings.EqualFold(strings.TrimSpace(request.Side), closeSide) {
		return TradingOrder{}, false, nil
	}
	requestedSize, ok := parseDecimalRat(request.Size)
	if !ok || requestedSize.Sign() <= 0 {
		return TradingOrder{}, false, ErrTradingPositionSizeInvalid
	}
	maxCloseSize, ok := parseDecimalRat(positionSize)
	if !ok {
		return TradingOrder{}, false, ErrTradingPositionSizeInvalid
	}
	if requestedSize.Cmp(maxCloseSize) > 0 {
		order, err := s.saveCrossesPositionOrder(ctx, request, subaccountNumber)
		return order, true, err
	}
	order, err := s.ClosePosition(ctx, TradingClosePositionRequest{
		UserID:                request.UserID,
		NECOAddress:           request.NECOAddress,
		ClientOrderID:         request.ClientOrderID,
		Market:                request.Market,
		OrderType:             request.OrderType,
		ExecutionOrderType:    request.ExecutionOrderType,
		Price:                 request.Price,
		ReferencePrice:        request.ReferencePrice,
		SlippageBps:           request.SlippageBps,
		MarketPriceSource:     request.MarketPriceSource,
		MarketPriceSnapshotID: request.MarketPriceSnapshotID,
		Size:                  request.Size,
	})
	return order, true, err
}

func (s *TradingService) saveCrossesPositionOrder(ctx context.Context, request TradingPlaceOrderRequest, subaccountNumber uint32) (TradingOrder, error) {
	dydxAccount, err := s.resolveDYDXAccount(ctx, request.UserID, "")
	if err != nil {
		return TradingOrder{}, err
	}
	order := TradingOrder{
		ID:               uuid.NewString(),
		UserID:           request.UserID,
		NECOAddress:      request.NECOAddress,
		DYDXAccount:      dydxAccount,
		SubaccountNumber: subaccountNumber,
		ClientOrderID:    request.ClientOrderID,
		Market:           request.Market,
		Side:             request.Side,
		OrderType:        request.OrderType,
		Price:            request.Price,
		Size:             request.Size,
		MarginAsset:      request.MarginAsset,
		MarginAmount:     request.MarginAmount,
		Status:           TradingOrderStatusCrossesPosition,
		Reason:           TradingOrderCrossesPositionReason,
	}
	createdOrder, _, err := s.createOrderIfAbsent(ctx, order)
	if err != nil {
		return TradingOrder{}, err
	}
	return s.ensureOrderSubaccountNumber(createdOrder), nil
}

type tradingPositionSyncer interface {
	SyncPosition(context.Context, TradingPositionSyncRequest) (TradingPosition, error)
}

func (s *TradingService) reconcileCachedPosition(ctx context.Context, request TradingPlaceOrderRequest, subaccountNumber uint32, cached TradingPosition) (TradingPosition, bool, error) {
	if s == nil || s.positionReconcile == nil {
		return cached, false, nil
	}
	market, ok := s.dydxMarkets[normalizeDYDXMarket(request.Market)]
	if !ok {
		return cached, false, nil
	}
	dydxAccount, err := s.resolveDYDXAccount(ctx, request.UserID, "")
	if err != nil {
		return TradingPosition{}, false, err
	}
	result, err := s.positionReconcile.ReconcileOrder(ctx, DYDXOrderReconcileRequest{
		UserID:           request.UserID,
		DYDXAccount:      dydxAccount,
		Market:           request.Market,
		SubaccountNumber: subaccountNumber,
		ClobPairID:       market.ClobPairID,
		OrderFlags:       DYDXOrderFlagsLongTerm,
	})
	if err != nil || !result.Subaccount.Found {
		return cached, false, nil
	}
	if position, ok := marketPositionFromReconcile(market, result.Subaccount.PerpetualPositions); ok {
		cached.Size = position.Quantums
		if size, converted := quantumsToUnitString(position.Quantums, market.QuantumsPerUnit); converted {
			cached.Size = size
		}
		cached.SubaccountNumber = subaccountNumber
		_ = s.saveReconciledCachedPosition(ctx, request, cached, "position")
		return cached, false, nil
	}
	if err := s.clearReconciledCachedPosition(ctx, request, cached, subaccountNumber); err != nil {
		return TradingPosition{}, false, err
	}
	return cached, true, nil
}

func (s *TradingService) saveReconciledCachedPosition(ctx context.Context, request TradingPlaceOrderRequest, position TradingPosition, prefix string) error {
	syncer, ok := s.positionProvider.(tradingPositionSyncer)
	if !ok {
		return nil
	}
	_, err := syncer.SyncPosition(ctx, TradingPositionSyncRequest{
		EventID:          tradingPositionReconcileEventID(prefix, request),
		UserID:           position.UserID,
		Market:           position.Market,
		SubaccountNumber: position.SubaccountNumber,
		Size:             position.Size,
		EntryPrice:       position.EntryPrice,
		MarkPrice:        position.MarkPrice,
		UnrealizedPNL:    position.UnrealizedPNL,
		MarginAsset:      position.MarginAsset,
		Margin:           position.Margin,
	})
	if errors.Is(err, ErrTradingSyncDuplicateEvent) {
		return nil
	}
	return err
}

func (s *TradingService) clearReconciledCachedPosition(ctx context.Context, request TradingPlaceOrderRequest, cached TradingPosition, subaccountNumber uint32) error {
	cached.Size = "0"
	cached.Margin = "0"
	cached.SubaccountNumber = subaccountNumber
	return s.saveReconciledCachedPosition(ctx, request, cached, "position-cleared")
}

func tradingPositionReconcileEventID(prefix string, request TradingPlaceOrderRequest) string {
	return "dydx-position-reconcile:" + prefix + ":" + request.UserID + ":" + normalizeDYDXMarket(request.Market) + ":" + request.ClientOrderID
}

func (s *TradingService) resumeFundingRequiredOrder(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	var err error
	order, err = s.ensureMinimumOpenOrderMargin(ctx, order)
	if err != nil {
		return TradingOrder{}, err
	}
	ledgerReady, err := s.fundingLedgerCoversMargin(ctx, order)
	if err != nil {
		return TradingOrder{}, err
	}
	if ledgerReady {
		return s.chargeSaveAndSubmit(ctx, order)
	}
	if s.fundingPreparer == nil {
		return s.chargeSaveAndSubmit(ctx, order)
	}
	return s.refreshFundingRequiredOrderFromLedger(ctx, order)
}

func (s *TradingService) refreshFundingRequiredOrderFromLedger(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	balance, err := s.ledger.GetBalance(ctx, order.UserID, order.MarginAsset)
	if err != nil {
		return TradingOrder{}, err
	}
	missing, err := missingLedgerAmount(order.MarginAmount, balance.Available)
	if err != nil {
		return TradingOrder{}, err
	}
	if missing == "0" {
		return s.chargeSaveAndSubmit(ctx, order)
	}
	order.Status = TradingOrderStatusFundingRequired
	order.FundingStatus = defaultString(order.FundingStatus, TradingFundingStatusWalletActionRequired)
	order.FundingAction = defaultString(order.FundingAction, TradingFundingActionTransfer)
	order.FundingAsset = defaultString(order.FundingAsset, order.MarginAsset)
	order.FundingAmount = missing
	order.FundingLedgerAvailable = balance.Available
	if err := s.store.SaveOrder(ctx, order); err != nil {
		return TradingOrder{}, err
	}
	return order, nil
}

func (s *TradingService) fundingLedgerCoversMargin(ctx context.Context, order TradingOrder) (bool, error) {
	balance, err := s.ledger.GetBalance(ctx, order.UserID, order.MarginAsset)
	if err != nil {
		return false, err
	}
	available, ok := parseLedgerAmount(balance.Available)
	if !ok {
		return false, errors.New("invalid available balance")
	}
	required, ok := parseLedgerAmount(order.MarginAmount)
	if !ok {
		return false, ErrTradingQuoteInvalidAmount
	}
	return available.Cmp(required) >= 0, nil
}

func (s *TradingService) minimumOpenOrderMarginUSD() string {
	if s == nil || s.dydxDepositor == nil {
		return ""
	}
	return s.dydxFunding.MinStatefulOrderMarginUSD
}

func (s *TradingService) openOrderCollateralQuantumsPerUSD() string {
	if s == nil || s.dydxDepositor == nil {
		return ""
	}
	return defaultString(s.dydxFunding.CollateralQuantumsPerUSD, "1000000")
}

func (s *TradingService) ensureMinimumOpenOrderMargin(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	minimum := s.minimumOpenOrderMarginUSD()
	collateralQuantumsPerUSD := s.openOrderCollateralQuantumsPerUSD()
	if order.ClosePosition || order.DYDXFundingTxHash != "" || s.marginQuoter == nil || (strings.TrimSpace(minimum) == "" && strings.TrimSpace(collateralQuantumsPerUSD) == "") {
		return order, nil
	}
	current, ok := parseDecimalRat(order.MarginUSD)
	minimumValue, minOK := parseDecimalRat(minimum)
	needsRequote := !ok
	if minOK && ok && current.Cmp(minimumValue) < 0 {
		needsRequote = true
	}
	if ok && strings.TrimSpace(collateralQuantumsPerUSD) != "" {
		aligned, err := marginUSDAlignedToCollateralQuantum(current, collateralQuantumsPerUSD)
		if err != nil {
			return TradingOrder{}, err
		}
		if !aligned {
			needsRequote = true
		}
	}
	if !needsRequote {
		return order, nil
	}
	quote, err := s.marginQuoter.QuoteMargin(ctx, TradingMarginQuoteRequest{
		UserID:                   order.UserID,
		ClientOrderID:            order.ClientOrderID,
		Market:                   order.Market,
		MarginAsset:              order.MarginAsset,
		Price:                    order.Price,
		Size:                     order.Size,
		MinMarginUSD:             minimum,
		CollateralQuantumsPerUSD: collateralQuantumsPerUSD,
	})
	if err != nil {
		return TradingOrder{}, err
	}
	order.MarginAsset = quote.ChargeAsset
	order.MarginAmount = quote.ChargeAmount
	order.MarginUSD = quote.MarginUSD
	order.PriceSnapshotID = quote.PriceSnapshotID
	order.DEDXUSDPrice = quote.DEDXUSDPrice
	order.AssetUSDPrice = quote.AssetUSDPrice
	order.PriceRoute = quote.PriceRoute
	return order, nil
}

func (s *TradingService) ClosePosition(ctx context.Context, request TradingClosePositionRequest) (TradingOrder, error) {
	if order, ok, err := s.store.GetOrderByClientID(ctx, request.UserID, request.ClientOrderID); err != nil {
		return TradingOrder{}, err
	} else if ok {
		order = s.ensureOrderSubaccountNumber(order)
		if order.Status == TradingOrderStatusClosePending || order.Status == TradingOrderStatusCloseFailed {
			return s.submitOrder(ctx, order)
		}
		return order, nil
	}
	if s.positionProvider == nil {
		return TradingOrder{}, ErrTradingPositionNotFound
	}
	subaccountNumber := s.subaccountNumberForMarket(request.Market)
	position, err := s.positionForSubaccount(ctx, request.UserID, request.Market, subaccountNumber)
	if err != nil {
		if errors.Is(err, ErrTradingSyncEventNotFound) {
			return TradingOrder{}, ErrTradingPositionNotFound
		}
		return TradingOrder{}, err
	}
	position, cleared, err := s.reconcileCachedPosition(ctx, TradingPlaceOrderRequest{
		UserID:        request.UserID,
		ClientOrderID: request.ClientOrderID,
		Market:        request.Market,
	}, subaccountNumber, position)
	if err != nil {
		return TradingOrder{}, err
	}
	if cleared {
		return TradingOrder{}, ErrTradingPositionNotFound
	}
	side, size, err := closeOrderSideAndSize(position.Size, request.Size)
	if err != nil {
		return TradingOrder{}, err
	}
	orderRequest := TradingPlaceOrderRequest{
		UserID:                request.UserID,
		ClientOrderID:         request.ClientOrderID,
		Market:                request.Market,
		Side:                  side,
		OrderType:             defaultString(request.OrderType, TradingOrderTypeLimit),
		ExecutionOrderType:    request.ExecutionOrderType,
		Price:                 request.Price,
		ReferencePrice:        request.ReferencePrice,
		SlippageBps:           request.SlippageBps,
		MarketPriceSource:     request.MarketPriceSource,
		MarketPriceSnapshotID: request.MarketPriceSnapshotID,
		Size:                  size,
		MarginAsset:           position.MarginAsset,
		MarginAmount:          "0",
	}
	orderRequest, err = s.prepareOrderRequest(ctx, orderRequest)
	if err != nil {
		return TradingOrder{}, err
	}
	orderRequest.ExecutionOrderType = TradingOrderTypeIOC
	dydxAccount, err := s.resolveDYDXAccount(ctx, request.UserID, "")
	if err != nil {
		return TradingOrder{}, err
	}
	order := TradingOrder{
		ID:                    uuid.NewString(),
		UserID:                request.UserID,
		NECOAddress:           request.NECOAddress,
		DYDXAccount:           dydxAccount,
		SubaccountNumber:      subaccountNumber,
		ClientOrderID:         request.ClientOrderID,
		Market:                request.Market,
		Side:                  orderRequest.Side,
		OrderType:             orderRequest.OrderType,
		ExecutionOrderType:    orderRequest.ExecutionOrderType,
		Price:                 orderRequest.Price,
		ReferencePrice:        orderRequest.ReferencePrice,
		SlippageBps:           orderRequest.SlippageBps,
		MarketPriceSource:     orderRequest.MarketPriceSource,
		MarketPriceSnapshotID: orderRequest.MarketPriceSnapshotID,
		Size:                  orderRequest.Size,
		MarginAsset:           position.MarginAsset,
		MarginAmount:          "0",
		MarginUSD:             closePositionMarginUSD(position.Margin),
		ReduceOnly:            true,
		ClosePosition:         true,
		Status:                TradingOrderStatusClosePending,
	}
	createdOrder, created, err := s.createOrderIfAbsent(ctx, order)
	if err != nil {
		return TradingOrder{}, err
	}
	if !created {
		createdOrder = s.ensureOrderSubaccountNumber(createdOrder)
		if createdOrder.Status == TradingOrderStatusClosePending || createdOrder.Status == TradingOrderStatusCloseFailed {
			return s.submitOrder(ctx, createdOrder)
		}
		return createdOrder, nil
	}
	return s.submitOrder(ctx, createdOrder)
}

func (s *TradingService) positionForSubaccount(ctx context.Context, userID, market string, subaccountNumber uint32) (TradingPosition, error) {
	if provider, ok := s.positionProvider.(TradingSubaccountPositionProvider); ok {
		return provider.GetPositionForSubaccount(ctx, userID, market, subaccountNumber)
	}
	return s.positionProvider.GetPosition(ctx, userID, market)
}

func closePositionMarginUSD(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || looksLikeAtomicUSD(value) {
		return ""
	}
	return value
}

func (s *TradingService) chargeSaveAndSubmit(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	order = s.ensureOrderSubaccountNumber(order)
	dydxAccount, err := s.resolveDYDXAccount(ctx, order.UserID, order.DYDXAccount)
	if err != nil {
		return TradingOrder{}, err
	}
	order.DYDXAccount = dydxAccount
	order.Status = TradingOrderStatusPendingSubmit
	if _, err := s.ledger.ChargeAvailable(ctx, LedgerChargeRequest{
		UserID: order.UserID,
		Asset:  order.MarginAsset,
		Amount: order.MarginAmount,
		RefID:  orderRef(order),
	}); err != nil {
		if errors.Is(err, ErrLedgerInsufficientBalance) && s.fundingPreparer != nil {
			return s.prepareFunding(ctx, order)
		}
		order.Status = TradingOrderStatusRejected
		_ = s.store.SaveOrder(ctx, order)
		return TradingOrder{}, err
	}
	if err := s.store.SaveOrder(ctx, order); err != nil {
		_, _ = s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
			UserID: order.UserID,
			Asset:  order.MarginAsset,
			Amount: order.MarginAmount,
			RefID:  orderRef(order) + ":save-failed-refund",
		})
		return TradingOrder{}, err
	}
	return s.fundSaveAndSubmit(ctx, order)
}

func (s *TradingService) fundSaveAndSubmit(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	var err error
	var funded bool
	var attempted bool
	order = s.ensureOrderSubaccountNumber(order)
	order.Status = TradingOrderStatusPendingSubmit
	order, funded, attempted, err = s.fundDYDXSubaccount(ctx, order)
	if err != nil {
		order.Status = TradingOrderStatusRejected
		if attempted {
			order.Status = TradingOrderStatusDYDXFundingFailed
		}
		_ = s.store.SaveOrder(ctx, order)
		if !attempted {
			_, _ = s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
				UserID: order.UserID,
				Asset:  order.MarginAsset,
				Amount: order.MarginAmount,
				RefID:  orderRef(order) + ":dydx-funding-failed-refund",
			})
		}
		return TradingOrder{}, err
	}
	if funded {
		if err := s.store.SaveOrder(ctx, order); err != nil {
			return TradingOrder{}, err
		}
	}
	return s.submitOrder(ctx, order)
}

func (s *TradingService) fundDYDXSubaccount(ctx context.Context, order TradingOrder) (TradingOrder, bool, bool, error) {
	order = s.ensureOrderSubaccountNumber(order)
	if s.dydxDepositor == nil || order.DYDXFundingTxHash != "" {
		return order, false, false, nil
	}
	marginUSD := firstNonEmpty(order.MarginUSD, order.MarginAmount)
	quantumsPerUSD := s.dydxFunding.CollateralQuantumsPerUSD
	if quantumsPerUSD == "" {
		quantumsPerUSD = "1000000"
	}
	quantums, err := parseScaledUint64(marginUSD, quantumsPerUSD)
	if err != nil {
		return order, false, false, err
	}
	retainedTarget, retainedTopUp, err := s.retainedCollateralTopUp(ctx, order, quantumsPerUSD)
	if err != nil {
		return order, false, false, err
	}
	totalQuantums, err := addUint64Checked(quantums, retainedTopUp)
	if err != nil {
		return order, false, false, err
	}
	req := DYDXSubaccountDepositRequest{
		ID:               order.ID + "-dydx-funding",
		UserID:           order.UserID,
		DYDXAccount:      order.DYDXAccount,
		SubaccountNumber: order.SubaccountNumber,
		AssetID:          s.dydxFunding.CollateralAssetID,
		Quantums:         strconv.FormatUint(totalQuantums, 10),
		MarginUSD:        marginUSD,
	}
	result, err := s.dydxDepositor.DepositToSubaccount(ctx, req)
	if err != nil {
		return order, false, true, err
	}
	order.DYDXFundingTxHash = result.TxHash
	order.DYDXFundingStatus = defaultString(result.Status, TradingOrderStatusBroadcasted)
	order.DYDXFundingAssetID = req.AssetID
	order.DYDXOrderMarginQuantums = strconv.FormatUint(quantums, 10)
	if retainedTopUp > 0 {
		order.DYDXRetainedCollateralQuantums = strconv.FormatUint(retainedTopUp, 10)
	}
	order.DYDXFundingQuantums = req.Quantums
	if retainedTarget > 0 && s.retainedCollateral != nil {
		_ = s.retainedCollateral.SaveRetainedCollateralQuantums(ctx, order.DYDXAccount, order.SubaccountNumber, strconv.FormatUint(retainedTarget, 10))
	}
	return order, true, true, nil
}

func (s *TradingService) retainedCollateralTopUp(ctx context.Context, order TradingOrder, quantumsPerUSD string) (uint64, uint64, error) {
	if s == nil || order.ClosePosition {
		return 0, 0, nil
	}
	targetUSD := strings.TrimSpace(s.dydxFunding.RetainedCollateralUSD)
	targetValue, ok := parseDecimalRat(targetUSD)
	if !ok || targetValue.Sign() < 0 {
		return 0, 0, errors.New("invalid retained dydx collateral amount")
	}
	if targetValue.Sign() == 0 {
		return 0, 0, nil
	}
	target, err := parseScaledUint64(targetUSD, quantumsPerUSD)
	if err != nil {
		return 0, 0, err
	}
	if s.retainedCollateral == nil {
		return target, target, nil
	}
	currentValue, err := s.retainedCollateral.RetainedCollateralQuantums(ctx, order.DYDXAccount, order.SubaccountNumber)
	if err != nil {
		return 0, 0, err
	}
	current, ok := new(big.Int).SetString(strings.TrimSpace(defaultString(currentValue, "0")), 10)
	if !ok || current.Sign() < 0 {
		return 0, 0, errors.New("invalid retained dydx collateral quantums")
	}
	targetBig := new(big.Int).SetUint64(target)
	if current.Cmp(targetBig) >= 0 {
		return target, 0, nil
	}
	topUp := new(big.Int).Sub(targetBig, current)
	if !topUp.IsUint64() {
		return 0, 0, errors.New("retained dydx collateral top-up overflows uint64")
	}
	return target, topUp.Uint64(), nil
}

func addUint64Checked(left, right uint64) (uint64, error) {
	if ^uint64(0)-left < right {
		return 0, errors.New("dydx subaccount funding quantums overflow")
	}
	return left + right, nil
}

func (s *TradingService) prepareFunding(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	order = s.ensureOrderSubaccountNumber(order)
	balance, err := s.ledger.GetBalance(ctx, order.UserID, order.MarginAsset)
	if err != nil {
		return TradingOrder{}, err
	}
	missing, err := missingLedgerAmount(order.MarginAmount, balance.Available)
	if err != nil {
		return TradingOrder{}, err
	}
	return s.prepareFundingAmount(ctx, order, balance.Available, missing)
}

func (s *TradingService) prepareFullFunding(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	order = s.ensureOrderSubaccountNumber(order)
	balance, err := s.ledger.GetBalance(ctx, order.UserID, order.MarginAsset)
	if err != nil {
		return TradingOrder{}, err
	}
	return s.prepareFundingAmount(ctx, order, balance.Available, order.MarginAmount)
}

func (s *TradingService) prepareFundingAmount(ctx context.Context, order TradingOrder, ledgerAvailable, fundingAmount string) (TradingOrder, error) {
	if _, ok := parseLedgerAmount(fundingAmount); !ok {
		return TradingOrder{}, ErrTradingQuoteInvalidAmount
	}
	result, err := s.fundingPreparer.PrepareFunding(ctx, TradingFundingPrepareRequest{
		UserID:          order.UserID,
		NECOAddress:     order.NECOAddress,
		ClientOrderID:   order.ClientOrderID,
		Market:          order.Market,
		ChargeAsset:     order.MarginAsset,
		ChargeAmount:    order.MarginAmount,
		LedgerAvailable: ledgerAvailable,
		MissingAmount:   fundingAmount,
		RefID:           orderRef(order),
	})
	if err != nil {
		return TradingOrder{}, err
	}
	order.Status = TradingOrderStatusFundingRequired
	order.FundingStatus = defaultString(result.Status, TradingFundingStatusWalletActionRequired)
	order.FundingAction = result.Action
	order.FundingAsset = defaultString(result.Asset, order.MarginAsset)
	order.FundingAmount = defaultString(result.Amount, fundingAmount)
	order.FundingTokenAddress = result.TokenAddress
	order.FundingFromAddress = result.FromAddress
	order.FundingToAddress = result.ToAddress
	order.FundingSpenderAddress = result.SpenderAddress
	order.FundingReason = result.Reason
	order.FundingWalletBalance = result.WalletBalance
	order.FundingLedgerAvailable = defaultString(result.LedgerAvailable, ledgerAvailable)
	if err := s.store.SaveOrder(ctx, order); err != nil {
		return TradingOrder{}, err
	}
	return order, nil
}

func (s *TradingService) submitOrder(ctx context.Context, order TradingOrder) (TradingOrder, error) {
	order = s.ensureOrderSubaccountNumber(order)
	orderType := defaultString(order.ExecutionOrderType, order.OrderType)
	result, err := s.submit(ctx, DYDXIntent{
		ID:     order.ID,
		Action: DYDXActionPlaceOrder,
		Params: map[string]interface{}{
			"userId":                order.UserID,
			"dydxAccount":           order.DYDXAccount,
			"subaccountNumber":      order.SubaccountNumber,
			"clientOrderId":         order.ClientOrderID,
			"market":                order.Market,
			"side":                  order.Side,
			"orderType":             orderType,
			"price":                 order.Price,
			"referencePrice":        order.ReferencePrice,
			"slippageBps":           order.SlippageBps,
			"marketPriceSource":     order.MarketPriceSource,
			"marketPriceSnapshotId": order.MarketPriceSnapshotID,
			"size":                  order.Size,
			"marginAsset":           order.MarginAsset,
			"marginAmount":          order.MarginAmount,
			"marginUsd":             order.MarginUSD,
			"priceSnapshotId":       order.PriceSnapshotID,
			"dedxUsdPrice":          order.DEDXUSDPrice,
			"assetUsdPrice":         order.AssetUSDPrice,
			"priceRoute":            order.PriceRoute,
			"reduceOnly":            order.ReduceOnly,
		},
	})
	order.DYDXOrderTxHash = result.TxHash
	if err != nil {
		if isDEDXUSDOpeningEquityTierLimit(order, err) {
			order.Status = TradingOrderStatusEquityTierLimit
			order.Reason = TradingOrderEquityTierLimitReason
			_ = s.store.SaveOrder(ctx, order)
			if order.DYDXFundingTxHash == "" {
				_, _ = s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
					UserID: order.UserID,
					Asset:  order.MarginAsset,
					Amount: order.MarginAmount,
					RefID:  orderRef(order) + ":equity-tier-limit-refund",
				})
			} else {
				s.settleFailedFundedOrder(ctx, order)
			}
			return order, nil
		}
		order.Status = TradingOrderStatusRejected
		if order.ClosePosition {
			order.Status = TradingOrderStatusCloseFailed
		}
		if order.DYDXFundingTxHash != "" && !order.ClosePosition {
			order.Status = TradingOrderStatusSubmitFailed
		}
		_ = s.store.SaveOrder(ctx, order)
		if order.DYDXFundingTxHash == "" && !order.ClosePosition {
			_, _ = s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
				UserID: order.UserID,
				Asset:  order.MarginAsset,
				Amount: order.MarginAmount,
				RefID:  orderRef(order) + ":submit-failed-refund",
			})
		} else {
			s.settleFailedFundedOrder(ctx, order)
		}
		return TradingOrder{}, err
	}
	if result.Status == TradingOrderStatusRejected {
		order.Status = orderStatusForDYDXResult(order, TradingOrderStatusRejected)
		_ = s.store.SaveOrder(ctx, order)
		if order.DYDXFundingTxHash == "" && !order.ClosePosition {
			_, _ = s.ledger.RefundAvailable(ctx, LedgerRefundRequest{
				UserID: order.UserID,
				Asset:  order.MarginAsset,
				Amount: order.MarginAmount,
				RefID:  orderRef(order) + ":submit-rejected-refund",
			})
		} else {
			s.settleFailedFundedOrder(ctx, order)
		}
		return TradingOrder{}, ErrDYDXOrderRejected
	}
	if result.Status != "" {
		order.Status = orderStatusForDYDXBroadcastResult(order, result.Status)
		if err := s.store.SaveOrder(ctx, order); err != nil {
			return TradingOrder{}, err
		}
	}
	return order, nil
}

func (s *TradingService) settleFailedFundedOrder(ctx context.Context, order TradingOrder) {
	if s == nil || s.failedSettlement == nil || order.ClosePosition || strings.TrimSpace(order.DYDXFundingTxHash) == "" {
		return
	}
	_, _ = s.failedSettlement.SettleFailedOrder(ctx, FailedOrderSettlementRequest{Order: order})
}

func isDEDXUSDOpeningEquityTierLimit(order TradingOrder, err error) bool {
	if err == nil || order.ClosePosition || order.ReduceOnly {
		return false
	}
	if normalizeDYDXMarket(order.Market) != builtInDEDXUSDMarket {
		return false
	}
	if order.SubaccountNumber != builtInDEDXUSDSubaccountNumber {
		return false
	}
	return isDYDXEquityTierLimitError(err)
}

func isDYDXEquityTierLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "code 10001") &&
		strings.Contains(message, "equity tier limit")
}

func (s *TradingService) prepareOrderRequest(ctx context.Context, request TradingPlaceOrderRequest) (TradingPlaceOrderRequest, error) {
	request.OrderType = normalizeTradingOrderType(request.OrderType)
	if request.OrderType != TradingOrderTypeMarket {
		return request, nil
	}
	if s.marketPrices == nil {
		return request, ErrTradingMarketPriceSourceNotConfigured
	}
	side, err := parseDYDXSide(request.Side)
	if err != nil {
		return request, err
	}
	slippageBps := request.SlippageBps
	if slippageBps == 0 {
		slippageBps = s.marketSlippage
	}
	if slippageBps == 0 {
		slippageBps = defaultMarketOrderSlippageBps
	}
	if slippageBps > maxMarketOrderSlippageBps {
		return request, ErrTradingMarketSlippageInvalid
	}
	snapshot, err := s.marketPrices.MarketPrice(ctx, TradingMarketPriceRequest{
		Market: request.Market,
		Side:   request.Side,
	})
	if err != nil {
		return request, err
	}
	referencePrice, ok := parseDecimalRat(snapshot.Price)
	if !ok || referencePrice.Sign() <= 0 {
		return request, ErrTradingMarketPriceInvalid
	}
	protectedPrice := applyMarketOrderSlippage(referencePrice, side, slippageBps)
	if protectedPrice.Sign() <= 0 {
		return request, ErrTradingMarketPriceInvalid
	}
	protectedPrice, err = s.alignMarketOrderProtectedPrice(request.Market, side, protectedPrice)
	if err != nil {
		return request, err
	}
	request.ExecutionOrderType = TradingOrderTypeIOC
	request.Price = ratToDecimalString(protectedPrice, 18)
	request.ReferencePrice = ratToDecimalString(referencePrice, 18)
	request.SlippageBps = slippageBps
	request.MarketPriceSource = snapshot.Source
	request.MarketPriceSnapshotID = snapshot.SnapshotID
	return request, nil
}

func normalizeTradingOrderType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "market":
		return TradingOrderTypeMarket
	case "ioc":
		return TradingOrderTypeIOC
	case "post_only", "post-only", "postonly":
		return TradingOrderTypePostOnly
	case "limit":
		return TradingOrderTypeLimit
	default:
		return value
	}
}

func applyMarketOrderSlippage(referencePrice *big.Rat, side uint32, slippageBps uint32) *big.Rat {
	factorNumerator := int64(10000 + slippageBps)
	if side == DYDXOrderSideSell {
		factorNumerator = int64(10000 - slippageBps)
	}
	return new(big.Rat).Mul(referencePrice, big.NewRat(factorNumerator, 10000))
}

func (s *TradingService) alignMarketOrderProtectedPrice(marketName string, side uint32, price *big.Rat) (*big.Rat, error) {
	if price == nil || price.Sign() <= 0 || len(s.dydxMarkets) == 0 {
		return price, nil
	}
	market, ok := s.dydxMarkets[normalizeDYDXMarket(marketName)]
	if !ok {
		return price, nil
	}
	return alignDYDXPriceToTick(price, side, market)
}

func alignDYDXPriceToTick(price *big.Rat, side uint32, market DYDXMarketConfig) (*big.Rat, error) {
	if price == nil || price.Sign() <= 0 {
		return nil, ErrTradingMarketPriceInvalid
	}
	scale, ok := new(big.Rat).SetString(strings.TrimSpace(market.SubticksPerUnit))
	if !ok || scale.Sign() <= 0 {
		return nil, ErrInvalidDYDXAmount
	}
	subticksPerTick, err := market.subticksPerTick()
	if err != nil {
		return nil, err
	}
	rawSubticks := new(big.Rat).Mul(price, scale)
	var aligned *big.Int
	switch side {
	case DYDXOrderSideBuy:
		aligned = floorRatToInt(rawSubticks)
		aligned = roundUintDownToMultiple(aligned, subticksPerTick)
	case DYDXOrderSideSell:
		aligned = ceilRatToInt(rawSubticks)
		aligned = roundUintUpToMultiple(aligned, subticksPerTick)
	default:
		return nil, ErrInvalidDYDXOrderSide
	}
	if aligned.Sign() <= 0 {
		return nil, ErrTradingMarketPriceInvalid
	}
	numerator := new(big.Int).Mul(aligned, scale.Denom())
	return new(big.Rat).SetFrac(numerator, scale.Num()), nil
}

func floorRatToInt(value *big.Rat) *big.Int {
	if value == nil || value.Sign() <= 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Quo(new(big.Int).Set(value.Num()), new(big.Int).Set(value.Denom()))
}

func roundUintDownToMultiple(value *big.Int, multiple uint64) *big.Int {
	if value == nil || value.Sign() <= 0 || multiple == 0 {
		return big.NewInt(0)
	}
	out := new(big.Int).Set(value)
	mod := new(big.Int).Mod(out, new(big.Int).SetUint64(multiple))
	return out.Sub(out, mod)
}

func roundUintUpToMultiple(value *big.Int, multiple uint64) *big.Int {
	if value == nil || value.Sign() <= 0 || multiple == 0 {
		return big.NewInt(0)
	}
	out := new(big.Int).Set(value)
	mod := new(big.Int).Mod(out, new(big.Int).SetUint64(multiple))
	if mod.Sign() == 0 {
		return out
	}
	return out.Add(out, new(big.Int).Sub(new(big.Int).SetUint64(multiple), mod))
}

func closeOrderSideAndSize(positionSize, requestedSize string) (string, string, error) {
	position, ok := parseDecimalRat(positionSize)
	if !ok {
		return "", "", ErrTradingPositionSizeInvalid
	}
	if position.Sign() == 0 {
		return "", "", ErrTradingPositionNotFound
	}
	absPosition := new(big.Rat).Abs(position)
	size := absPosition
	if strings.TrimSpace(requestedSize) != "" {
		requested, ok := parseDecimalRat(requestedSize)
		if !ok || requested.Sign() <= 0 || requested.Cmp(absPosition) > 0 {
			return "", "", ErrTradingPositionSizeInvalid
		}
		size = requested
	}
	side := "sell"
	if position.Sign() < 0 {
		side = "buy"
	}
	return side, ratToDecimalString(size, 18), nil
}

func orderStatusForDYDXResult(order TradingOrder, status string) string {
	if !order.ClosePosition {
		return status
	}
	switch status {
	case "":
		return ""
	case TradingOrderStatusPendingSubmit:
		return TradingOrderStatusClosePending
	case TradingOrderStatusBroadcasted:
		return TradingOrderStatusCloseBroadcasted
	case TradingOrderStatusSubmitted:
		return TradingOrderStatusCloseSubmitted
	case TradingOrderStatusPartialFilled:
		return TradingOrderStatusClosePartialFilled
	case TradingOrderStatusFilled:
		return TradingOrderStatusClosed
	case TradingOrderStatusCancelled:
		return TradingOrderStatusCloseCancelled
	case TradingOrderStatusRejected, TradingOrderStatusSubmitFailed, TradingOrderStatusDYDXFundingFailed:
		return TradingOrderStatusCloseFailed
	default:
		return status
	}
}

func orderStatusForDYDXBroadcastResult(order TradingOrder, status string) string {
	switch status {
	case TradingOrderStatusPartialFilled,
		TradingOrderStatusFilled,
		TradingOrderStatusCancelled,
		TradingOrderStatusClosePartialFilled,
		TradingOrderStatusClosed,
		TradingOrderStatusCloseCancelled:
		return orderStatusForDYDXResult(order, TradingOrderStatusBroadcasted)
	default:
		return orderStatusForDYDXResult(order, status)
	}
}

func missingLedgerAmount(requiredValue, availableValue string) (string, error) {
	required, ok := parseLedgerAmount(requiredValue)
	if !ok {
		return "", errors.New("invalid required amount")
	}
	available, ok := parseLedgerAmount(availableValue)
	if !ok {
		return "", errors.New("invalid available amount")
	}
	if available.Cmp(required) >= 0 {
		return "0", nil
	}
	return new(big.Int).Sub(required, available).String(), nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (s *TradingService) CancelOrder(ctx context.Context, request TradingCancelOrderRequest) (TradingOrder, error) {
	order, ok, err := s.findOrder(ctx, request)
	if err != nil {
		return TradingOrder{}, err
	}
	if !ok {
		return TradingOrder{}, ErrTradingOrderNotFound
	}
	order = s.ensureOrderSubaccountNumber(order)
	order.Status = TradingOrderStatusCancelPending
	dydxAccount, err := s.resolveDYDXAccount(ctx, order.UserID, order.DYDXAccount)
	if err != nil {
		return TradingOrder{}, err
	}
	order.DYDXAccount = dydxAccount
	if err := s.store.SaveOrder(ctx, order); err != nil {
		return TradingOrder{}, err
	}
	result, err := s.submit(ctx, DYDXIntent{
		ID:     order.ID + "-cancel",
		Action: DYDXActionCancelOrder,
		Params: map[string]interface{}{
			"userId":           order.UserID,
			"dydxAccount":      order.DYDXAccount,
			"subaccountNumber": order.SubaccountNumber,
			"orderId":          order.ID,
			"clientOrderId":    order.ClientOrderID,
			"market":           order.Market,
		},
	})
	if err != nil {
		return TradingOrder{}, err
	}
	order.DYDXCancelTxHash = result.TxHash
	if err := s.store.SaveOrder(ctx, order); err != nil {
		return TradingOrder{}, err
	}
	return order, nil
}

func (s *TradingService) submit(ctx context.Context, intent DYDXIntent) (DYDXTradingResult, error) {
	if submitter, ok := s.submitter.(DYDXResultSubmitter); ok {
		return submitter.SubmitWithResult(ctx, intent)
	}
	if err := s.submitter.Submit(ctx, intent); err != nil {
		return DYDXTradingResult{}, err
	}
	return DYDXTradingResult{}, nil
}

func (s *TradingService) resolveDYDXAccount(ctx context.Context, userID, existing string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	if s.accountResolver == nil {
		return "", nil
	}
	account, err := s.accountResolver.DYDXAccountForUser(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrWalletBindingNotFound) {
			return "", ErrTradingDYDXBindingRequired
		}
		return "", err
	}
	if account == "" {
		return "", ErrTradingDYDXBindingRequired
	}
	return account, nil
}

func (s *TradingService) createOrderIfAbsent(ctx context.Context, order TradingOrder) (TradingOrder, bool, error) {
	creator, ok := s.store.(TradingOrderCreateStore)
	if !ok {
		return order, true, nil
	}
	return creator.CreateOrderIfAbsent(ctx, order)
}

func (s *TradingService) GetOrder(ctx context.Context, request TradingCancelOrderRequest) (TradingOrder, error) {
	order, ok, err := s.findOrder(ctx, request)
	if err != nil {
		return TradingOrder{}, err
	}
	if !ok {
		return TradingOrder{}, ErrTradingOrderNotFound
	}
	return s.ensureOrderSubaccountNumber(order), nil
}

func (s *TradingService) ListOrders(ctx context.Context, userID string) ([]TradingOrder, error) {
	lister, ok := s.store.(interface {
		ListOrders(context.Context, string) ([]TradingOrder, error)
	})
	if !ok {
		return []TradingOrder{}, nil
	}
	orders, err := lister.ListOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i] = s.ensureOrderSubaccountNumber(orders[i])
	}
	return orders, nil
}

func (s *TradingService) findOrder(ctx context.Context, request TradingCancelOrderRequest) (TradingOrder, bool, error) {
	if request.OrderID != "" {
		return s.store.GetOrderByID(ctx, request.UserID, request.OrderID)
	}
	return s.store.GetOrderByClientID(ctx, request.UserID, request.ClientOrderID)
}

func orderRef(order TradingOrder) string {
	return "order:" + order.UserID + ":" + order.ClientOrderID
}

type MemoryTradingStore struct {
	mu               sync.Mutex
	ordersByID       map[string]TradingOrder
	ordersByClientID map[string]string
}

func NewMemoryTradingStore() *MemoryTradingStore {
	return &MemoryTradingStore{
		ordersByID:       map[string]TradingOrder{},
		ordersByClientID: map[string]string{},
	}
}

func (s *MemoryTradingStore) GetOrderByClientID(ctx context.Context, userID, clientOrderID string) (TradingOrder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.ordersByClientID[tradingClientKey(userID, clientOrderID)]
	if !ok {
		return TradingOrder{}, false, nil
	}
	order, ok := s.ordersByID[tradingOrderKey(userID, id)]
	return order, ok, nil
}

func (s *MemoryTradingStore) GetOrderByID(ctx context.Context, userID, orderID string) (TradingOrder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, ok := s.ordersByID[tradingOrderKey(userID, orderID)]
	return order, ok, nil
}

func (s *MemoryTradingStore) SaveOrder(ctx context.Context, order TradingOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order = stampMemoryTradingOrder(s.ordersByID[tradingOrderKey(order.UserID, order.ID)], order)
	s.ordersByID[tradingOrderKey(order.UserID, order.ID)] = order
	s.ordersByClientID[tradingClientKey(order.UserID, order.ClientOrderID)] = order.ID
	return nil
}

func (s *MemoryTradingStore) CreateOrderIfAbsent(ctx context.Context, order TradingOrder) (TradingOrder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientKey := tradingClientKey(order.UserID, order.ClientOrderID)
	if id, ok := s.ordersByClientID[clientKey]; ok {
		existing := s.ordersByID[tradingOrderKey(order.UserID, id)]
		return existing, false, nil
	}
	order = stampMemoryTradingOrder(TradingOrder{}, order)
	s.ordersByID[tradingOrderKey(order.UserID, order.ID)] = order
	s.ordersByClientID[clientKey] = order.ID
	return order, true, nil
}

func (s *MemoryTradingStore) ListOrders(ctx context.Context, userID string) ([]TradingOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	orders := make([]TradingOrder, 0)
	for _, order := range s.ordersByID {
		if order.UserID == userID {
			orders = append(orders, order)
		}
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].ClientOrderID == orders[j].ClientOrderID {
			return orders[i].ID < orders[j].ID
		}
		return orders[i].ClientOrderID < orders[j].ClientOrderID
	})
	return orders, nil
}

func (s *MemoryTradingStore) ListReconcilableOrders(ctx context.Context, limit int) ([]TradingOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = defaultTradingReconcileLimit
	}
	orders := make([]TradingOrder, 0)
	for _, order := range s.ordersByID {
		if !shouldListTradingOrderForReconcile(order) {
			continue
		}
		orders = append(orders, order)
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].ClientOrderID == orders[j].ClientOrderID {
			return orders[i].ID < orders[j].ID
		}
		return orders[i].ClientOrderID < orders[j].ClientOrderID
	})
	if len(orders) > limit {
		return orders[:limit], nil
	}
	return orders, nil
}

func shouldListTradingOrderForReconcile(order TradingOrder) bool {
	if isFailedFundedTradingOrder(order) {
		return true
	}
	return tradingOrderReconcileTxHash(order) != "" && isReconcilableTradingOrderStatus(order.Status)
}

func tradingOrderKey(userID, orderID string) string {
	return userID + ":" + orderID
}

func stampMemoryTradingOrder(existing, order TradingOrder) TradingOrder {
	now := time.Now().Unix()
	if order.CreatedAt == 0 {
		order.CreatedAt = existing.CreatedAt
	}
	if order.CreatedAt == 0 {
		order.CreatedAt = now
	}
	order.UpdatedAt = now
	return order
}

func tradingClientKey(userID, clientOrderID string) string {
	return userID + ":" + clientOrderID
}

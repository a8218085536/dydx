package tyche

import (
	"context"
	"errors"
	"hash/fnv"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	TradingTPSLKindTakeProfit = "take_profit"
	TradingTPSLKindStopLoss   = "stop_loss"

	TradingTPSLStatusWaitingTrigger = "waiting_trigger"
	TradingTPSLStatusTriggered      = "triggered"
	TradingTPSLStatusCancelled      = "cancelled"
	TradingTPSLStatusFailed         = "failed"
)

var ErrTradingTPSLNotFound = errors.New("trading tpsl not found")
var ErrTradingTPSLInvalidTrigger = errors.New("trading tpsl invalid trigger")
var ErrTradingTPSLPositionChanged = errors.New("trading tpsl position changed")

type TradingTPSLCreateRequest struct {
	UserID          string `json:"userId"`
	Market          string `json:"market"`
	Size            string `json:"size,omitempty"`
	TakeProfitPrice string `json:"takeProfitPrice,omitempty"`
	StopLossPrice   string `json:"stopLossPrice,omitempty"`
	SlippageBps     uint32 `json:"slippageBps,omitempty"`
}

type TradingTPSLCancelRequest struct {
	UserID  string `json:"userId"`
	ID      string `json:"id,omitempty"`
	GroupID string `json:"groupId,omitempty"`
}

type TradingTPSLGetRequest struct {
	UserID string `json:"userId"`
	ID     string `json:"id"`
}

type TradingTPSLListRequest struct {
	UserID string `json:"userId"`
}

type TradingTPSL struct {
	ID                 string `json:"id" redis:"id"`
	GroupID            string `json:"groupId" redis:"groupId"`
	UserID             string `json:"userId" redis:"userId"`
	DYDXAccount        string `json:"dydxAccount,omitempty" redis:"dydxAccount"`
	Market             string `json:"market" redis:"market"`
	Kind               string `json:"kind" redis:"kind"`
	PositionSide       string `json:"positionSide" redis:"positionSide"`
	TriggerPrice       string `json:"triggerPrice" redis:"triggerPrice"`
	Size               string `json:"size" redis:"size"`
	CloseClientOrderID string `json:"closeClientOrderId" redis:"closeClientOrderId"`
	CloseOrderID       string `json:"closeOrderId,omitempty" redis:"closeOrderId"`
	SlippageBps        uint32 `json:"slippageBps,omitempty" redis:"slippageBps"`
	Status             string `json:"status" redis:"status"`
	Reason             string `json:"reason,omitempty" redis:"reason"`
	CreatedAt          int64  `json:"createdAt" redis:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt" redis:"updatedAt"`
	TriggeredAt        int64  `json:"triggeredAt,omitempty" redis:"triggeredAt"`
}

type TradingTPSLGroup struct {
	GroupID string        `json:"groupId"`
	Orders  []TradingTPSL `json:"orders"`
}

type TradingTPSLStore interface {
	CreateTPSLIfAbsent(context.Context, TradingTPSL) (TradingTPSL, bool, error)
	SaveTPSL(context.Context, TradingTPSL) error
	GetTPSLByID(context.Context, string, string) (TradingTPSL, bool, error)
	ListTPSLByUser(context.Context, string) ([]TradingTPSL, error)
	ListTPSLByGroup(context.Context, string, string) ([]TradingTPSL, error)
	ListWaitingTPSL(context.Context, int) ([]TradingTPSL, error)
}

type TradingTPSLService struct {
	store     TradingTPSLStore
	trading   *TradingService
	prices    TradingMarketPriceSource
	positions TradingPositionProvider
	now       func() time.Time
}

func NewTradingTPSLService(store TradingTPSLStore, trading *TradingService, prices TradingMarketPriceSource, positions TradingPositionProvider) *TradingTPSLService {
	return &TradingTPSLService{
		store:     store,
		trading:   trading,
		prices:    prices,
		positions: positions,
		now:       time.Now,
	}
}

func (s *TradingTPSLService) Create(ctx context.Context, request TradingTPSLCreateRequest) (TradingTPSLGroup, error) {
	if s == nil || s.store == nil || s.trading == nil || s.positions == nil {
		return TradingTPSLGroup{}, ErrTradingPositionNotFound
	}
	if strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.Market) == "" {
		return TradingTPSLGroup{}, ErrTradingTPSLInvalidTrigger
	}
	if strings.TrimSpace(request.TakeProfitPrice) == "" && strings.TrimSpace(request.StopLossPrice) == "" {
		return TradingTPSLGroup{}, ErrTradingTPSLInvalidTrigger
	}
	position, err := s.positionForMarket(ctx, request.UserID, request.Market)
	if err != nil {
		if errors.Is(err, ErrTradingSyncEventNotFound) {
			return TradingTPSLGroup{}, ErrTradingPositionNotFound
		}
		return TradingTPSLGroup{}, err
	}
	positionSide, positionSize, err := tpslPositionSideAndAbsSize(position.Size)
	if err != nil {
		return TradingTPSLGroup{}, err
	}
	size := positionSize
	if strings.TrimSpace(request.Size) != "" {
		requestedSize, ok := parseDecimalRat(request.Size)
		if !ok || requestedSize.Sign() <= 0 || requestedSize.Cmp(positionSize) > 0 {
			return TradingTPSLGroup{}, ErrTradingPositionSizeInvalid
		}
		size = requestedSize
	}
	if err := validateTPSLTriggerPair(positionSide, request.TakeProfitPrice, request.StopLossPrice); err != nil {
		return TradingTPSLGroup{}, err
	}
	dydxAccount, err := s.trading.resolveDYDXAccount(ctx, request.UserID, "")
	if err != nil {
		return TradingTPSLGroup{}, err
	}

	now := s.now().Unix()
	groupID := uuid.NewString()
	orders := make([]TradingTPSL, 0, 2)
	for _, candidate := range []struct {
		kind  string
		price string
	}{
		{kind: TradingTPSLKindTakeProfit, price: request.TakeProfitPrice},
		{kind: TradingTPSLKindStopLoss, price: request.StopLossPrice},
	} {
		if strings.TrimSpace(candidate.price) == "" {
			continue
		}
		triggerPrice, ok := parseDecimalRat(candidate.price)
		if !ok || triggerPrice.Sign() <= 0 {
			return TradingTPSLGroup{}, ErrTradingTPSLInvalidTrigger
		}
		id := uuid.NewString()
		entry := TradingTPSL{
			ID:                 id,
			GroupID:            groupID,
			UserID:             request.UserID,
			DYDXAccount:        dydxAccount,
			Market:             request.Market,
			Kind:               candidate.kind,
			PositionSide:       positionSide,
			TriggerPrice:       ratToDecimalString(triggerPrice, 18),
			Size:               ratToDecimalString(size, 18),
			CloseClientOrderID: tpslCloseClientOrderID(id, candidate.kind),
			SlippageBps:        request.SlippageBps,
			Status:             TradingTPSLStatusWaitingTrigger,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		stored, _, err := s.store.CreateTPSLIfAbsent(ctx, entry)
		if err != nil {
			return TradingTPSLGroup{}, err
		}
		orders = append(orders, stored)
	}
	return TradingTPSLGroup{GroupID: groupID, Orders: orders}, nil
}

func (s *TradingTPSLService) PollOnce(ctx context.Context, limit int) ([]TradingTPSL, error) {
	if s == nil || s.store == nil || s.trading == nil || s.prices == nil {
		return []TradingTPSL{}, nil
	}
	entries, err := s.store.ListWaitingTPSL(ctx, limit)
	if err != nil {
		return nil, err
	}
	updated := make([]TradingTPSL, 0)
	for _, entry := range entries {
		latest, ok, err := s.store.GetTPSLByID(ctx, entry.UserID, entry.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		entry = latest
		if entry.Status != TradingTPSLStatusWaitingTrigger {
			continue
		}
		snapshot, err := s.prices.MarketPrice(ctx, TradingMarketPriceRequest{Market: entry.Market})
		if err != nil {
			continue
		}
		price, ok := parseDecimalRat(snapshot.Price)
		if !ok || price.Sign() <= 0 || !tpslShouldTrigger(entry, price) {
			continue
		}
		next, siblings, err := s.trigger(ctx, entry)
		if err != nil {
			if errors.Is(err, ErrTradingTPSLPositionChanged) || errors.Is(err, ErrTradingPositionNotFound) {
				cancelled, cancelErr := s.cancelWaitingGroup(ctx, entry, err.Error())
				if cancelErr != nil {
					return nil, cancelErr
				}
				updated = append(updated, cancelled...)
				continue
			}
			next = entry
			next.Status = TradingTPSLStatusFailed
			next.Reason = err.Error()
			next.UpdatedAt = s.now().Unix()
			if saveErr := s.store.SaveTPSL(ctx, next); saveErr != nil {
				return nil, saveErr
			}
			updated = append(updated, next)
			continue
		}
		updated = append(updated, next)
		updated = append(updated, siblings...)
	}
	return updated, nil
}

func (s *TradingTPSLService) Get(ctx context.Context, request TradingTPSLGetRequest) (TradingTPSL, error) {
	entry, ok, err := s.store.GetTPSLByID(ctx, request.UserID, request.ID)
	if err != nil {
		return TradingTPSL{}, err
	}
	if !ok {
		return TradingTPSL{}, ErrTradingTPSLNotFound
	}
	return entry, nil
}

func (s *TradingTPSLService) List(ctx context.Context, request TradingTPSLListRequest) ([]TradingTPSL, error) {
	return s.store.ListTPSLByUser(ctx, request.UserID)
}

func (s *TradingTPSLService) Cancel(ctx context.Context, request TradingTPSLCancelRequest) ([]TradingTPSL, error) {
	var entries []TradingTPSL
	var err error
	if request.GroupID != "" {
		entries, err = s.store.ListTPSLByGroup(ctx, request.UserID, request.GroupID)
	} else if request.ID != "" {
		var entry TradingTPSL
		var ok bool
		entry, ok, err = s.store.GetTPSLByID(ctx, request.UserID, request.ID)
		if ok {
			entries = []TradingTPSL{entry}
		}
	} else {
		return nil, ErrTradingTPSLNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrTradingTPSLNotFound
	}
	now := s.now().Unix()
	cancelled := make([]TradingTPSL, 0, len(entries))
	for _, entry := range entries {
		if entry.Status != TradingTPSLStatusWaitingTrigger {
			continue
		}
		entry.Status = TradingTPSLStatusCancelled
		entry.Reason = "cancelled by user"
		entry.UpdatedAt = now
		if err := s.store.SaveTPSL(ctx, entry); err != nil {
			return nil, err
		}
		cancelled = append(cancelled, entry)
	}
	return cancelled, nil
}

func (s *TradingTPSLService) trigger(ctx context.Context, entry TradingTPSL) (TradingTPSL, []TradingTPSL, error) {
	if s.positions != nil {
		position, err := s.positionForMarket(ctx, entry.UserID, entry.Market)
		if err != nil {
			if errors.Is(err, ErrTradingSyncEventNotFound) {
				return TradingTPSL{}, nil, ErrTradingPositionNotFound
			}
			return TradingTPSL{}, nil, err
		}
		positionSide, _, err := tpslPositionSideAndAbsSize(position.Size)
		if err != nil {
			return TradingTPSL{}, nil, err
		}
		if positionSide != entry.PositionSide {
			return TradingTPSL{}, nil, ErrTradingTPSLPositionChanged
		}
	}
	order, err := s.trading.ClosePosition(ctx, TradingClosePositionRequest{
		UserID:             entry.UserID,
		ClientOrderID:      entry.CloseClientOrderID,
		Market:             entry.Market,
		OrderType:          TradingOrderTypeMarket,
		ExecutionOrderType: TradingOrderTypeIOC,
		SlippageBps:        entry.SlippageBps,
		Size:               entry.Size,
	})
	if err != nil {
		return TradingTPSL{}, nil, err
	}
	now := s.now().Unix()
	entry.Status = TradingTPSLStatusTriggered
	entry.CloseOrderID = order.ID
	entry.TriggeredAt = now
	entry.UpdatedAt = now
	if err := s.store.SaveTPSL(ctx, entry); err != nil {
		return TradingTPSL{}, nil, err
	}
	siblings, err := s.cancelWaitingSiblings(ctx, entry)
	if err != nil {
		return TradingTPSL{}, nil, err
	}
	return entry, siblings, nil
}

func (s *TradingTPSLService) cancelWaitingGroup(ctx context.Context, entry TradingTPSL, reason string) ([]TradingTPSL, error) {
	entries, err := s.store.ListTPSLByGroup(ctx, entry.UserID, entry.GroupID)
	if err != nil {
		return nil, err
	}
	now := s.now().Unix()
	cancelled := make([]TradingTPSL, 0)
	for _, item := range entries {
		if item.Status != TradingTPSLStatusWaitingTrigger {
			continue
		}
		item.Status = TradingTPSLStatusCancelled
		item.Reason = reason
		item.UpdatedAt = now
		if err := s.store.SaveTPSL(ctx, item); err != nil {
			return nil, err
		}
		cancelled = append(cancelled, item)
	}
	return cancelled, nil
}

func (s *TradingTPSLService) cancelWaitingSiblings(ctx context.Context, triggered TradingTPSL) ([]TradingTPSL, error) {
	entries, err := s.store.ListTPSLByGroup(ctx, triggered.UserID, triggered.GroupID)
	if err != nil {
		return nil, err
	}
	now := s.now().Unix()
	cancelled := make([]TradingTPSL, 0)
	for _, entry := range entries {
		if entry.ID == triggered.ID || entry.Status != TradingTPSLStatusWaitingTrigger {
			continue
		}
		entry.Status = TradingTPSLStatusCancelled
		entry.Reason = "oco sibling triggered"
		entry.UpdatedAt = now
		if err := s.store.SaveTPSL(ctx, entry); err != nil {
			return nil, err
		}
		cancelled = append(cancelled, entry)
	}
	return cancelled, nil
}

type TradingTPSLWorker struct {
	service *TradingTPSLService
	limit   int
}

func NewTradingTPSLWorker(service *TradingTPSLService, limit int) *TradingTPSLWorker {
	return &TradingTPSLWorker{service: service, limit: limit}
}

func (w *TradingTPSLWorker) PollOnce(ctx context.Context) error {
	if w == nil || w.service == nil {
		return nil
	}
	_, err := w.service.PollOnce(ctx, w.limit)
	return err
}

func (s *TradingTPSLService) positionForMarket(ctx context.Context, userID, market string) (TradingPosition, error) {
	if s.trading != nil {
		if provider, ok := s.positions.(TradingSubaccountPositionProvider); ok {
			return provider.GetPositionForSubaccount(ctx, userID, market, s.trading.SubaccountNumberForMarket(market))
		}
	}
	return s.positions.GetPosition(ctx, userID, market)
}

func tpslPositionSideAndAbsSize(size string) (string, *big.Rat, error) {
	position, ok := parseDecimalRat(size)
	if !ok {
		return "", nil, ErrTradingPositionSizeInvalid
	}
	if position.Sign() == 0 {
		return "", nil, ErrTradingPositionNotFound
	}
	side := "long"
	if position.Sign() < 0 {
		side = "short"
	}
	return side, new(big.Rat).Abs(position), nil
}

func validateTPSLTriggerPair(positionSide, takeProfitPrice, stopLossPrice string) error {
	if strings.TrimSpace(takeProfitPrice) == "" || strings.TrimSpace(stopLossPrice) == "" {
		return nil
	}
	tp, ok := parseDecimalRat(takeProfitPrice)
	if !ok || tp.Sign() <= 0 {
		return ErrTradingTPSLInvalidTrigger
	}
	sl, ok := parseDecimalRat(stopLossPrice)
	if !ok || sl.Sign() <= 0 {
		return ErrTradingTPSLInvalidTrigger
	}
	switch positionSide {
	case "long":
		if tp.Cmp(sl) <= 0 {
			return ErrTradingTPSLInvalidTrigger
		}
	case "short":
		if tp.Cmp(sl) >= 0 {
			return ErrTradingTPSLInvalidTrigger
		}
	default:
		return ErrTradingTPSLInvalidTrigger
	}
	return nil
}

func tpslShouldTrigger(entry TradingTPSL, currentPrice *big.Rat) bool {
	triggerPrice, ok := parseDecimalRat(entry.TriggerPrice)
	if !ok || triggerPrice.Sign() <= 0 || currentPrice == nil || currentPrice.Sign() <= 0 {
		return false
	}
	switch entry.Kind {
	case TradingTPSLKindTakeProfit:
		if entry.PositionSide == "short" {
			return currentPrice.Cmp(triggerPrice) <= 0
		}
		return currentPrice.Cmp(triggerPrice) >= 0
	case TradingTPSLKindStopLoss:
		if entry.PositionSide == "short" {
			return currentPrice.Cmp(triggerPrice) >= 0
		}
		return currentPrice.Cmp(triggerPrice) <= 0
	default:
		return false
	}
}

func tpslCloseClientOrderID(id, kind string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id + ":" + kind))
	value := hash.Sum32()
	if value == 0 {
		value = 1
	}
	return strconv.FormatUint(uint64(value), 10)
}

type MemoryTradingTPSLStore struct {
	mu      sync.Mutex
	byID    map[string]TradingTPSL
	byUser  map[string]map[string]bool
	byGroup map[string]map[string]bool
}

func NewMemoryTradingTPSLStore() *MemoryTradingTPSLStore {
	return &MemoryTradingTPSLStore{
		byID:    map[string]TradingTPSL{},
		byUser:  map[string]map[string]bool{},
		byGroup: map[string]map[string]bool{},
	}
}

func (s *MemoryTradingTPSLStore) CreateTPSLIfAbsent(ctx context.Context, entry TradingTPSL) (TradingTPSL, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := tpslKey(entry.UserID, entry.ID)
	if existing, ok := s.byID[key]; ok {
		return existing, false, nil
	}
	s.byID[key] = entry
	if s.byUser[entry.UserID] == nil {
		s.byUser[entry.UserID] = map[string]bool{}
	}
	s.byUser[entry.UserID][entry.ID] = true
	groupKey := tpslGroupKey(entry.UserID, entry.GroupID)
	if s.byGroup[groupKey] == nil {
		s.byGroup[groupKey] = map[string]bool{}
	}
	s.byGroup[groupKey][entry.ID] = true
	return entry, true, nil
}

func (s *MemoryTradingTPSLStore) SaveTPSL(ctx context.Context, entry TradingTPSL) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.byID[tpslKey(entry.UserID, entry.ID)] = entry
	if s.byUser[entry.UserID] == nil {
		s.byUser[entry.UserID] = map[string]bool{}
	}
	s.byUser[entry.UserID][entry.ID] = true
	groupKey := tpslGroupKey(entry.UserID, entry.GroupID)
	if s.byGroup[groupKey] == nil {
		s.byGroup[groupKey] = map[string]bool{}
	}
	s.byGroup[groupKey][entry.ID] = true
	return nil
}

func (s *MemoryTradingTPSLStore) GetTPSLByID(ctx context.Context, userID, id string) (TradingTPSL, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tpslKey(userID, id)]
	return entry, ok, nil
}

func (s *MemoryTradingTPSLStore) ListTPSLByUser(ctx context.Context, userID string) ([]TradingTPSL, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]TradingTPSL, 0)
	for id := range s.byUser[userID] {
		if entry, ok := s.byID[tpslKey(userID, id)]; ok {
			entries = append(entries, entry)
		}
	}
	sortTPSLEntries(entries)
	return entries, nil
}

func (s *MemoryTradingTPSLStore) ListTPSLByGroup(ctx context.Context, userID, groupID string) ([]TradingTPSL, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]TradingTPSL, 0)
	for id := range s.byGroup[tpslGroupKey(userID, groupID)] {
		if entry, ok := s.byID[tpslKey(userID, id)]; ok {
			entries = append(entries, entry)
		}
	}
	sortTPSLEntries(entries)
	return entries, nil
}

func (s *MemoryTradingTPSLStore) ListWaitingTPSL(ctx context.Context, limit int) ([]TradingTPSL, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	entries := make([]TradingTPSL, 0)
	for _, entry := range s.byID {
		if entry.Status == TradingTPSLStatusWaitingTrigger {
			entries = append(entries, entry)
		}
	}
	sortTPSLEntries(entries)
	if len(entries) > limit {
		return entries[:limit], nil
	}
	return entries, nil
}

func sortTPSLEntries(entries []TradingTPSL) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt == entries[j].CreatedAt {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].CreatedAt < entries[j].CreatedAt
	})
}

func tpslKey(userID, id string) string {
	return userID + ":" + id
}

func tpslGroupKey(userID, groupID string) string {
	return userID + ":" + groupID
}

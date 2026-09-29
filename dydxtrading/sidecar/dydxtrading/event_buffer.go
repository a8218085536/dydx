package dydxtrading

import (
	"context"
	"errors"
	"strconv"
	"sync"
)

type TradingEventBatch struct {
	Fills         []TradingFillEvent        `json:"fills"`
	OrderStatuses []TradingOrderStatusEvent `json:"orderStatuses"`
	Positions     []TradingPositionEvent    `json:"positions"`
}

type TradingEventIngestor interface {
	Ingest(context.Context, TradingEventBatch) error
}

type BufferedTradingEventProvider interface {
	TradingEventProvider
	TradingEventIngestor
}

type MemoryTradingEventBuffer struct {
	mu               sync.RWMutex
	fillIDs          map[string]bool
	orderStatusIDs   map[string]bool
	positionIDs      map[string]bool
	fills            []TradingFillEvent
	orderStatuses    []TradingOrderStatusEvent
	positions        []TradingPositionEvent
	defaultPageLimit int
}

func NewMemoryTradingEventBuffer() *MemoryTradingEventBuffer {
	return &MemoryTradingEventBuffer{
		fillIDs:          map[string]bool{},
		orderStatusIDs:   map[string]bool{},
		positionIDs:      map[string]bool{},
		defaultPageLimit: 200,
	}
}

func (b *MemoryTradingEventBuffer) Ingest(ctx context.Context, batch TradingEventBatch) error {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, event := range batch.Fills {
		if event.EventID == "" {
			return errors.New("missing event id for fill")
		}
		if b.fillIDs[event.EventID] {
			continue
		}
		b.fillIDs[event.EventID] = true
		b.fills = append(b.fills, event)
	}
	for _, event := range batch.OrderStatuses {
		if event.EventID == "" {
			return errors.New("missing event id for order status")
		}
		if b.orderStatusIDs[event.EventID] {
			continue
		}
		b.orderStatusIDs[event.EventID] = true
		b.orderStatuses = append(b.orderStatuses, event)
	}
	for _, event := range batch.Positions {
		if event.EventID == "" {
			return errors.New("missing event id for position")
		}
		if b.positionIDs[event.EventID] {
			continue
		}
		b.positionIDs[event.EventID] = true
		b.positions = append(b.positions, event)
	}
	return nil
}

func (b *MemoryTradingEventBuffer) Fills(ctx context.Context, cursor string) (TradingEventPage[TradingFillEvent], error) {
	return b.FillsPage(ctx, cursor, b.defaultPageLimit)
}

func (b *MemoryTradingEventBuffer) OrderStatuses(ctx context.Context, cursor string) (TradingEventPage[TradingOrderStatusEvent], error) {
	return b.OrderStatusesPage(ctx, cursor, b.defaultPageLimit)
}

func (b *MemoryTradingEventBuffer) Positions(ctx context.Context, cursor string) (TradingEventPage[TradingPositionEvent], error) {
	return b.PositionsPage(ctx, cursor, b.defaultPageLimit)
}

func (b *MemoryTradingEventBuffer) FillsPage(ctx context.Context, cursor string, limit int) (TradingEventPage[TradingFillEvent], error) {
	_ = ctx
	b.mu.RLock()
	defer b.mu.RUnlock()
	return pageEvents(b.fills, cursor, limit)
}

func (b *MemoryTradingEventBuffer) OrderStatusesPage(ctx context.Context, cursor string, limit int) (TradingEventPage[TradingOrderStatusEvent], error) {
	_ = ctx
	b.mu.RLock()
	defer b.mu.RUnlock()
	return pageEvents(b.orderStatuses, cursor, limit)
}

func (b *MemoryTradingEventBuffer) PositionsPage(ctx context.Context, cursor string, limit int) (TradingEventPage[TradingPositionEvent], error) {
	_ = ctx
	b.mu.RLock()
	defer b.mu.RUnlock()
	return pageEvents(b.positions, cursor, limit)
}

func pageEvents[T any](events []T, cursor string, limit int) (TradingEventPage[T], error) {
	if limit <= 0 {
		limit = 200
	}
	start, err := parseEventCursor(cursor)
	if err != nil {
		return TradingEventPage[T]{}, err
	}
	if start > len(events) {
		start = len(events)
	}
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	page := append([]T(nil), events[start:end]...)
	return TradingEventPage[T]{
		Data:       page,
		NextCursor: strconv.Itoa(end),
	}, nil
}

func parseEventCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(cursor)
	if err != nil || value < 0 {
		return 0, errors.New("invalid event cursor")
	}
	return value, nil
}

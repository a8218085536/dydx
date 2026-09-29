package dydxtrading

import (
	"context"
	"errors"
	"sync"
)

const (
	NormalizedIndexerEventTypeOrderFill            = "order_fill"
	NormalizedIndexerEventTypeStatefulOrderRemoval = "stateful_order_removal"
	NormalizedIndexerEventTypeSubaccountUpdate     = "subaccount_update"
)

type NormalizedIndexerEvent struct {
	EventID              string                               `json:"eventId"`
	Type                 string                               `json:"type"`
	OrderFill            *NormalizedOrderFillEvent            `json:"orderFill,omitempty"`
	StatefulOrderRemoval *NormalizedStatefulOrderRemovalEvent `json:"statefulOrderRemoval,omitempty"`
	SubaccountUpdate     *NormalizedSubaccountUpdateEvent     `json:"subaccountUpdate,omitempty"`
}

type NormalizedIndexerEventPage struct {
	Events     []NormalizedIndexerEvent
	NextCursor string
}

type NormalizedIndexerEventSource interface {
	Fetch(context.Context, string) (NormalizedIndexerEventPage, error)
}

type IndexerConsumerCursorStore interface {
	GetIndexerConsumerCursor(context.Context) (string, error)
	SaveIndexerConsumerCursor(context.Context, string) error
}

type IndexerConsumerWorkerConfig struct {
	Source     NormalizedIndexerEventSource
	Cursor     IndexerConsumerCursorStore
	Ingestor   TradingEventIngestor
	Translator *IndexerEventTranslator
}

type IndexerConsumerWorker struct {
	source     NormalizedIndexerEventSource
	cursor     IndexerConsumerCursorStore
	ingestor   TradingEventIngestor
	translator *IndexerEventTranslator
}

func NewIndexerConsumerWorker(cfg IndexerConsumerWorkerConfig) *IndexerConsumerWorker {
	return &IndexerConsumerWorker{
		source:     cfg.Source,
		cursor:     cfg.Cursor,
		ingestor:   cfg.Ingestor,
		translator: cfg.Translator,
	}
}

func (w *IndexerConsumerWorker) PollOnce(ctx context.Context) error {
	if w.source == nil {
		return errors.New("missing indexer event source")
	}
	if w.cursor == nil {
		return errors.New("missing indexer consumer cursor store")
	}
	if w.ingestor == nil {
		return errors.New("missing trading event ingestor")
	}
	if w.translator == nil {
		return errors.New("missing indexer event translator")
	}
	cursor, err := w.cursor.GetIndexerConsumerCursor(ctx)
	if err != nil {
		return err
	}
	page, err := w.source.Fetch(ctx, cursor)
	if err != nil {
		return err
	}
	batch, err := w.translate(ctx, page.Events)
	if err != nil {
		return err
	}
	if hasTradingEvents(batch) {
		if err := w.ingestor.Ingest(ctx, batch); err != nil {
			return err
		}
	}
	if page.NextCursor != "" {
		return w.cursor.SaveIndexerConsumerCursor(ctx, page.NextCursor)
	}
	return nil
}

func (w *IndexerConsumerWorker) translate(ctx context.Context, events []NormalizedIndexerEvent) (TradingEventBatch, error) {
	var batch TradingEventBatch
	for _, event := range events {
		switch event.Type {
		case NormalizedIndexerEventTypeOrderFill:
			if event.OrderFill == nil {
				continue
			}
			fills, err := w.translator.TranslateOrderFill(ctx, event.EventID, *event.OrderFill)
			if err != nil {
				return TradingEventBatch{}, err
			}
			batch.Fills = append(batch.Fills, fills...)
		case NormalizedIndexerEventTypeStatefulOrderRemoval:
			if event.StatefulOrderRemoval == nil {
				continue
			}
			status, err := w.translator.TranslateStatefulOrderRemoval(ctx, event.EventID, *event.StatefulOrderRemoval)
			if err != nil {
				return TradingEventBatch{}, err
			}
			if status.EventID != "" {
				batch.OrderStatuses = append(batch.OrderStatuses, status)
			}
		case NormalizedIndexerEventTypeSubaccountUpdate:
			if event.SubaccountUpdate == nil {
				continue
			}
			positions, err := w.translator.TranslateSubaccountUpdate(ctx, event.EventID, *event.SubaccountUpdate)
			if err != nil {
				return TradingEventBatch{}, err
			}
			batch.Positions = append(batch.Positions, positions...)
		}
	}
	return batch, nil
}

func hasTradingEvents(batch TradingEventBatch) bool {
	return len(batch.Fills) > 0 || len(batch.OrderStatuses) > 0 || len(batch.Positions) > 0
}

type MemoryIndexerConsumerCursorStore struct {
	mu     sync.Mutex
	cursor string
}

func NewMemoryIndexerConsumerCursorStore(cursor string) *MemoryIndexerConsumerCursorStore {
	return &MemoryIndexerConsumerCursorStore{cursor: cursor}
}

func (s *MemoryIndexerConsumerCursorStore) GetIndexerConsumerCursor(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursor, nil
}

func (s *MemoryIndexerConsumerCursorStore) SaveIndexerConsumerCursor(_ context.Context, cursor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursor = cursor
	return nil
}

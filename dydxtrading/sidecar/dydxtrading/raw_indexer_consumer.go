package dydxtrading

import (
	"errors"
	"net/http"
)

type RawIndexerConsumerWorkerConfig struct {
	Source              NormalizedIndexerEventSource
	RawSourceURL        string
	RawSourceAPIKey     string
	TycheURL            string
	TycheInternalAPIKey string
	Client              *http.Client
	Cursor              IndexerConsumerCursorStore
	Ingestor            TradingEventIngestor
	Markets             map[uint32]TradingMarketMapping
	Perpetuals          map[uint32]TradingMarketMapping
}

func NewRawIndexerConsumerWorker(cfg RawIndexerConsumerWorkerConfig) (*IndexerConsumerWorker, bool, error) {
	source := cfg.Source
	if source == nil && cfg.RawSourceURL == "" {
		return nil, false, nil
	}
	if cfg.TycheURL == "" {
		return nil, false, errors.New("missing Tyche URL")
	}
	if cfg.Ingestor == nil {
		return nil, false, errors.New("missing trading event ingestor")
	}
	cursor := cfg.Cursor
	if cursor == nil {
		cursor = NewMemoryIndexerConsumerCursorStore("")
	}
	if source == nil {
		source = NewHTTPRawIndexerEventSource(HTTPRawIndexerEventSourceConfig{
			URL:    cfg.RawSourceURL,
			APIKey: cfg.RawSourceAPIKey,
			Client: cfg.Client,
		})
	}
	resolver := NewTycheDYDXUserResolver(TycheDYDXUserResolverConfig{
		URL:            cfg.TycheURL,
		InternalAPIKey: cfg.TycheInternalAPIKey,
		Client:         cfg.Client,
	})
	worker := NewIndexerConsumerWorker(IndexerConsumerWorkerConfig{
		Source:   source,
		Cursor:   cursor,
		Ingestor: cfg.Ingestor,
		Translator: NewIndexerEventTranslator(IndexerEventTranslatorConfig{
			UserResolver: resolver,
			Markets:      cfg.Markets,
			Perpetuals:   cfg.Perpetuals,
		}),
	})
	return worker, true, nil
}

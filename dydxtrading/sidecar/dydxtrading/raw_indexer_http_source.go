package dydxtrading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type HTTPRawIndexerEventSourceConfig struct {
	URL    string
	APIKey string
	Client *http.Client
}

type HTTPRawIndexerEventSource struct {
	url     string
	apiKey  string
	client  *http.Client
	adapter *RawIndexerEventAdapter
}

func NewHTTPRawIndexerEventSource(cfg HTTPRawIndexerEventSourceConfig) *HTTPRawIndexerEventSource {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPRawIndexerEventSource{
		url:     cfg.URL,
		apiKey:  cfg.APIKey,
		client:  client,
		adapter: NewRawIndexerEventAdapter(),
	}
}

func (s *HTTPRawIndexerEventSource) Fetch(ctx context.Context, cursor string) (NormalizedIndexerEventPage, error) {
	if s.url == "" {
		return NormalizedIndexerEventPage{}, errors.New("missing raw indexer event source URL")
	}
	endpoint, err := url.Parse(s.url)
	if err != nil {
		return NormalizedIndexerEventPage{}, err
	}
	query := endpoint.Query()
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return NormalizedIndexerEventPage{}, err
	}
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return NormalizedIndexerEventPage{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Code       int               `json:"code"`
		Msg        string            `json:"msg"`
		NextCursor string            `json:"nextCursor"`
		Data       []RawIndexerEvent `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return NormalizedIndexerEventPage{}, err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("raw indexer event source returned code %d", result.Code)
		}
		return NormalizedIndexerEventPage{}, errors.New(result.Msg)
	}

	events := make([]NormalizedIndexerEvent, 0, len(result.Data))
	for _, rawEvent := range result.Data {
		event, err := s.adapter.Convert(rawEvent)
		if err != nil {
			return NormalizedIndexerEventPage{}, err
		}
		events = append(events, event)
	}
	return NormalizedIndexerEventPage{Events: events, NextCursor: result.NextCursor}, nil
}

package dydxtrading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type HTTPNormalizedIndexerEventSourceConfig struct {
	URL    string
	APIKey string
	Client *http.Client
}

type HTTPNormalizedIndexerEventSource struct {
	url    string
	apiKey string
	client *http.Client
}

func NewHTTPNormalizedIndexerEventSource(cfg HTTPNormalizedIndexerEventSourceConfig) *HTTPNormalizedIndexerEventSource {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPNormalizedIndexerEventSource{url: cfg.URL, apiKey: cfg.APIKey, client: client}
}

func (s *HTTPNormalizedIndexerEventSource) Fetch(ctx context.Context, cursor string) (NormalizedIndexerEventPage, error) {
	if s.url == "" {
		return NormalizedIndexerEventPage{}, errors.New("missing normalized indexer event source URL")
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
		Code       int                      `json:"code"`
		Msg        string                   `json:"msg"`
		NextCursor string                   `json:"nextCursor"`
		Data       []NormalizedIndexerEvent `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return NormalizedIndexerEventPage{}, err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("normalized indexer event source returned code %d", result.Code)
		}
		return NormalizedIndexerEventPage{}, errors.New(result.Msg)
	}
	return NormalizedIndexerEventPage{Events: result.Data, NextCursor: result.NextCursor}, nil
}

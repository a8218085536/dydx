package tyche

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type DYDXTradingEventHTTPSourceConfig struct {
	URL    string `json:"url" toml:"url"`
	APIKey string `json:"apiKey" toml:"apiKey"`
}

type DYDXTradingEventHTTPSource struct {
	conf   DYDXTradingEventHTTPSourceConfig
	client *http.Client
}

func NewDYDXTradingEventHTTPSource(conf DYDXTradingEventHTTPSourceConfig) *DYDXTradingEventHTTPSource {
	return &DYDXTradingEventHTTPSource{conf: conf, client: http.DefaultClient}
}

func (s *DYDXTradingEventHTTPSource) FetchFills(ctx context.Context, cursor string) ([]TradingFillSyncRequest, string, error) {
	var out []TradingFillSyncRequest
	next, err := s.get(ctx, "/fills", cursor, &out)
	return out, next, err
}

func (s *DYDXTradingEventHTTPSource) FetchPositions(ctx context.Context, cursor string) ([]TradingPositionSyncRequest, string, error) {
	var out []TradingPositionSyncRequest
	next, err := s.get(ctx, "/positions", cursor, &out)
	return out, next, err
}

func (s *DYDXTradingEventHTTPSource) FetchOrderStatuses(ctx context.Context, cursor string) ([]TradingOrderStatusSyncRequest, string, error) {
	var out []TradingOrderStatusSyncRequest
	next, err := s.get(ctx, "/order-statuses", cursor, &out)
	return out, next, err
}

func (s *DYDXTradingEventHTTPSource) get(ctx context.Context, path, cursor string, target interface{}) (string, error) {
	if s.conf.URL == "" {
		return "", ErrDYDXChainClientNotConfigured
	}
	endpoint, err := url.Parse(strings.TrimRight(s.conf.URL, "/") + path)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	if s.conf.APIKey != "" {
		req.Header.Set("X-API-Key", s.conf.APIKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code       int             `json:"code"`
		Msg        string          `json:"msg"`
		NextCursor string          `json:"nextCursor"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("dydx event source returned code %d", result.Code)
		}
		return "", errors.New(result.Msg)
	}
	if len(result.Data) > 0 {
		if err := json.Unmarshal(result.Data, target); err != nil {
			return "", err
		}
	}
	return result.NextCursor, nil
}

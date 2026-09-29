package dydxtrading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type RESTTradingEventProviderConfig struct {
	URL    string
	APIKey string
	Client *http.Client
}

type RESTTradingEventProvider struct {
	url    string
	apiKey string
	client *http.Client
}

func NewRESTTradingEventProvider(cfg RESTTradingEventProviderConfig) *RESTTradingEventProvider {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &RESTTradingEventProvider{
		url:    strings.TrimRight(cfg.URL, "/"),
		apiKey: cfg.APIKey,
		client: client,
	}
}

func (p *RESTTradingEventProvider) Fills(ctx context.Context, cursor string) (TradingEventPage[TradingFillEvent], error) {
	var data []TradingFillEvent
	next, err := p.get(ctx, "/fills", cursor, &data)
	return TradingEventPage[TradingFillEvent]{Data: data, NextCursor: next}, err
}

func (p *RESTTradingEventProvider) OrderStatuses(ctx context.Context, cursor string) (TradingEventPage[TradingOrderStatusEvent], error) {
	var data []TradingOrderStatusEvent
	next, err := p.get(ctx, "/order-statuses", cursor, &data)
	return TradingEventPage[TradingOrderStatusEvent]{Data: data, NextCursor: next}, err
}

func (p *RESTTradingEventProvider) Positions(ctx context.Context, cursor string) (TradingEventPage[TradingPositionEvent], error) {
	var data []TradingPositionEvent
	next, err := p.get(ctx, "/positions", cursor, &data)
	return TradingEventPage[TradingPositionEvent]{Data: data, NextCursor: next}, err
}

func (p *RESTTradingEventProvider) get(ctx context.Context, path, cursor string, target any) (string, error) {
	if p.url == "" {
		return "", errors.New("missing dYdX event upstream URL")
	}
	endpoint, err := url.Parse(p.url + path)
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
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}

	resp, err := p.client.Do(req)
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
			result.Msg = fmt.Sprintf("dYdX event upstream returned code %d", result.Code)
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

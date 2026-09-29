package dydxtrading

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type HTTPTradingEventIngestorConfig struct {
	URL         string
	BearerToken string
	Client      *http.Client
}

type HTTPTradingEventIngestor struct {
	url         string
	bearerToken string
	client      *http.Client
}

func NewHTTPTradingEventIngestor(cfg HTTPTradingEventIngestorConfig) *HTTPTradingEventIngestor {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTradingEventIngestor{
		url:         strings.TrimRight(cfg.URL, "/"),
		bearerToken: cfg.BearerToken,
		client:      client,
	}
}

func (i *HTTPTradingEventIngestor) Ingest(ctx context.Context, batch TradingEventBatch) error {
	if i.url == "" {
		return errors.New("missing dYdX event ingest URL")
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.url+"/dydx/events/ingest", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if i.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+i.bearerToken)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("dYdX event ingest returned code %d", result.Code)
		}
		return errors.New(result.Msg)
	}
	return nil
}

package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type DYDXHTTPBroadcasterConfig struct {
	URL     string
	Account string
}

type DYDXHTTPBroadcaster struct {
	conf   DYDXHTTPBroadcasterConfig
	client *http.Client
}

func NewDYDXHTTPBroadcaster(conf DYDXHTTPBroadcasterConfig) *DYDXHTTPBroadcaster {
	return &DYDXHTTPBroadcaster{
		conf:   conf,
		client: http.DefaultClient,
	}
}

func (b *DYDXHTTPBroadcaster) Broadcast(ctx context.Context, intent DYDXIntent) (string, error) {
	if b.conf.URL == "" {
		return "", ErrDYDXBroadcastNotConfigured
	}

	body := map[string]interface{}{
		"id":      intent.ID,
		"action":  intent.Action,
		"account": b.conf.Account,
		"params":  intent.Params,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.conf.URL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TxHash string `json:"txHash"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("gateway returned code %d", result.Code)
		}
		return "", errors.New(result.Msg)
	}
	if result.Data.TxHash == "" {
		return "", errors.New("gateway response missing txHash")
	}
	return result.Data.TxHash, nil
}

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

const tycheInternalAPIKeyHeader = "X-Tyche-Internal-Key"

type TycheDYDXUserResolverConfig struct {
	URL            string
	InternalAPIKey string
	Client         *http.Client
}

type TycheDYDXUserResolver struct {
	url            string
	internalAPIKey string
	client         *http.Client
}

func NewTycheDYDXUserResolver(cfg TycheDYDXUserResolverConfig) *TycheDYDXUserResolver {
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &TycheDYDXUserResolver{
		url:            strings.TrimRight(cfg.URL, "/"),
		internalAPIKey: cfg.InternalAPIKey,
		client:         client,
	}
}

func (r *TycheDYDXUserResolver) UserIDByDYDXAddress(ctx context.Context, dydxAddress string) (string, bool, error) {
	if r.url == "" {
		return "", false, errors.New("missing Tyche URL")
	}
	body, err := json.Marshal(map[string]string{"dydxAddress": dydxAddress})
	if err != nil {
		return "", false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/tyche/api/wallet-binding/get-by-dydx", bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.internalAPIKey != "" {
		req.Header.Set(tycheInternalAPIKeyHeader, r.internalAPIKey)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserID      string `json:"userId"`
			DYDXAddress string `json:"dydxAddress"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, err
	}
	if result.Code == http.StatusBadRequest && strings.Contains(strings.ToLower(result.Msg), "not found") {
		return "", false, nil
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("Tyche wallet binding resolver returned code %d", result.Code)
		}
		return "", false, errors.New(result.Msg)
	}
	if result.Data.Status != "active" || result.Data.UserID == "" {
		return "", false, nil
	}
	return result.Data.UserID, true, nil
}

package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

var ErrDYDXWalletProviderNotConfigured = errors.New("dydx wallet provider is not configured")

type DYDXProxyWalletHTTPProviderConfig struct {
	URL         string
	BearerToken string
	Client      *http.Client
}

type DYDXProxyWalletHTTPProvider struct {
	conf   DYDXProxyWalletHTTPProviderConfig
	client *http.Client
}

func NewDYDXProxyWalletHTTPProvider(conf DYDXProxyWalletHTTPProviderConfig) *DYDXProxyWalletHTTPProvider {
	client := conf.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &DYDXProxyWalletHTTPProvider{conf: conf, client: client}
}

func (p *DYDXProxyWalletHTTPProvider) CreateDYDXAddress(ctx context.Context, user AuthUser) (string, error) {
	if p == nil || p.conf.URL == "" {
		return "", ErrDYDXWalletProviderNotConfigured
	}
	payload := map[string]string{
		"userId":      user.UserID,
		"necoAddress": user.NECOAddress,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.conf.URL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.conf.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.conf.BearerToken)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("dydx wallet provider returned http %d", resp.StatusCode)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			DYDXAddress string `json:"dydxAddress"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("dydx wallet provider returned code %d", result.Code)
		}
		return "", errors.New(result.Msg)
	}
	if result.Data.DYDXAddress == "" {
		return "", errors.New("dydx wallet provider response missing dydxAddress")
	}
	return result.Data.DYDXAddress, nil
}

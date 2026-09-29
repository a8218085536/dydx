package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type DYDXChainHTTPClientConfig struct {
	URL string
}

type DYDXChainHTTPClient struct {
	conf   DYDXChainHTTPClientConfig
	client *http.Client
}

func NewDYDXChainHTTPClient(conf DYDXChainHTTPClientConfig) *DYDXChainHTTPClient {
	return &DYDXChainHTTPClient{
		conf:   conf,
		client: http.DefaultClient,
	}
}

func (c *DYDXChainHTTPClient) Broadcast(ctx context.Context, request DYDXChainRequest) (string, error) {
	if c.conf.URL == "" {
		return "", ErrDYDXChainClientNotConfigured
	}

	raw, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.conf.URL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
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
			result.Msg = fmt.Sprintf("upstream returned code %d", result.Code)
		}
		return "", errors.New(result.Msg)
	}
	if result.Data.TxHash == "" {
		return "", errors.New("upstream response missing txHash")
	}
	return result.Data.TxHash, nil
}

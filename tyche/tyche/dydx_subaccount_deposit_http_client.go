package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type DYDXSubaccountDepositHTTPClientConfig struct {
	URL         string
	BearerToken string
}

type DYDXSubaccountDepositHTTPClient struct {
	conf   DYDXSubaccountDepositHTTPClientConfig
	client *http.Client
}

func NewDYDXSubaccountDepositHTTPClient(conf DYDXSubaccountDepositHTTPClientConfig) *DYDXSubaccountDepositHTTPClient {
	return &DYDXSubaccountDepositHTTPClient{conf: conf, client: http.DefaultClient}
}

func (c *DYDXSubaccountDepositHTTPClient) DepositToSubaccount(ctx context.Context, request DYDXSubaccountDepositRequest) (DYDXSubaccountDepositResult, error) {
	if c.conf.URL == "" {
		return DYDXSubaccountDepositResult{}, ErrDYDXChainClientNotConfigured
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return DYDXSubaccountDepositResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.conf.URL, bytes.NewReader(raw))
	if err != nil {
		return DYDXSubaccountDepositResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.conf.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.conf.BearerToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return DYDXSubaccountDepositResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return DYDXSubaccountDepositResult{}, fmt.Errorf("dydx subaccount deposit upstream returned http %d", resp.StatusCode)
	}
	var result struct {
		Code int                         `json:"code"`
		Msg  string                      `json:"msg"`
		Data DYDXSubaccountDepositResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return DYDXSubaccountDepositResult{}, err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("dydx subaccount deposit upstream returned code %d", result.Code)
		}
		return DYDXSubaccountDepositResult{}, errors.New(result.Msg)
	}
	if result.Data.Status == "" {
		result.Data.Status = TradingOrderStatusBroadcasted
	}
	if result.Data.AssetID == 0 {
		result.Data.AssetID = request.AssetID
	}
	if result.Data.Quantums == "" {
		result.Data.Quantums = request.Quantums
	}
	if result.Data.DYDXAccount == "" {
		result.Data.DYDXAccount = request.DYDXAccount
	}
	return result.Data, nil
}

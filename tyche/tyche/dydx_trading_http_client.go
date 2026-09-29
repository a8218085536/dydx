package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var dydxBroadcastCode19Pattern = regexp.MustCompile(`\bcode\s+19\b`)

type DYDXTradingHTTPClientConfig struct {
	URL         string
	Account     string
	BearerToken string
}

type DYDXPlaceOrder struct {
	ID               string `json:"id"`
	UserID           string `json:"userId"`
	DYDXAccount      string `json:"dydxAccount,omitempty"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
	ClientOrderID    string `json:"clientOrderId"`
	Market           string `json:"market"`
	Side             string `json:"side"`
	OrderType        string `json:"orderType"`
	Price            string `json:"price"`
	Size             string `json:"size"`
	MarginAsset      string `json:"marginAsset"`
	MarginAmount     string `json:"marginAmount"`
	ReduceOnly       bool   `json:"reduceOnly,omitempty"`
}

type DYDXCancelOrder struct {
	ID               string `json:"id"`
	UserID           string `json:"userId"`
	DYDXAccount      string `json:"dydxAccount,omitempty"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
	OrderID          string `json:"orderId"`
	ClientOrderID    string `json:"clientOrderId"`
	Market           string `json:"market"`
}

type DYDXTradingResult struct {
	TxHash string `json:"txHash"`
	Status string `json:"status"`
}

type DYDXTradingHTTPClient struct {
	conf   DYDXTradingHTTPClientConfig
	client *http.Client
}

func NewDYDXTradingHTTPClient(conf DYDXTradingHTTPClientConfig) *DYDXTradingHTTPClient {
	return &DYDXTradingHTTPClient{conf: conf, client: http.DefaultClient}
}

func (c *DYDXTradingHTTPClient) PlaceOrder(ctx context.Context, order DYDXPlaceOrder) (DYDXTradingResult, error) {
	return c.post(ctx, order.ID, DYDXActionPlaceOrder, firstNonEmpty(order.DYDXAccount, c.conf.Account), map[string]interface{}{"order": order})
}

func (c *DYDXTradingHTTPClient) CancelOrder(ctx context.Context, order DYDXCancelOrder) (DYDXTradingResult, error) {
	return c.post(ctx, order.ID, DYDXActionCancelOrder, firstNonEmpty(order.DYDXAccount, c.conf.Account), map[string]interface{}{"order": order})
}

func (c *DYDXTradingHTTPClient) SubmitCosmosTx(ctx context.Context, tx DYDXCosmosTxRequest) (DYDXTradingResult, error) {
	return c.post(ctx, tx.ID, tx.Action, firstNonEmpty(tx.Account, c.conf.Account), map[string]interface{}{DYDXCosmosTxParamKey: tx})
}

func (c *DYDXTradingHTTPClient) post(ctx context.Context, id, action, account string, params interface{}) (DYDXTradingResult, error) {
	if c.conf.URL == "" {
		return DYDXTradingResult{}, ErrDYDXChainClientNotConfigured
	}
	body := map[string]interface{}{
		"id":      id,
		"action":  action,
		"account": account,
		"params":  params,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return DYDXTradingResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.conf.URL, bytes.NewReader(raw))
	if err != nil {
		return DYDXTradingResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.conf.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.conf.BearerToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return DYDXTradingResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return DYDXTradingResult{}, fmt.Errorf("dydx trading upstream returned http %d", resp.StatusCode)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TxHash string `json:"txHash"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return DYDXTradingResult{}, err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("dydx trading upstream returned code %d", result.Code)
		}
		if isDYDXAlreadyInMempoolMessage(result.Msg) {
			status := result.Data.Status
			if status == "" {
				status = TradingOrderStatusBroadcasted
			}
			return DYDXTradingResult{TxHash: result.Data.TxHash, Status: status}, nil
		}
		return DYDXTradingResult{}, errors.New(result.Msg)
	}
	status := result.Data.Status
	if status == "" {
		status = TradingOrderStatusBroadcasted
	}
	return DYDXTradingResult{TxHash: result.Data.TxHash, Status: status}, nil
}

func isDYDXAlreadyInMempoolMessage(message string) bool {
	normalized := strings.ToLower(message)
	return dydxBroadcastCode19Pattern.MatchString(normalized)
}

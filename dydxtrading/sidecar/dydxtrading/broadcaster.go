//go:build cgo

package dydxtrading

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type RESTBroadcaster struct {
	endpoint string
	client   *http.Client
}

func NewRESTBroadcaster(endpoint string, client *http.Client) RESTBroadcaster {
	if client == nil {
		client = http.DefaultClient
	}
	return RESTBroadcaster{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   client,
	}
}

func (b RESTBroadcaster) Broadcast(ctx context.Context, req BroadcastRequest) (BroadcastResult, error) {
	if b.endpoint == "" {
		return BroadcastResult{}, fmt.Errorf("missing broadcast endpoint")
	}
	mode := req.Mode
	if mode == "" {
		mode = BroadcastModeSync
	}
	payload := struct {
		TxBytes string `json:"tx_bytes"`
		Mode    string `json:"mode"`
	}{
		TxBytes: base64.StdEncoding.EncodeToString(req.TxBytes),
		Mode:    mode,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return BroadcastResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
	if err != nil {
		return BroadcastResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := b.client.Do(httpReq)
	if err != nil {
		return BroadcastResult{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return BroadcastResult{}, fmt.Errorf("broadcast returned http %d", httpResp.StatusCode)
	}

	var response struct {
		TxResponse struct {
			TxHash string `json:"txhash"`
			Code   uint32 `json:"code"`
			RawLog string `json:"raw_log"`
		} `json:"tx_response"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return BroadcastResult{}, err
	}
	result := BroadcastResult{
		TxHash: response.TxResponse.TxHash,
		Code:   response.TxResponse.Code,
		RawLog: response.TxResponse.RawLog,
	}
	if result.Code != 0 {
		return result, fmt.Errorf("broadcast rejected with code %d: %s", result.Code, result.RawLog)
	}
	return result, nil
}

type RESTAccountProvider struct {
	baseURL string
	client  *http.Client
}

func NewRESTAccountProvider(baseURL string, client *http.Client) RESTAccountProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return RESTAccountProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (p RESTAccountProvider) Account(ctx context.Context, address string) (AccountInfo, error) {
	if p.baseURL == "" {
		return AccountInfo{}, fmt.Errorf("missing account REST base URL")
	}
	endpoint := p.baseURL + "/cosmos/auth/v1beta1/accounts/" + url.PathEscape(address)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return AccountInfo{}, err
	}
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return AccountInfo{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return AccountInfo{}, fmt.Errorf("account query returned http %d", httpResp.StatusCode)
	}

	var response map[string]any
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return AccountInfo{}, err
	}
	accountNumber, ok := findUint64(response, "account_number")
	if !ok {
		return AccountInfo{}, fmt.Errorf("account_number not found in account response")
	}
	sequence, ok := findUint64(response, "sequence")
	if !ok {
		return AccountInfo{}, fmt.Errorf("sequence not found in account response")
	}
	return AccountInfo{
		AccountNumber: accountNumber,
		Sequence:      sequence,
	}, nil
}

type StaticAccountProvider struct {
	AccountNumber uint64
	Sequence      uint64
}

func (p StaticAccountProvider) Account(context.Context, string) (AccountInfo, error) {
	return AccountInfo{
		AccountNumber: p.AccountNumber,
		Sequence:      p.Sequence,
	}, nil
}

func findUint64(value any, key string) (uint64, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if found, ok := typed[key]; ok {
			parsed, err := parseUint64(found)
			if err == nil {
				return parsed, true
			}
		}
		for _, nested := range typed {
			if parsed, ok := findUint64(nested, key); ok {
				return parsed, true
			}
		}
	case []any:
		for _, nested := range typed {
			if parsed, ok := findUint64(nested, key); ok {
				return parsed, true
			}
		}
	}
	return 0, false
}

func parseUint64(value any) (uint64, error) {
	switch typed := value.(type) {
	case string:
		return strconv.ParseUint(typed, 10, 64)
	case float64:
		if typed < 0 || typed != float64(uint64(typed)) {
			return 0, fmt.Errorf("invalid integer value %v", typed)
		}
		return uint64(typed), nil
	case json.Number:
		return strconv.ParseUint(typed.String(), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported integer value %T", value)
	}
}

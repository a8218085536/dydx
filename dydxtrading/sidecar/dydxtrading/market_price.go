package dydxtrading

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

type MarketPriceRequest struct {
	MarketID uint32 `json:"marketId"`
	Market   string `json:"market,omitempty"`
}

type MarketPriceSnapshot struct {
	MarketID   uint32 `json:"marketId"`
	Market     string `json:"market,omitempty"`
	Price      string `json:"price"`
	Source     string `json:"source,omitempty"`
	SnapshotID string `json:"snapshotId,omitempty"`
	RawPrice   string `json:"rawPrice,omitempty"`
	Exponent   int32  `json:"exponent,omitempty"`
}

type MarketPriceProvider interface {
	MarketPrice(context.Context, MarketPriceRequest) (MarketPriceSnapshot, error)
}

type RESTMarketPriceProvider struct {
	baseURL string
	client  *http.Client
}

func NewRESTMarketPriceProvider(baseURL string, client *http.Client) RESTMarketPriceProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return RESTMarketPriceProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (p RESTMarketPriceProvider) MarketPrice(ctx context.Context, request MarketPriceRequest) (MarketPriceSnapshot, error) {
	if p.baseURL == "" {
		return MarketPriceSnapshot{}, fmt.Errorf("missing market price REST base URL")
	}
	endpoint := p.baseURL + "/dydxprotocol/prices/market/" + strconv.FormatUint(uint64(request.MarketID), 10)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return MarketPriceSnapshot{}, err
	}
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return MarketPriceSnapshot{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return MarketPriceSnapshot{}, fmt.Errorf("market price query returned http %d", httpResp.StatusCode)
	}
	var response struct {
		MarketPrice struct {
			ID       uint32 `json:"id"`
			Exponent int32  `json:"exponent"`
			Price    string `json:"price"`
		} `json:"market_price"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return MarketPriceSnapshot{}, err
	}
	marketID := response.MarketPrice.ID
	if marketID == 0 {
		marketID = request.MarketID
	}
	price, err := marketPriceDecimal(response.MarketPrice.Price, response.MarketPrice.Exponent)
	if err != nil {
		return MarketPriceSnapshot{}, err
	}
	return MarketPriceSnapshot{
		MarketID:   marketID,
		Market:     request.Market,
		Price:      price,
		Source:     "dydx-rest",
		SnapshotID: fmt.Sprintf("dydx-market-price:%d:%s:%d", marketID, response.MarketPrice.Price, response.MarketPrice.Exponent),
		RawPrice:   response.MarketPrice.Price,
		Exponent:   response.MarketPrice.Exponent,
	}, nil
}

func marketPriceDecimal(raw string, exponent int32) (string, error) {
	raw = strings.TrimSpace(raw)
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() <= 0 {
		return "", fmt.Errorf("invalid market price")
	}
	price := new(big.Rat).SetInt(value)
	if exponent < 0 {
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-exponent)), nil)
		price.Quo(price, new(big.Rat).SetInt(scale))
	} else if exponent > 0 {
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
		price.Mul(price, new(big.Rat).SetInt(scale))
	}
	out := price.FloatString(18)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" {
		return "0", nil
	}
	return out, nil
}

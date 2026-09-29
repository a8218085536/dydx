package tyche

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type DYDXHTTPMarketPriceSourceConfig struct {
	URL     string             `json:"url" toml:"url"`
	APIKey  string             `json:"apiKey" toml:"apiKey"`
	Markets []DYDXMarketConfig `json:"markets" toml:"markets"`
	Client  *http.Client       `json:"-" toml:"-"`
}

type DYDXHTTPMarketPriceSource struct {
	conf    DYDXHTTPMarketPriceSourceConfig
	client  *http.Client
	markets map[string]DYDXMarketConfig
}

func NewDYDXHTTPMarketPriceSource(conf DYDXHTTPMarketPriceSourceConfig) *DYDXHTTPMarketPriceSource {
	client := conf.Client
	if client == nil {
		client = http.DefaultClient
	}
	source := &DYDXHTTPMarketPriceSource{
		conf:    conf,
		client:  client,
		markets: map[string]DYDXMarketConfig{},
	}
	for _, market := range conf.Markets {
		source.markets[normalizeDYDXMarket(market.Market)] = market
	}
	return source
}

func (s *DYDXHTTPMarketPriceSource) MarketPrice(ctx context.Context, request TradingMarketPriceRequest) (TradingMarketPriceSnapshot, error) {
	if s == nil || strings.TrimSpace(s.conf.URL) == "" {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceSourceNotConfigured
	}
	market, ok := s.markets[normalizeDYDXMarket(request.Market)]
	if !ok {
		return TradingMarketPriceSnapshot{}, ErrDYDXMarketNotConfigured
	}
	endpoint, err := url.Parse(s.conf.URL)
	if err != nil {
		return TradingMarketPriceSnapshot{}, err
	}
	query := endpoint.Query()
	query.Set("market", market.Market)
	query.Set("marketId", strconv.FormatUint(uint64(market.ClobPairID), 10))
	query.Set("clobPairId", strconv.FormatUint(uint64(market.ClobPairID), 10))
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return TradingMarketPriceSnapshot{}, err
	}
	if s.conf.APIKey != "" {
		httpReq.Header.Set("X-API-Key", s.conf.APIKey)
		httpReq.Header.Set("Authorization", "Bearer "+s.conf.APIKey)
	}
	httpResp, err := s.client.Do(httpReq)
	if err != nil {
		return TradingMarketPriceSnapshot{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return TradingMarketPriceSnapshot{}, fmt.Errorf("dydx market price upstream returned http %d", httpResp.StatusCode)
	}
	var response struct {
		Code int                        `json:"code"`
		Msg  string                     `json:"msg"`
		Data TradingMarketPriceSnapshot `json:"data"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&response); err != nil {
		return TradingMarketPriceSnapshot{}, err
	}
	if response.Code != http.StatusOK {
		if response.Msg == "" {
			response.Msg = fmt.Sprintf("dydx market price upstream returned code %d", response.Code)
		}
		if response.Code == http.StatusNotFound {
			return TradingMarketPriceSnapshot{}, errors.Join(ErrTradingMarketPriceNotFound, errors.New(response.Msg))
		}
		return TradingMarketPriceSnapshot{}, errors.New(response.Msg)
	}
	if response.Data.Price == "" {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceInvalid
	}
	if _, ok := parseDecimalRat(response.Data.Price); !ok {
		return TradingMarketPriceSnapshot{}, ErrTradingMarketPriceInvalid
	}
	if response.Data.Market == "" {
		response.Data.Market = market.Market
	}
	if response.Data.MarketID == 0 {
		response.Data.MarketID = market.ClobPairID
	}
	if response.Data.Source == "" {
		response.Data.Source = "dydxtrading"
	}
	if response.Data.SnapshotID == "" {
		response.Data.SnapshotID = fmt.Sprintf("dydx-market-price:%d:%s", response.Data.MarketID, response.Data.Price)
	}
	return response.Data, nil
}

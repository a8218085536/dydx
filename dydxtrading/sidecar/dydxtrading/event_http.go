package dydxtrading

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type HTTPConfig struct {
	BearerToken         string
	EventAPIKey         string
	EventProvider       TradingEventProvider
	EventIngestor       TradingEventIngestor
	IngestEventKey      string
	WalletProvisioner   ProxyWalletProvisioner
	MarketPriceProvider MarketPriceProvider
	SubaccountDepositor SubaccountDepositService
	ReconcileProvider   OrderReconcileProvider
}

type tradingEventResponse struct {
	Code       int    `json:"code"`
	Msg        string `json:"msg,omitempty"`
	NextCursor string `json:"nextCursor,omitempty"`
	Data       any    `json:"data,omitempty"`
}

func RegisterTradingEventRoutes(mux *http.ServeMux, cfg HTTPConfig) {
	eventProvider := cfg.EventProvider
	if eventProvider == nil {
		eventProvider = EmptyTradingEventProvider{}
	}
	eventIngestor := cfg.EventIngestor
	mux.HandleFunc("/fills", func(w http.ResponseWriter, r *http.Request) {
		handleTradingEventEndpoint(w, r, cfg, func() (tradingEventResponse, error) {
			page, err := eventProvider.Fills(r.Context(), r.URL.Query().Get("cursor"))
			return tradingEventResponse{Code: http.StatusOK, NextCursor: page.NextCursor, Data: page.Data}, err
		})
	})
	mux.HandleFunc("/order-statuses", func(w http.ResponseWriter, r *http.Request) {
		handleTradingEventEndpoint(w, r, cfg, func() (tradingEventResponse, error) {
			page, err := eventProvider.OrderStatuses(r.Context(), r.URL.Query().Get("cursor"))
			return tradingEventResponse{Code: http.StatusOK, NextCursor: page.NextCursor, Data: page.Data}, err
		})
	})
	mux.HandleFunc("/positions", func(w http.ResponseWriter, r *http.Request) {
		handleTradingEventEndpoint(w, r, cfg, func() (tradingEventResponse, error) {
			page, err := eventProvider.Positions(r.Context(), r.URL.Query().Get("cursor"))
			return tradingEventResponse{Code: http.StatusOK, NextCursor: page.NextCursor, Data: page.Data}, err
		})
	})
	mux.HandleFunc("/dydx/events/ingest", func(w http.ResponseWriter, r *http.Request) {
		handleTradingEventIngestEndpoint(w, r, cfg, eventIngestor)
	})
	RegisterMarketPriceRoutes(mux, cfg)
	RegisterReconcileRoutes(mux, cfg)
}

func NewTradingEventHTTPHandler(cfg HTTPConfig) http.Handler {
	mux := http.NewServeMux()
	RegisterTradingEventRoutes(mux, cfg)
	return mux
}

func RegisterMarketPriceRoutes(mux *http.ServeMux, cfg HTTPConfig) {
	mux.HandleFunc("/prices", func(w http.ResponseWriter, r *http.Request) {
		handleTradingEventEndpoint(w, r, cfg, func() (tradingEventResponse, error) {
			if cfg.MarketPriceProvider == nil {
				return tradingEventResponse{
					Code: http.StatusServiceUnavailable,
					Msg:  "market price provider not configured",
				}, nil
			}
			marketIDValue := r.URL.Query().Get("marketId")
			if marketIDValue == "" {
				marketIDValue = r.URL.Query().Get("clobPairId")
			}
			if marketIDValue == "" {
				return tradingEventResponse{
					Code: http.StatusBadRequest,
					Msg:  "marketId is required",
				}, nil
			}
			marketID, err := strconv.ParseUint(marketIDValue, 10, 32)
			if err != nil {
				return tradingEventResponse{
					Code: http.StatusBadRequest,
					Msg:  "marketId is invalid",
				}, nil
			}
			snapshot, err := cfg.MarketPriceProvider.MarketPrice(r.Context(), MarketPriceRequest{
				MarketID: uint32(marketID),
				Market:   r.URL.Query().Get("market"),
			})
			return tradingEventResponse{
				Code: http.StatusOK,
				Msg:  "succ",
				Data: snapshot,
			}, err
		})
	})
}

func RegisterReconcileRoutes(mux *http.ServeMux, cfg HTTPConfig) {
	provider := cfg.ReconcileProvider
	if provider == nil {
		provider = EmptyOrderReconcileProvider{}
	}
	mux.HandleFunc("/reconcile/order", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusMethodNotAllowed,
				Msg:  "method not allowed",
			})
			return
		}
		if !authorizedTradingEventRequest(r, cfg) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusUnauthorized,
				Msg:  "unauthorized",
			})
			return
		}
		var req OrderReconcileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusBadRequest,
				Msg:  err.Error(),
			})
			return
		}
		result, err := provider.ReconcileOrder(r.Context(), req)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusInternalServerError,
				Msg:  err.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusOK,
			Msg:  "succ",
			Data: result,
		})
	})
}

func handleTradingEventIngestEndpoint(w http.ResponseWriter, r *http.Request, cfg HTTPConfig, ingestor TradingEventIngestor) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusMethodNotAllowed,
			Msg:  "method not allowed",
		})
		return
	}
	if !authorizedTradingEventIngestRequest(r, cfg) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusUnauthorized,
			Msg:  "unauthorized",
		})
		return
	}
	if ingestor == nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusServiceUnavailable,
			Msg:  "event ingestor not configured",
		})
		return
	}
	var batch TradingEventBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusBadRequest,
			Msg:  err.Error(),
		})
		return
	}
	if err := ingestor.Ingest(r.Context(), batch); err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusBadRequest,
			Msg:  err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tradingEventResponse{Code: http.StatusOK})
}

func handleTradingEventEndpoint(w http.ResponseWriter, r *http.Request, cfg HTTPConfig, load func() (tradingEventResponse, error)) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusMethodNotAllowed,
			Msg:  "method not allowed",
		})
		return
	}
	if !authorizedTradingEventRequest(r, cfg) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusUnauthorized,
			Msg:  "unauthorized",
		})
		return
	}
	resp, err := load()
	if err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tradingEventResponse{
			Code: http.StatusInternalServerError,
			Msg:  err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func authorizedTradingEventRequest(r *http.Request, cfg HTTPConfig) bool {
	key := cfg.EventAPIKey
	if key == "" {
		key = cfg.BearerToken
	}
	if key == "" {
		return true
	}
	return r.Header.Get("X-API-Key") == key || bearerToken(r) == key
}

func authorizedTradingEventIngestRequest(r *http.Request, cfg HTTPConfig) bool {
	key := cfg.IngestEventKey
	if key == "" {
		key = cfg.EventAPIKey
	}
	if key == "" {
		key = cfg.BearerToken
	}
	if key == "" {
		return true
	}
	return r.Header.Get("X-API-Key") == key || bearerToken(r) == key
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if value == "" {
		return ""
	}
	prefix := "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

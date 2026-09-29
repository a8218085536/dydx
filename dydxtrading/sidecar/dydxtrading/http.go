//go:build cgo

package dydxtrading

import (
	"encoding/json"
	"net/http"
)

func NewHTTPHandler(service *Service, cfg HTTPConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/wallet/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusMethodNotAllowed,
				Msg:  "method not allowed",
			})
			return
		}
		if cfg.BearerToken != "" && bearerToken(r) != cfg.BearerToken {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusUnauthorized,
				Msg:  "unauthorized",
			})
			return
		}
		if cfg.WalletProvisioner == nil {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusServiceUnavailable,
				Msg:  "wallet provisioner not configured",
			})
			return
		}
		var req ProxyWalletCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusBadRequest,
				Msg:  err.Error(),
			})
			return
		}
		wallet, err := cfg.WalletProvisioner.CreateOrGetProxyWallet(r.Context(), req)
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
			Data: wallet,
		})
	})
	mux.HandleFunc("/dydx/trading", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(TycheTradingResponse{
				Code: http.StatusMethodNotAllowed,
				Msg:  "method not allowed",
			})
			return
		}
		if cfg.BearerToken != "" && bearerToken(r) != cfg.BearerToken {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(TycheTradingResponse{
				Code: http.StatusUnauthorized,
				Msg:  "unauthorized",
			})
			return
		}
		var req TycheTradingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(TycheTradingResponse{
				Code: http.StatusBadRequest,
				Msg:  err.Error(),
			})
			return
		}
		resp, err := service.Submit(r.Context(), req)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(TycheTradingResponse{
				Code: http.StatusInternalServerError,
				Msg:  err.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/subaccount/deposit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusMethodNotAllowed,
				Msg:  "method not allowed",
			})
			return
		}
		if cfg.BearerToken != "" && bearerToken(r) != cfg.BearerToken {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusUnauthorized,
				Msg:  "unauthorized",
			})
			return
		}
		if cfg.SubaccountDepositor == nil {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusServiceUnavailable,
				Msg:  "subaccount depositor not configured",
			})
			return
		}
		var req SubaccountDepositRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(tradingEventResponse{
				Code: http.StatusBadRequest,
				Msg:  err.Error(),
			})
			return
		}
		result, err := cfg.SubaccountDepositor.DepositToSubaccount(r.Context(), req)
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
	RegisterTradingEventRoutes(mux, cfg)
	return mux
}

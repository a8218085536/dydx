package tyche

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

type MockDYDXSidecarConfig struct {
	TxHash                string
	SubmitStatus          string
	RejectReason          string
	WalletAddressPrefix   string
	Fills                 []TradingFillSyncRequest
	OrderStatuses         []TradingOrderStatusSyncRequest
	Positions             []TradingPositionSyncRequest
	NextFillsCursor       string
	NextOrderStatusCursor string
	NextPositionsCursor   string
	MarketPrices          map[uint32]string
}

type MockDYDXSidecar struct {
	conf     MockDYDXSidecarConfig
	mu       sync.Mutex
	requests []DYDXChainRequest
}

func NewMockDYDXSidecar(conf MockDYDXSidecarConfig) *MockDYDXSidecar {
	return &MockDYDXSidecar{conf: conf}
}

func (s *MockDYDXSidecar) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/wallet/create", s.handleWalletCreate)
	mux.HandleFunc("/dydx/trading", s.handleTrading)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/fills", s.handleFills)
	mux.HandleFunc("/order-statuses", s.handleOrderStatuses)
	mux.HandleFunc("/positions", s.handlePositions)
	mux.HandleFunc("/prices", s.handlePrices)
	return mux
}

func (s *MockDYDXSidecar) Requests() []DYDXChainRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]DYDXChainRequest{}, s.requests...)
}

func (s *MockDYDXSidecar) handleWalletCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeMockResponse(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var request struct {
		UserID      string `json:"userId"`
		NECOAddress string `json:"necoAddress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeMockResponse(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if request.UserID == "" {
		writeMockResponse(w, http.StatusBadRequest, "userId is required", nil)
		return
	}
	prefix := s.conf.WalletAddressPrefix
	if prefix == "" {
		prefix = "dydx1mock"
	}
	writeMockResponse(w, http.StatusOK, "succ", map[string]string{
		"userId":      request.UserID,
		"necoAddress": request.NECOAddress,
		"dydxAddress": prefix + request.UserID,
	})
}

func (s *MockDYDXSidecar) handleTrading(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeMockResponse(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var request DYDXChainRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeMockResponse(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := validateMockCosmosTx(request); err != nil {
		writeMockResponse(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()

	status := s.conf.SubmitStatus
	if status == "" {
		status = TradingOrderStatusSubmitted
	}
	txHash := s.conf.TxHash
	if txHash == "" {
		txHash = "mock-dydx-tx"
	}
	writeMockResponse(w, http.StatusOK, "succ", map[string]string{
		"txHash": txHash,
		"status": status,
		"reason": s.conf.RejectReason,
	})
}

func (s *MockDYDXSidecar) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		writeMockResponse(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"result": map[string]interface{}{
			"sync_info": map[string]string{
				"latest_block_height": "100",
				"latest_block_time":   "2026-08-17T03:12:00Z",
			},
		},
	})
}

func (s *MockDYDXSidecar) handleFills(w http.ResponseWriter, r *http.Request) {
	writeMockEventResponse(w, s.conf.Fills, s.conf.NextFillsCursor)
}

func (s *MockDYDXSidecar) handleOrderStatuses(w http.ResponseWriter, r *http.Request) {
	writeMockEventResponse(w, s.conf.OrderStatuses, s.conf.NextOrderStatusCursor)
}

func (s *MockDYDXSidecar) handlePositions(w http.ResponseWriter, r *http.Request) {
	writeMockEventResponse(w, s.conf.Positions, s.conf.NextPositionsCursor)
}

func (s *MockDYDXSidecar) handlePrices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		writeMockResponse(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	marketIDValue := r.URL.Query().Get("marketId")
	if marketIDValue == "" {
		marketIDValue = r.URL.Query().Get("clobPairId")
	}
	if marketIDValue == "" {
		writeMockResponse(w, http.StatusBadRequest, "marketId is required", nil)
		return
	}
	parsed, err := strconv.ParseUint(marketIDValue, 10, 32)
	if err != nil {
		writeMockResponse(w, http.StatusBadRequest, "marketId is invalid", nil)
		return
	}
	marketID := uint32(parsed)
	price := "2500"
	if s.conf.MarketPrices != nil {
		var ok bool
		price, ok = s.conf.MarketPrices[marketID]
		if !ok {
			writeMockResponse(w, http.StatusNotFound, "market price not found", nil)
			return
		}
	}
	writeMockResponse(w, http.StatusOK, "succ", TradingMarketPriceSnapshot{
		MarketID:   marketID,
		Market:     r.URL.Query().Get("market"),
		Price:      price,
		Source:     "mock-dydx-sidecar",
		SnapshotID: fmt.Sprintf("mock-dydx-market-price:%d:%s", marketID, price),
	})
}

func validateMockCosmosTx(request DYDXChainRequest) error {
	raw, ok := request.Params[DYDXCosmosTxParamKey]
	if !ok {
		return fmt.Errorf("missing %s", DYDXCosmosTxParamKey)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var tx DYDXCosmosTxRequest
	if err := json.Unmarshal(encoded, &tx); err != nil {
		return fmt.Errorf("decode %s: %w", DYDXCosmosTxParamKey, err)
	}
	if tx.ID == "" || tx.Action == "" || tx.Account == "" || tx.ChainID == "" {
		return fmt.Errorf("invalid %s identity", DYDXCosmosTxParamKey)
	}
	switch tx.Message.TypeURL {
	case DYDXCosmosMsgPlaceOrderTypeURL, DYDXCosmosMsgCancelOrderTypeURL:
		return nil
	default:
		return fmt.Errorf("unsupported cosmosTx typeUrl %q", tx.Message.TypeURL)
	}
}

func writeMockEventResponse(w http.ResponseWriter, data interface{}, nextCursor string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":       http.StatusOK,
		"msg":        "succ",
		"nextCursor": nextCursor,
		"data":       data,
	})
}

func writeMockResponse(w http.ResponseWriter, code int, msg string, data interface{}) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code": code,
		"msg":  msg,
		"data": data,
	})
}

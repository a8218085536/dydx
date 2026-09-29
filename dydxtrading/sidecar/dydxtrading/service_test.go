//go:build cgo

package dydxtrading

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	appconfig "github.com/dydxprotocol/v4-chain/protocol/app/config"
	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"
	sendingtypes "github.com/dydxprotocol/v4-chain/protocol/x/sending/types"
	"github.com/stretchr/testify/require"
)

func TestBuildMessageFromTychePlaceOrder(t *testing.T) {
	account := testAccount()
	req := testPlaceOrderRequest(account)
	req.Params.CosmosTx.GasLimit = 700000
	req.Params.CosmosTx.FeeAmount = "12345adedx"

	msg, meta, err := BuildMessage(req)

	require.NoError(t, err)
	require.Equal(t, ActionPlaceOrder, meta.Action)
	require.Equal(t, "dydx-local-1", meta.ChainID)
	require.Equal(t, uint64(700000), meta.GasLimit)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("adedx", 12345)), meta.FeeAmount)
	placeOrder, ok := msg.(*clobtypes.MsgPlaceOrder)
	require.True(t, ok)
	require.Equal(t, account, placeOrder.Order.OrderId.SubaccountId.Owner)
	require.Equal(t, uint32(0), placeOrder.Order.OrderId.SubaccountId.Number)
	require.Equal(t, uint32(123), placeOrder.Order.OrderId.ClientId)
	require.Equal(t, uint32(7), placeOrder.Order.OrderId.ClobPairId)
	require.Equal(t, uint64(1_250_000), placeOrder.Order.Quantums)
	require.Equal(t, uint64(2_500_500_000), placeOrder.Order.Subticks)
	require.Equal(t, uint32(120), placeOrder.Order.GetGoodTilBlock())
	require.Equal(t, clobtypes.Order_SIDE_BUY, placeOrder.Order.Side)
	require.Equal(t, clobtypes.Order_TIME_IN_FORCE_UNSPECIFIED, placeOrder.Order.TimeInForce)
	require.NoError(t, placeOrder.ValidateBasic())
}

func TestBuildMessageFromTycheLongTermPlaceOrder(t *testing.T) {
	account := testAccount()
	req := testPlaceOrderRequest(account)
	setCosmosMessageValue(t, &req, map[string]any{
		"order": map[string]any{
			"orderId": map[string]any{
				"subaccountId": map[string]any{
					"owner":  account,
					"number": 0,
				},
				"clientId":   123,
				"orderFlags": uint32(64),
				"clobPairId": 7,
			},
			"side":             1,
			"quantums":         1_250_000,
			"subticks":         2_500_500_000,
			"goodTilBlockTime": 1786903600,
			"timeInForce":      0,
			"reduceOnly":       false,
			"clientMetadata":   0,
		},
	})

	msg, _, err := BuildMessage(req)

	require.NoError(t, err)
	placeOrder, ok := msg.(*clobtypes.MsgPlaceOrder)
	require.True(t, ok)
	require.Equal(t, uint32(64), placeOrder.Order.OrderId.OrderFlags)
	require.Equal(t, uint32(1786903600), placeOrder.Order.GetGoodTilBlockTime())
	require.Equal(t, uint32(0), placeOrder.Order.GetGoodTilBlock())
	require.NoError(t, placeOrder.ValidateBasic())
}

func TestBuildMessageFromTycheCancelOrder(t *testing.T) {
	account := testAccount()
	req := testCancelOrderRequest(account)

	msg, meta, err := BuildMessage(req)

	require.NoError(t, err)
	require.Equal(t, ActionCancelOrder, meta.Action)
	cancelOrder, ok := msg.(*clobtypes.MsgCancelOrder)
	require.True(t, ok)
	require.Equal(t, account, cancelOrder.OrderId.SubaccountId.Owner)
	require.Equal(t, uint32(123), cancelOrder.OrderId.ClientId)
	require.Equal(t, uint32(7), cancelOrder.OrderId.ClobPairId)
	require.Equal(t, uint32(130), cancelOrder.GetGoodTilBlock())
	require.NoError(t, cancelOrder.ValidateBasic())
}

func TestBuildMessageFromTycheLongTermCancelOrder(t *testing.T) {
	account := testAccount()
	req := testCancelOrderRequest(account)
	setCosmosMessageValue(t, &req, map[string]any{
		"orderId": map[string]any{
			"subaccountId": map[string]any{
				"owner":  account,
				"number": 0,
			},
			"clientId":   123,
			"orderFlags": uint32(64),
			"clobPairId": 7,
		},
		"goodTilBlockTime": 1786903600,
	})

	msg, _, err := BuildMessage(req)

	require.NoError(t, err)
	cancelOrder, ok := msg.(*clobtypes.MsgCancelOrder)
	require.True(t, ok)
	require.Equal(t, uint32(64), cancelOrder.OrderId.OrderFlags)
	require.Equal(t, uint32(1786903600), cancelOrder.GetGoodTilBlockTime())
	require.Equal(t, uint32(0), cancelOrder.GetGoodTilBlock())
	require.NoError(t, cancelOrder.ValidateBasic())
}

func TestServiceSignsAndBroadcastsTycheRequest(t *testing.T) {
	account := testAccount()
	signer := &recordingSigner{txBytes: []byte{9, 8, 7}}
	broadcaster := &recordingBroadcaster{txHash: "ABC123"}
	service := NewService(ServiceConfig{
		Signer: signer,
		AccountProvider: StaticAccountProvider{
			AccountNumber: 44,
			Sequence:      9,
		},
		Broadcaster: broadcaster,
		PrivateKeyProvider: staticPrivateKeyProvider{
			account: fakePrivKey{},
		},
	})

	resp, err := service.Submit(context.Background(), testPlaceOrderRequest(account))

	require.NoError(t, err)
	require.Equal(t, 200, resp.Code)
	require.Equal(t, "ABC123", resp.Data.TxHash)
	require.Equal(t, StatusBroadcasted, resp.Data.Status)
	require.Equal(t, BroadcastModeSync, broadcaster.mode)
	require.NotEmpty(t, broadcaster.txBytes)
	require.Equal(t, "dydx-local-1", signer.req.ChainID)
	require.Equal(t, uint64(44), signer.req.AccountNumber)
	require.Equal(t, uint64(9), signer.req.Sequence)
	require.Equal(t, account, signer.req.SignerAddress)
	require.Equal(t, DefaultTxGasLimit, signer.req.GasLimit)
	require.Equal(t, defaultTxFeeAmount(nil), signer.req.FeeAmount)
}

func TestServiceTreatsAlreadyInMempoolBroadcastAsBroadcasted(t *testing.T) {
	account := testAccount()
	sequences := NewSequenceManager(&countingAccountProvider{info: AccountInfo{
		AccountNumber: 44,
		Sequence:      9,
	}})
	service := NewService(ServiceConfig{
		Signer:          &recordingSigner{txBytes: []byte{9, 8, 7}},
		AccountProvider: sequences,
		Broadcaster: &recordingBroadcaster{
			txHash: "ABC123",
			code:   BroadcastCodeTxAlreadyInMempool,
			err:    errors.New("broadcast rejected with code 19: tx already in mempool"),
		},
		PrivateKeyProvider: staticPrivateKeyProvider{
			account: fakePrivKey{},
		},
	})

	resp, err := service.Submit(context.Background(), testPlaceOrderRequest(account))

	require.NoError(t, err)
	require.Equal(t, 200, resp.Code)
	require.Equal(t, "ABC123", resp.Data.TxHash)
	require.Equal(t, StatusBroadcasted, resp.Data.Status)
	next, err := sequences.Account(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, uint64(9), next.Sequence)
}

func TestServiceReleasesSequenceAfterShortTermPlaceOrderBroadcast(t *testing.T) {
	account := testAccount()
	sequences := NewSequenceManager(&countingAccountProvider{info: AccountInfo{
		AccountNumber: 44,
		Sequence:      4,
	}})
	service := NewService(ServiceConfig{
		Signer:          &recordingSigner{txBytes: []byte{9, 8, 7}},
		AccountProvider: sequences,
		Broadcaster:     &recordingBroadcaster{txHash: "SHORTTERM"},
		PrivateKeyProvider: staticPrivateKeyProvider{
			account: fakePrivKey{},
		},
	})

	resp, err := service.Submit(context.Background(), testPlaceOrderRequest(account))

	require.NoError(t, err)
	require.Equal(t, StatusBroadcasted, resp.Data.Status)
	next, err := sequences.Account(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, uint64(4), next.Sequence)
}

func TestServiceKeepsSequenceAfterStatefulPlaceOrderBroadcast(t *testing.T) {
	account := testAccount()
	sequences := NewSequenceManager(&countingAccountProvider{info: AccountInfo{
		AccountNumber: 44,
		Sequence:      4,
	}})
	service := NewService(ServiceConfig{
		Signer:          &recordingSigner{txBytes: []byte{9, 8, 7}},
		AccountProvider: sequences,
		Broadcaster:     &recordingBroadcaster{txHash: "STATEFUL"},
		PrivateKeyProvider: staticPrivateKeyProvider{
			account: fakePrivKey{},
		},
	})

	resp, err := service.Submit(context.Background(), testStatefulPlaceOrderRequest(account))

	require.NoError(t, err)
	require.Equal(t, StatusBroadcasted, resp.Data.Status)
	next, err := sequences.Account(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, uint64(5), next.Sequence)
}

func TestSubaccountDepositorSignsAndBroadcastsDepositToSubaccount(t *testing.T) {
	userAccount := testAccount()
	systemAccount := testOtherAccount(9)
	systemKey := fakePrivKey{id: "system-wallet"}
	signer := &recordingSigner{txBytes: []byte{1, 2, 3}}
	broadcaster := &recordingBroadcaster{txHash: "FUNDINGTX1"}
	depositor := NewSubaccountDepositor(SubaccountDepositorConfig{
		ChainID:          "dydx-local-1",
		SystemAddress:    systemAccount,
		SystemPrivateKey: systemKey,
		Signer:           signer,
		AccountProvider: StaticAccountProvider{
			AccountNumber: 99,
			Sequence:      4,
		},
		Broadcaster:   broadcaster,
		BroadcastMode: BroadcastModeSync,
	})

	result, err := depositor.DepositToSubaccount(context.Background(), SubaccountDepositRequest{
		ID:               "fund-order-1",
		UserID:           "user-1",
		DYDXAccount:      userAccount,
		SubaccountNumber: 0,
		AssetID:          0,
		Quantums:         "1892000",
		MarginUSD:        "1.892",
	})

	require.NoError(t, err)
	require.Equal(t, "FUNDINGTX1", result.TxHash)
	require.Equal(t, StatusBroadcasted, result.Status)
	require.Equal(t, "1892000", result.Quantums)
	require.Equal(t, BroadcastModeSync, broadcaster.mode)
	require.Equal(t, systemAccount, signer.req.SignerAddress)
	require.Equal(t, systemKey.Bytes(), signer.req.PrivateKey.Bytes())
	require.Equal(t, DefaultTxGasLimit, signer.req.GasLimit)
	require.Equal(t, defaultTxFeeAmount(nil), signer.req.FeeAmount)
	require.Len(t, signer.req.Messages, 1)
	msg, ok := signer.req.Messages[0].(*sendingtypes.MsgDepositToSubaccount)
	require.True(t, ok)
	require.Equal(t, systemAccount, msg.Sender)
	require.Equal(t, userAccount, msg.Recipient.Owner)
	require.Equal(t, uint32(0), msg.Recipient.Number)
	require.Equal(t, uint32(0), msg.AssetId)
	require.Equal(t, uint64(1_892_000), msg.Quantums)
	require.NoError(t, msg.ValidateBasic())
}

func TestServiceSignsWithProxyWalletKeyForRequestedAccount(t *testing.T) {
	account := testAccount()
	proxyKey := fakePrivKey{id: "proxy-wallet"}
	systemKey := fakePrivKey{id: "system-wallet"}
	signer := &recordingSigner{txBytes: []byte{9, 8, 7}}
	service := NewService(ServiceConfig{
		Signer: signer,
		AccountProvider: StaticAccountProvider{
			AccountNumber: 44,
			Sequence:      9,
		},
		Broadcaster: &recordingBroadcaster{txHash: "ABC123"},
		PrivateKeyProvider: staticPrivateKeyProvider{
			account: proxyKey,
		},
		SystemAddress: testOtherAccount(9),
		GasTopUpper:   noopGasTopUpper{},
	})

	_, err := service.Submit(context.Background(), testPlaceOrderRequest(account))

	require.NoError(t, err)
	require.Equal(t, account, signer.req.SignerAddress)
	require.Equal(t, proxyKey.Bytes(), signer.req.PrivateKey.Bytes())
	require.NotEqual(t, systemKey.Bytes(), signer.req.PrivateKey.Bytes())
}

func TestServiceRejectsUnknownProxyWalletAccount(t *testing.T) {
	account := testAccount()
	broadcaster := &recordingBroadcaster{txHash: "ABC123"}
	service := NewService(ServiceConfig{
		Signer: &recordingSigner{txBytes: []byte{9, 8, 7}},
		AccountProvider: StaticAccountProvider{
			AccountNumber: 44,
			Sequence:      9,
		},
		Broadcaster:        broadcaster,
		PrivateKeyProvider: staticPrivateKeyProvider{},
		SystemAddress:      testOtherAccount(9),
		GasTopUpper:        noopGasTopUpper{},
	})

	_, err := service.Submit(context.Background(), testPlaceOrderRequest(account))

	require.ErrorIs(t, err, ErrProxyWalletNotFound)
	require.Empty(t, broadcaster.txBytes)
}

func TestServiceRejectsSystemWalletAsTradingAccount(t *testing.T) {
	systemAddress := testOtherAccount(9)
	broadcaster := &recordingBroadcaster{txHash: "ABC123"}
	service := NewService(ServiceConfig{
		Signer: &recordingSigner{txBytes: []byte{9, 8, 7}},
		AccountProvider: StaticAccountProvider{
			AccountNumber: 44,
			Sequence:      9,
		},
		Broadcaster: broadcaster,
		PrivateKeyProvider: staticPrivateKeyProvider{
			systemAddress: fakePrivKey{id: "proxy-wallet"},
		},
		SystemAddress: systemAddress,
		GasTopUpper:   noopGasTopUpper{},
	})

	_, err := service.Submit(context.Background(), testPlaceOrderRequest(systemAddress))

	require.ErrorIs(t, err, ErrSystemWalletTradingAccount)
	require.Empty(t, broadcaster.txBytes)
}

func TestServiceRejectsAccountOwnerMismatch(t *testing.T) {
	account := testAccount()
	otherAccount := testOtherAccount(8)
	broadcaster := &recordingBroadcaster{txHash: "ABC123"}
	req := testPlaceOrderRequest(account)
	req.Account = otherAccount
	req.Params.CosmosTx.Account = otherAccount
	service := NewService(ServiceConfig{
		Signer: &recordingSigner{txBytes: []byte{9, 8, 7}},
		AccountProvider: StaticAccountProvider{
			AccountNumber: 44,
			Sequence:      9,
		},
		Broadcaster: broadcaster,
		PrivateKeyProvider: staticPrivateKeyProvider{
			otherAccount: fakePrivKey{id: "proxy-wallet"},
		},
	})

	_, err := service.Submit(context.Background(), req)

	require.ErrorIs(t, err, ErrTradingAccountMessageOwnerMismatch)
	require.Empty(t, broadcaster.txBytes)
}

func TestHTTPHandlerReturnsTycheCompatibleResponse(t *testing.T) {
	account := testAccount()
	service := NewService(ServiceConfig{
		Signer: &recordingSigner{txBytes: []byte{1, 2, 3}},
		AccountProvider: StaticAccountProvider{
			AccountNumber: 1,
			Sequence:      0,
		},
		Broadcaster: &recordingBroadcaster{txHash: "HTTPHASH"},
		PrivateKeyProvider: staticPrivateKeyProvider{
			account: fakePrivKey{},
		},
	})
	body, err := json.Marshal(testPlaceOrderRequest(account))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()

	NewHTTPHandler(service, HTTPConfig{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/dydx/trading", bytes.NewReader(body)))

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp TycheTradingResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 200, resp.Code)
	require.Equal(t, "HTTPHASH", resp.Data.TxHash)
}

func TestHTTPHandlerRejectsMissingBearerTokenWhenConfigured(t *testing.T) {
	service := NewService(ServiceConfig{})
	recorder := httptest.NewRecorder()

	NewHTTPHandler(service, HTTPConfig{BearerToken: "secret"}).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/dydx/trading", strings.NewReader(`{}`)),
	)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	var resp TycheTradingResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestHTTPHandlerServesTycheTradingEventEndpoints(t *testing.T) {
	provider := &recordingTradingEventProvider{
		fills: TradingEventPage[TradingFillEvent]{
			NextCursor: "fill-next",
			Data: []TradingFillEvent{
				{
					EventID:        "fill-1",
					UserID:         "user-1",
					OrderID:        "order-1",
					Market:         "ETH-USD",
					Asset:          "USDT",
					ReleasedMargin: "12",
					Fee:            "1",
					RealizedPNL:    "3",
					OrderStatus:    "partial_filled",
				},
			},
		},
		statuses: TradingEventPage[TradingOrderStatusEvent]{
			NextCursor: "status-next",
			Data: []TradingOrderStatusEvent{
				{
					EventID:      "status-1",
					UserID:       "user-1",
					OrderID:      "order-1",
					Status:       "rejected",
					Reason:       "insufficient margin",
					RefundAsset:  "USDT",
					RefundAmount: "10",
				},
			},
		},
		positions: TradingEventPage[TradingPositionEvent]{
			NextCursor: "position-next",
			Data: []TradingPositionEvent{
				{
					EventID:       "position-1",
					UserID:        "user-1",
					Market:        "ETH-USD",
					Size:          "0.2",
					EntryPrice:    "2500",
					MarkPrice:     "2510",
					UnrealizedPNL: "2",
					MarginAsset:   "USDT",
					Margin:        "100",
				},
			},
		},
	}
	handler := NewHTTPHandler(NewService(ServiceConfig{}), HTTPConfig{
		EventAPIKey:   "event-secret",
		EventProvider: provider,
	})

	fillRecorder := httptest.NewRecorder()
	fillReq := httptest.NewRequest(http.MethodGet, "/fills?cursor=fill-cursor", nil)
	fillReq.Header.Set("X-API-Key", "event-secret")
	handler.ServeHTTP(fillRecorder, fillReq)

	require.Equal(t, http.StatusOK, fillRecorder.Code)
	var fillResp struct {
		Code       int                `json:"code"`
		NextCursor string             `json:"nextCursor"`
		Data       []TradingFillEvent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(fillRecorder.Body.Bytes(), &fillResp))
	require.Equal(t, 200, fillResp.Code)
	require.Equal(t, "fill-next", fillResp.NextCursor)
	require.Equal(t, "fill-cursor", provider.fillCursor)
	require.Equal(t, "fill-1", fillResp.Data[0].EventID)

	statusRecorder := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodGet, "/order-statuses?cursor=status-cursor", nil)
	statusReq.Header.Set("X-API-Key", "event-secret")
	handler.ServeHTTP(statusRecorder, statusReq)

	require.Equal(t, http.StatusOK, statusRecorder.Code)
	var statusResp struct {
		Code       int                       `json:"code"`
		NextCursor string                    `json:"nextCursor"`
		Data       []TradingOrderStatusEvent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRecorder.Body.Bytes(), &statusResp))
	require.Equal(t, 200, statusResp.Code)
	require.Equal(t, "status-next", statusResp.NextCursor)
	require.Equal(t, "status-cursor", provider.statusCursor)
	require.Equal(t, "rejected", statusResp.Data[0].Status)

	positionRecorder := httptest.NewRecorder()
	positionReq := httptest.NewRequest(http.MethodGet, "/positions?cursor=position-cursor", nil)
	positionReq.Header.Set("Authorization", "Bearer event-secret")
	handler.ServeHTTP(positionRecorder, positionReq)

	require.Equal(t, http.StatusOK, positionRecorder.Code)
	var positionResp struct {
		Code       int                    `json:"code"`
		NextCursor string                 `json:"nextCursor"`
		Data       []TradingPositionEvent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(positionRecorder.Body.Bytes(), &positionResp))
	require.Equal(t, 200, positionResp.Code)
	require.Equal(t, "position-next", positionResp.NextCursor)
	require.Equal(t, "position-cursor", provider.positionCursor)
	require.Equal(t, "2", positionResp.Data[0].UnrealizedPNL)
}

func TestHTTPHandlerServesMarketPriceEndpoint(t *testing.T) {
	provider := &recordingMarketPriceProvider{
		snapshot: MarketPriceSnapshot{
			MarketID: 1,
			Market:   "ETH-USD",
			Price:    "2500",
			Source:   "test-price-source",
		},
	}
	handler := NewHTTPHandler(NewService(ServiceConfig{}), HTTPConfig{
		EventAPIKey:         "event-secret",
		MarketPriceProvider: provider,
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/prices?market=ETH-USD&marketId=1", nil)
	req.Header.Set("X-API-Key", "event-secret")

	handler.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int                 `json:"code"`
		Data MarketPriceSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, uint32(1), provider.marketID)
	require.Equal(t, "2500", resp.Data.Price)
	require.Equal(t, "ETH-USD", resp.Data.Market)
}

func TestHTTPHandlerRejectsTradingEventEndpointWithoutAPIKey(t *testing.T) {
	recorder := httptest.NewRecorder()

	NewHTTPHandler(NewService(ServiceConfig{}), HTTPConfig{
		EventAPIKey:   "event-secret",
		EventProvider: EmptyTradingEventProvider{},
	}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fills", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestRESTBroadcasterPostsBase64TxBytes(t *testing.T) {
	var posted struct {
		TxBytes string `json:"tx_bytes"`
		Mode    string `json:"mode"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, err := base64.StdEncoding.DecodeString(posted.TxBytes)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`{"tx_response":{"txhash":"RESTHASH","code":0,"raw_log":""}}`))
	}))
	defer server.Close()

	result, err := NewRESTBroadcaster(server.URL, server.Client()).Broadcast(context.Background(), BroadcastRequest{
		TxBytes: []byte{1, 2, 3},
		Mode:    BroadcastModeSync,
	})

	require.NoError(t, err)
	require.Equal(t, "RESTHASH", result.TxHash)
	require.Equal(t, BroadcastModeSync, posted.Mode)
}

func TestRESTAccountProviderExtractsNestedAccountNumberAndSequence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasPrefix(r.URL.Path, "/cosmos/auth/v1beta1/accounts/"))
		_, _ = w.Write([]byte(`{
			"account": {
				"@type": "/dydxprotocol.accountplus.AccountPlus",
				"base_account": {
					"account_number": "77",
					"sequence": "8"
				}
			}
		}`))
	}))
	defer server.Close()

	info, err := NewRESTAccountProvider(server.URL, server.Client()).Account(context.Background(), "dydx1abc")

	require.NoError(t, err)
	require.Equal(t, uint64(77), info.AccountNumber)
	require.Equal(t, uint64(8), info.Sequence)
}

func TestRESTMarketPriceProviderConvertsChainPriceExponent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dydxprotocol/prices/market/1", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"market_price": map[string]any{
				"id":       1,
				"exponent": -6,
				"price":    "2500000000",
			},
		})
	}))
	defer server.Close()

	snapshot, err := NewRESTMarketPriceProvider(server.URL, server.Client()).MarketPrice(context.Background(), MarketPriceRequest{
		MarketID: 1,
		Market:   "ETH-USD",
	})

	require.NoError(t, err)
	require.Equal(t, uint32(1), snapshot.MarketID)
	require.Equal(t, "ETH-USD", snapshot.Market)
	require.Equal(t, "2500", snapshot.Price)
	require.Equal(t, "2500000000", snapshot.RawPrice)
	require.Equal(t, int32(-6), snapshot.Exponent)
}

func TestSequenceManagerReservesSequencePerAddress(t *testing.T) {
	upstream := &countingAccountProvider{info: AccountInfo{AccountNumber: 12, Sequence: 5}}
	manager := NewSequenceManager(upstream)

	first, err := manager.Account(context.Background(), "dydx1first")
	require.NoError(t, err)
	second, err := manager.Account(context.Background(), "dydx1first")
	require.NoError(t, err)
	other, err := manager.Account(context.Background(), "dydx1other")
	require.NoError(t, err)

	require.Equal(t, uint64(5), first.Sequence)
	require.Equal(t, uint64(6), second.Sequence)
	require.Equal(t, uint64(5), other.Sequence)
	require.Equal(t, 2, upstream.calls)
}

func TestSequenceManagerCanReleaseFailedReservation(t *testing.T) {
	upstream := &countingAccountProvider{info: AccountInfo{AccountNumber: 12, Sequence: 5}}
	manager := NewSequenceManager(upstream)

	reserved, err := manager.Account(context.Background(), "dydx1first")
	require.NoError(t, err)
	manager.Release("dydx1first", reserved.Sequence)
	next, err := manager.Account(context.Background(), "dydx1first")
	require.NoError(t, err)

	require.Equal(t, reserved.Sequence, next.Sequence)
	require.Equal(t, 1, upstream.calls)
}

func TestPrivateKeyFromHexRejectsInvalidLength(t *testing.T) {
	_, err := PrivateKeyFromHex("abcd")

	require.Error(t, err)
}

type countingAccountProvider struct {
	info  AccountInfo
	calls int
}

func (p *countingAccountProvider) Account(context.Context, string) (AccountInfo, error) {
	p.calls++
	return p.info, nil
}

type recordingTradingEventProvider struct {
	fills          TradingEventPage[TradingFillEvent]
	statuses       TradingEventPage[TradingOrderStatusEvent]
	positions      TradingEventPage[TradingPositionEvent]
	fillCursor     string
	statusCursor   string
	positionCursor string
}

func (p *recordingTradingEventProvider) Fills(_ context.Context, cursor string) (TradingEventPage[TradingFillEvent], error) {
	p.fillCursor = cursor
	return p.fills, nil
}

func (p *recordingTradingEventProvider) OrderStatuses(_ context.Context, cursor string) (TradingEventPage[TradingOrderStatusEvent], error) {
	p.statusCursor = cursor
	return p.statuses, nil
}

func (p *recordingTradingEventProvider) Positions(_ context.Context, cursor string) (TradingEventPage[TradingPositionEvent], error) {
	p.positionCursor = cursor
	return p.positions, nil
}

type recordingMarketPriceProvider struct {
	marketID uint32
	snapshot MarketPriceSnapshot
	err      error
}

func (p *recordingMarketPriceProvider) MarketPrice(ctx context.Context, request MarketPriceRequest) (MarketPriceSnapshot, error) {
	p.marketID = request.MarketID
	return p.snapshot, p.err
}

type recordingBroadcaster struct {
	txHash  string
	txBytes []byte
	mode    string
	code    uint32
	rawLog  string
	err     error
}

func (b *recordingBroadcaster) Broadcast(_ context.Context, req BroadcastRequest) (BroadcastResult, error) {
	b.txBytes = append([]byte(nil), req.TxBytes...)
	b.mode = req.Mode
	return BroadcastResult{TxHash: b.txHash, Code: b.code, RawLog: b.rawLog}, b.err
}

type recordingSigner struct {
	req     SignRequest
	txBytes []byte
}

func (s *recordingSigner) Sign(_ context.Context, req SignRequest) ([]byte, error) {
	s.req = req
	return append([]byte(nil), s.txBytes...), nil
}

type fakePrivKey struct {
	id string
}

func (fakePrivKey) Reset() {}
func (k fakePrivKey) String() string {
	if k.id == "" {
		return "fake"
	}
	return "fake-" + k.id
}
func (fakePrivKey) ProtoMessage() {}
func (k fakePrivKey) Bytes() []byte {
	if k.id == "" {
		return []byte("fake-private-key")
	}
	return []byte("fake-private-key-" + k.id)
}
func (fakePrivKey) Sign([]byte) ([]byte, error)           { return []byte("signature"), nil }
func (fakePrivKey) PubKey() cryptotypes.PubKey            { return fakePubKey{} }
func (fakePrivKey) Equals(cryptotypes.LedgerPrivKey) bool { return false }
func (fakePrivKey) Type() string                          { return "fake" }

type fakePubKey struct{}

func (fakePubKey) Reset()         {}
func (fakePubKey) String() string { return "fake" }
func (fakePubKey) ProtoMessage()  {}
func (fakePubKey) Address() cryptotypes.Address {
	return cryptotypes.Address(bytes.Repeat([]byte{2}, 20))
}
func (fakePubKey) Bytes() []byte                       { return bytes.Repeat([]byte{3}, 33) }
func (fakePubKey) VerifySignature([]byte, []byte) bool { return true }
func (fakePubKey) Equals(cryptotypes.PubKey) bool      { return false }
func (fakePubKey) Type() string                        { return "fake" }

func testAccount() string {
	return sdk.MustBech32ifyAddressBytes(appconfig.Bech32PrefixAccAddr, bytes.Repeat([]byte{1}, 20))
}

func testOtherAccount(seed byte) string {
	return sdk.MustBech32ifyAddressBytes(appconfig.Bech32PrefixAccAddr, bytes.Repeat([]byte{seed}, 20))
}

type staticPrivateKeyProvider map[string]cryptotypes.PrivKey

func (p staticPrivateKeyProvider) PrivateKeyForAddress(_ context.Context, address string) (cryptotypes.PrivKey, error) {
	key, ok := p[address]
	if !ok {
		return nil, ErrProxyWalletNotFound
	}
	return key, nil
}

type noopGasTopUpper struct{}

func (noopGasTopUpper) EnsureGas(context.Context, string) error {
	return nil
}

func testPlaceOrderRequest(account string) TycheTradingRequest {
	return TycheTradingRequest{
		ID:      "order-1",
		Action:  ActionPlaceOrder,
		Account: account,
		Params: TycheTradingParams{
			CosmosTx: CosmosTx{
				ID:            "order-1",
				Action:        ActionPlaceOrder,
				Account:       account,
				ChainID:       "dydx-local-1",
				SignMode:      SignModeDirect,
				BroadcastMode: BroadcastModeSync,
				Message: CosmosMessage{
					TypeURL: TypeURLPlaceOrder,
					Value: mustRawJSON(map[string]any{
						"order": map[string]any{
							"orderId": map[string]any{
								"subaccountId": map[string]any{
									"owner":  account,
									"number": 0,
								},
								"clientId":   123,
								"orderFlags": 0,
								"clobPairId": 7,
							},
							"side":           1,
							"quantums":       1_250_000,
							"subticks":       2_500_500_000,
							"goodTilBlock":   120,
							"timeInForce":    0,
							"reduceOnly":     false,
							"clientMetadata": 0,
						},
					}),
				},
			},
		},
	}
}

func testStatefulPlaceOrderRequest(account string) TycheTradingRequest {
	req := testPlaceOrderRequest(account)
	req.Params.CosmosTx.Message.Value = mustRawJSON(map[string]any{
		"order": map[string]any{
			"orderId": map[string]any{
				"subaccountId": map[string]any{
					"owner":  account,
					"number": 0,
				},
				"clientId":   123,
				"orderFlags": 64,
				"clobPairId": 7,
			},
			"side":             1,
			"quantums":         1_250_000,
			"subticks":         2_500_500_000,
			"goodTilBlockTime": 1_786_903_600,
			"timeInForce":      0,
			"reduceOnly":       false,
			"clientMetadata":   0,
		},
	})
	return req
}

func testCancelOrderRequest(account string) TycheTradingRequest {
	return TycheTradingRequest{
		ID:      "cancel-1",
		Action:  ActionCancelOrder,
		Account: account,
		Params: TycheTradingParams{
			CosmosTx: CosmosTx{
				ID:            "cancel-1",
				Action:        ActionCancelOrder,
				Account:       account,
				ChainID:       "dydx-local-1",
				SignMode:      SignModeDirect,
				BroadcastMode: BroadcastModeSync,
				Message: CosmosMessage{
					TypeURL: TypeURLCancelOrder,
					Value: mustRawJSON(map[string]any{
						"orderId": map[string]any{
							"subaccountId": map[string]any{
								"owner":  account,
								"number": 0,
							},
							"clientId":   123,
							"orderFlags": 0,
							"clobPairId": 7,
						},
						"goodTilBlock": 130,
					}),
				},
			},
		},
	}
}

func mustRawJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func setCosmosMessageValue(t *testing.T, req *TycheTradingRequest, value any) {
	t.Helper()
	req.Params.CosmosTx.Message.Value = mustRawJSON(value)
}

var _ = appconfig.AccountAddressPrefix

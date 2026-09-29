//go:build cgo

package dydxtrading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"
	satypes "github.com/dydxprotocol/v4-chain/protocol/x/subaccounts/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type ChainReconcileProvider struct {
	restURL string
	grpcURL string
	client  *http.Client

	mu   sync.Mutex
	conn *grpc.ClientConn
}

func NewChainReconcileProvider(restURL, grpcURL string, client *http.Client) *ChainReconcileProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &ChainReconcileProvider{
		restURL: strings.TrimRight(restURL, "/"),
		grpcURL: strings.TrimSpace(grpcURL),
		client:  client,
	}
}

func (p *ChainReconcileProvider) ReconcileOrder(ctx context.Context, request OrderReconcileRequest) (OrderReconcileResult, error) {
	if p == nil {
		return OrderReconcileResult{}, errors.New("dydx chain reconcile provider is not configured")
	}
	var result OrderReconcileResult
	var err error
	if strings.TrimSpace(request.TxHash) != "" {
		result.Tx, err = p.queryTx(ctx, request.TxHash)
		if err != nil {
			return OrderReconcileResult{}, err
		}
	}
	if request.DYDXAccount != "" && request.ClientOrderID != "" && request.ClobPairID != 0 {
		result.StatefulOrder, err = p.queryStatefulOrder(ctx, request)
		if err != nil {
			return OrderReconcileResult{}, err
		}
	}
	if request.DYDXAccount != "" {
		result.Subaccount, err = p.querySubaccount(ctx, request.DYDXAccount, request.SubaccountNumber)
		if err != nil {
			return OrderReconcileResult{}, err
		}
	}
	return result, nil
}

func (p *ChainReconcileProvider) queryTx(ctx context.Context, txHash string) (TxReconcileSnapshot, error) {
	if p.restURL == "" {
		return TxReconcileSnapshot{}, errors.New("dydx REST URL is not configured")
	}
	endpoint := p.restURL + "/cosmos/tx/v1beta1/txs/" + url.PathEscape(strings.TrimSpace(txHash))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TxReconcileSnapshot{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return TxReconcileSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return TxReconcileSnapshot{Found: false, TxHash: txHash}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return TxReconcileSnapshot{}, fmt.Errorf("dydx tx query returned http %d", resp.StatusCode)
	}
	var body struct {
		TxHash     string `json:"txhash"`
		Height     string `json:"height"`
		Code       uint32 `json:"code"`
		RawLog     string `json:"raw_log"`
		TxResponse *struct {
			TxHash string `json:"txhash"`
			Height string `json:"height"`
			Code   uint32 `json:"code"`
			RawLog string `json:"raw_log"`
		} `json:"tx_response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return TxReconcileSnapshot{}, err
	}
	out := TxReconcileSnapshot{
		Found:  true,
		TxHash: firstReconcileString(body.TxHash, txHash),
		Height: body.Height,
		Code:   body.Code,
		RawLog: body.RawLog,
	}
	if body.TxResponse != nil {
		out.TxHash = firstReconcileString(body.TxResponse.TxHash, out.TxHash)
		out.Height = firstReconcileString(body.TxResponse.Height, out.Height)
		out.Code = body.TxResponse.Code
		out.RawLog = firstReconcileString(body.TxResponse.RawLog, out.RawLog)
	}
	return out, nil
}

func (p *ChainReconcileProvider) queryStatefulOrder(ctx context.Context, request OrderReconcileRequest) (StatefulOrderReconcileSnapshot, error) {
	if p.grpcURL == "" {
		return StatefulOrderReconcileSnapshot{Checked: false}, nil
	}
	clientID, err := strconv.ParseUint(strings.TrimSpace(request.ClientOrderID), 10, 32)
	if err != nil {
		return StatefulOrderReconcileSnapshot{}, fmt.Errorf("invalid clientOrderId: %w", err)
	}
	conn, err := p.grpcConn(ctx)
	if err != nil {
		return StatefulOrderReconcileSnapshot{}, err
	}
	resp, err := clobtypes.NewQueryClient(conn).StatefulOrder(ctx, &clobtypes.QueryStatefulOrderRequest{
		OrderId: clobtypes.OrderId{
			SubaccountId: satypes.SubaccountId{
				Owner:  request.DYDXAccount,
				Number: request.SubaccountNumber,
			},
			ClientId:   uint32(clientID),
			ClobPairId: request.ClobPairID,
			OrderFlags: request.OrderFlags,
		},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return StatefulOrderReconcileSnapshot{Checked: true, Found: false}, nil
		}
		return StatefulOrderReconcileSnapshot{}, err
	}
	return StatefulOrderReconcileSnapshot{
		Checked:    true,
		Found:      true,
		FillAmount: strconv.FormatUint(resp.GetFillAmount(), 10),
	}, nil
}

func (p *ChainReconcileProvider) querySubaccount(ctx context.Context, owner string, number uint32) (SubaccountReconcileSnapshot, error) {
	if p.restURL == "" {
		return SubaccountReconcileSnapshot{}, errors.New("dydx REST URL is not configured")
	}
	endpoint := fmt.Sprintf("%s/dydxprotocol/subaccounts/subaccount/%s/%d", p.restURL, url.PathEscape(owner), number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SubaccountReconcileSnapshot{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return SubaccountReconcileSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return SubaccountReconcileSnapshot{Found: false}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return SubaccountReconcileSnapshot{}, fmt.Errorf("dydx subaccount query returned http %d", resp.StatusCode)
	}
	var body struct {
		Subaccount struct {
			AssetPositions []struct {
				AssetID  uint32          `json:"asset_id"`
				Quantums json.RawMessage `json:"quantums"`
			} `json:"asset_positions"`
			PerpetualPositions []struct {
				PerpetualID  uint32          `json:"perpetual_id"`
				Quantums     json.RawMessage `json:"quantums"`
				FundingIndex json.RawMessage `json:"funding_index"`
				QuoteBalance json.RawMessage `json:"quote_balance"`
			} `json:"perpetual_positions"`
		} `json:"subaccount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return SubaccountReconcileSnapshot{}, err
	}
	out := SubaccountReconcileSnapshot{
		Found:              true,
		AssetPositions:     make([]AssetPositionReconcileSnapshot, 0, len(body.Subaccount.AssetPositions)),
		PerpetualPositions: make([]PerpetualPositionReconcileSnapshot, 0, len(body.Subaccount.PerpetualPositions)),
	}
	for _, position := range body.Subaccount.AssetPositions {
		out.AssetPositions = append(out.AssetPositions, AssetPositionReconcileSnapshot{
			AssetID:  position.AssetID,
			Quantums: rawJSONString(position.Quantums),
		})
	}
	for _, position := range body.Subaccount.PerpetualPositions {
		out.PerpetualPositions = append(out.PerpetualPositions, PerpetualPositionReconcileSnapshot{
			PerpetualID:  position.PerpetualID,
			Quantums:     rawJSONString(position.Quantums),
			FundingIndex: rawJSONString(position.FundingIndex),
			QuoteBalance: rawJSONString(position.QuoteBalance),
		})
	}
	return out, nil
}

func (p *ChainReconcileProvider) grpcConn(ctx context.Context) (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		return p.conn, nil
	}
	conn, err := grpc.DialContext(ctx, p.grpcURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	p.conn = conn
	return p.conn, nil
}

func firstReconcileString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func rawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return strings.Trim(string(raw), `"`)
}

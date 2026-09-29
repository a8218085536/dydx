//go:build cgo

package dydxtrading

import (
	"context"
	"encoding/json"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	ActionPlaceOrder  = "place_order"
	ActionCancelOrder = "cancel_order"

	TypeURLPlaceOrder  = "/dydxprotocol.clob.MsgPlaceOrder"
	TypeURLCancelOrder = "/dydxprotocol.clob.MsgCancelOrder"

	SignModeDirect    = "SIGN_MODE_DIRECT"
	BroadcastModeSync = "BROADCAST_MODE_SYNC"
	StatusBroadcasted = "broadcasted"
	StatusSubmitted   = "submitted"

	BroadcastCodeTxAlreadyInMempool uint32 = 19
	DefaultTxGasLimit               uint64 = 500000
	DefaultTxFeeAmount              string = "12500000000000000adedx"
)

type TycheTradingRequest struct {
	ID      string             `json:"id"`
	Action  string             `json:"action"`
	Account string             `json:"account"`
	Params  TycheTradingParams `json:"params"`
}

type TycheTradingParams struct {
	CosmosTx CosmosTx `json:"cosmosTx"`
}

type CosmosTx struct {
	ID            string        `json:"id"`
	Action        string        `json:"action"`
	Account       string        `json:"account"`
	ChainID       string        `json:"chainId"`
	SignMode      string        `json:"signMode"`
	BroadcastMode string        `json:"broadcastMode"`
	GasLimit      uint64        `json:"gasLimit,omitempty"`
	FeeAmount     string        `json:"feeAmount,omitempty"`
	Message       CosmosMessage `json:"message"`
}

type CosmosMessage struct {
	TypeURL string          `json:"typeUrl"`
	Value   json.RawMessage `json:"value"`
}

type MessageMeta struct {
	ID            string
	Action        string
	Account       string
	ChainID       string
	BroadcastMode string
	GasLimit      uint64
	FeeAmount     sdk.Coins
}

type TycheTradingResponse struct {
	Code int              `json:"code"`
	Msg  string           `json:"msg"`
	Data TycheTradingData `json:"data"`
}

type TycheTradingData struct {
	TxHash string `json:"txHash"`
	Status string `json:"status"`
}

type AccountInfo struct {
	AccountNumber uint64
	Sequence      uint64
}

type AccountProvider interface {
	Account(ctx context.Context, address string) (AccountInfo, error)
}

type SignRequest struct {
	ChainID       string
	AccountNumber uint64
	Sequence      uint64
	SignerAddress string
	PrivateKey    cryptotypes.PrivKey
	Messages      []sdk.Msg
	GasLimit      uint64
	FeeAmount     sdk.Coins
}

type Signer interface {
	Sign(ctx context.Context, req SignRequest) ([]byte, error)
}

type BroadcastRequest struct {
	TxBytes []byte
	Mode    string
}

type BroadcastResult struct {
	TxHash string
	Code   uint32
	RawLog string
}

type Broadcaster interface {
	Broadcast(ctx context.Context, req BroadcastRequest) (BroadcastResult, error)
}

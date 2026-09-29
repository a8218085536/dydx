package dydxtrading

import "context"

type SubaccountDepositRequest struct {
	ID               string `json:"id"`
	UserID           string `json:"userId,omitempty"`
	DYDXAccount      string `json:"dydxAccount"`
	SubaccountNumber uint32 `json:"subaccountNumber"`
	AssetID          uint32 `json:"assetId"`
	Quantums         string `json:"quantums"`
	MarginUSD        string `json:"marginUsd,omitempty"`
}

type SubaccountDepositResult struct {
	TxHash           string `json:"txHash"`
	Status           string `json:"status"`
	DYDXAccount      string `json:"dydxAccount,omitempty"`
	SubaccountNumber uint32 `json:"subaccountNumber,omitempty"`
	AssetID          uint32 `json:"assetId"`
	Quantums         string `json:"quantums"`
}

type SubaccountDepositService interface {
	DepositToSubaccount(context.Context, SubaccountDepositRequest) (SubaccountDepositResult, error)
}

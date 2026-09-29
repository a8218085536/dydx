package dydxtrading

import "context"

type ProxyWallet struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress,omitempty"`
	DYDXAddress string `json:"dydxAddress"`
	CreatedAt   int64  `json:"createdAt"`
}

type ProxyWalletCreateRequest struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress,omitempty"`
}

type ProxyWalletProvisioner interface {
	CreateOrGetProxyWallet(context.Context, ProxyWalletCreateRequest) (ProxyWallet, error)
}

package tyche

import (
	"context"
	"errors"
)

var ErrDYDXChainClientNotConfigured = errors.New("dydx chain client is not configured")

type DYDXChainRequest struct {
	ID      string                 `json:"id"`
	Action  string                 `json:"action"`
	Account string                 `json:"account"`
	Params  map[string]interface{} `json:"params"`
}

type DYDXChainClient interface {
	Broadcast(context.Context, DYDXChainRequest) (string, error)
}

type unconfiguredDYDXChainClient struct{}

func (unconfiguredDYDXChainClient) Broadcast(context.Context, DYDXChainRequest) (string, error) {
	return "", ErrDYDXChainClientNotConfigured
}

type DYDXChainGateway struct {
	client DYDXChainClient
}

func NewDYDXChainGateway(client DYDXChainClient) *DYDXChainGateway {
	if client == nil {
		client = unconfiguredDYDXChainClient{}
	}
	return &DYDXChainGateway{client: client}
}

func (g *DYDXChainGateway) Broadcast(ctx context.Context, request DYDXChainRequest) (string, error) {
	switch request.Action {
	case DYDXActionPlaceOrder, DYDXActionCancelOrder:
		return g.client.Broadcast(ctx, request)
	default:
		return "", ErrUnsupportedDYDXAction
	}
}

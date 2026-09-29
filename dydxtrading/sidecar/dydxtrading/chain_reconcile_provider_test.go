//go:build cgo

package dydxtrading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChainReconcileProviderQueriesRESTTxAndSubaccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cosmos/tx/v1beta1/txs/HASH":
			_, _ = w.Write([]byte(`{"tx_response":{"txhash":"HASH","height":"10","code":0,"raw_log":""}}`))
		case "/dydxprotocol/subaccounts/subaccount/dydx1abc/0":
			_, _ = w.Write([]byte(`{
				"subaccount":{
					"id":{"owner":"dydx1abc","number":0},
					"asset_positions":[{"asset_id":0,"quantums":"669006551","index":"0"}],
					"perpetual_positions":[{"perpetual_id":1,"quantums":"-1000000","funding_index":"1633","quote_balance":"0"}],
					"margin_enabled":false
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider := NewChainReconcileProvider(server.URL, "", server.Client())

	result, err := provider.ReconcileOrder(context.Background(), OrderReconcileRequest{
		TxHash:           "HASH",
		DYDXAccount:      "dydx1abc",
		SubaccountNumber: 0,
	})

	require.NoError(t, err)
	require.True(t, result.Tx.Found)
	require.Equal(t, "10", result.Tx.Height)
	require.True(t, result.Subaccount.Found)
	require.Equal(t, "669006551", result.Subaccount.AssetPositions[0].Quantums)
	require.Equal(t, "-1000000", result.Subaccount.PerpetualPositions[0].Quantums)
}

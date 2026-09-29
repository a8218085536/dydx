package tyche

import (
	"reflect"
	"testing"
)

func TestRedisScannedStructsDeclareRedisTags(t *testing.T) {
	types := []interface{}{
		AuthChallenge{},
		AuthUser{},
		WalletBinding{},
		NECODeposit{},
		NECOWithdrawalRecord{},
		LedgerWithdrawal{},
		TradingOrder{},
		TradingFill{},
		TradingPosition{},
		TradingSyncCursor{},
		TradingOrderStatusEvent{},
		TradingSettlement{},
		TradingTPSL{},
		NECOCachedAssetPrice{},
	}

	for _, sample := range types {
		typ := reflect.TypeOf(sample)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			if field.Tag.Get("redis") == "" {
				t.Fatalf("%s.%s is missing redis tag", typ.Name(), field.Name)
			}
		}
	}
}

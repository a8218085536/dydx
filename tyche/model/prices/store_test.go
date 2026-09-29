package prices

import (
	"ethgo/tyche"
	"fmt"
	"testing"
)

func TestAssetPriceRedisHashRoundTripPreservesSnapshotFields(t *testing.T) {
	entry := tyche.NECOCachedAssetPrice{
		ID:           "snap-1",
		Asset:        "DEDX",
		USDAsset:     "USD",
		USDPrice:     "4.0001",
		RouteName:    "DEDX-NECO-USD",
		Source:       "v2_pair:DEDX-NECO-USD",
		BlockNumber:  123,
		MinLiquidity: "4000000",
		Pairs:        []string{"pair-a", "pair-b"},
		UpdatedAt:    1000,
		ExpiresAt:    1030,
	}

	values := redisReplyValues(assetPriceHashValues(entry))
	got, err := scanAssetPriceHash(values)
	if err != nil {
		t.Fatalf("scanAssetPriceHash() error = %v", err)
	}

	if got.Asset != entry.Asset || got.USDPrice != entry.USDPrice || got.RouteName != entry.RouteName {
		t.Fatalf("round-trip entry = %+v", got)
	}
	if got.BlockNumber != entry.BlockNumber || got.UpdatedAt != entry.UpdatedAt || got.ExpiresAt != entry.ExpiresAt {
		t.Fatalf("round-trip timestamps/block = %+v", got)
	}
	if len(got.Pairs) != 2 || got.Pairs[0] != "pair-a" || got.Pairs[1] != "pair-b" {
		t.Fatalf("round-trip pairs = %+v", got.Pairs)
	}
}

func redisReplyValues(values []interface{}) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, []byte(fmt.Sprint(value)))
	}
	return out
}

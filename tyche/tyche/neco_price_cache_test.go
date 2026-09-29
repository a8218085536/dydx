package tyche

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNECOPriceCacheWorkerPollOnceCachesUniqueAssets(t *testing.T) {
	now := time.Unix(1000, 0)
	store := newMemoryNECOPriceCacheStore()
	worker := NewNECOPriceCacheWorker(
		staticAssetUSDOracle{
			snapshots: map[string]AssetPriceSnapshot{
				"DEDX": {
					ID:           "snap-dedx",
					Asset:        "DEDX",
					USDAsset:     "USD",
					USDPrice:     "4",
					RouteName:    "DEDX-NECO-USD",
					Source:       "v2_pair:DEDX-NECO-USD",
					BlockNumber:  123,
					MinLiquidity: "4000000",
					Pairs:        []string{"pair-a", "pair-b"},
				},
				"USDT": {
					ID:        "snap-usdt",
					Asset:     "USDT",
					USDAsset:  "USD",
					USDPrice:  "1",
					RouteName: "USDT-USD",
					Source:    "stable_usd:USDT-USD",
				},
			},
		},
		store,
		[]string{" dedx ", "USDT", "DEDX"},
		time.Second,
		30*time.Second,
	)
	worker.now = func() time.Time { return now }

	entries, err := worker.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("cached entries = %d, want 2", len(entries))
	}

	dedx, ok, err := store.GetAssetPrice(context.Background(), "DEDX")
	if err != nil {
		t.Fatalf("GetAssetPrice() error = %v", err)
	}
	if !ok {
		t.Fatal("DEDX price was not cached")
	}
	if dedx.Asset != "DEDX" || dedx.USDPrice != "4" || dedx.RouteName != "DEDX-NECO-USD" {
		t.Fatalf("DEDX cache entry = %+v", dedx)
	}
	if dedx.UpdatedAt != now.Unix() || dedx.ExpiresAt != now.Add(30*time.Second).Unix() {
		t.Fatalf("DEDX cache timestamps = updated %d expires %d", dedx.UpdatedAt, dedx.ExpiresAt)
	}
	if len(dedx.Pairs) != 2 || dedx.Pairs[0] != "pair-a" || dedx.Pairs[1] != "pair-b" {
		t.Fatalf("DEDX pairs = %+v", dedx.Pairs)
	}
}

func TestCachedAssetUSDOracleRejectsStaleCachedPrice(t *testing.T) {
	store := newMemoryNECOPriceCacheStore()
	if err := store.SaveAssetPrice(context.Background(), NECOCachedAssetPrice{
		Asset:     "DEDX",
		USDAsset:  "USD",
		USDPrice:  "4",
		UpdatedAt: 1000,
		ExpiresAt: 1010,
	}); err != nil {
		t.Fatalf("SaveAssetPrice() error = %v", err)
	}
	oracle := NewCachedAssetUSDOracle(store)
	oracle.now = func() time.Time { return time.Unix(1011, 0) }

	_, err := oracle.AssetUSDPrice(context.Background(), "DEDX")
	if !errors.Is(err, ErrNECOPriceStale) {
		t.Fatalf("AssetUSDPrice() error = %v, want %v", err, ErrNECOPriceStale)
	}
}

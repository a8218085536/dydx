package tyche

import (
	"context"
	"sort"
	"time"
)

type NECOCachedAssetPrice struct {
	ID           string   `json:"id" redis:"id"`
	Asset        string   `json:"asset" redis:"asset"`
	USDAsset     string   `json:"usdAsset" redis:"usdAsset"`
	USDPrice     string   `json:"usdPrice" redis:"usdPrice"`
	RouteName    string   `json:"routeName" redis:"routeName"`
	Source       string   `json:"source" redis:"source"`
	BlockNumber  uint64   `json:"blockNumber" redis:"blockNumber"`
	MinLiquidity string   `json:"minLiquidityUsd,omitempty" redis:"minLiquidityUsd"`
	Pairs        []string `json:"pairs,omitempty" redis:"pairs"`
	UpdatedAt    int64    `json:"updatedAt" redis:"updatedAt"`
	ExpiresAt    int64    `json:"expiresAt" redis:"expiresAt"`
}

func (p NECOCachedAssetPrice) Fresh(now time.Time) bool {
	return p.ExpiresAt == 0 || now.Unix() <= p.ExpiresAt
}

func (p NECOCachedAssetPrice) Snapshot() AssetPriceSnapshot {
	return AssetPriceSnapshot{
		ID:           p.ID,
		Asset:        p.Asset,
		USDAsset:     p.USDAsset,
		USDPrice:     p.USDPrice,
		RouteName:    p.RouteName,
		Source:       p.Source,
		BlockNumber:  p.BlockNumber,
		MinLiquidity: p.MinLiquidity,
		Pairs:        append([]string(nil), p.Pairs...),
	}
}

type NECOAssetPriceCacheStore interface {
	SaveAssetPrice(context.Context, NECOCachedAssetPrice) error
	GetAssetPrice(context.Context, string) (NECOCachedAssetPrice, bool, error)
}

type NECOPriceCacheWorker struct {
	oracle   AssetUSDOracle
	store    NECOAssetPriceCacheStore
	assets   []string
	interval time.Duration
	ttl      time.Duration
	now      func() time.Time
}

func NewNECOPriceCacheWorker(oracle AssetUSDOracle, store NECOAssetPriceCacheStore, assets []string, interval, ttl time.Duration) *NECOPriceCacheWorker {
	if interval <= 0 {
		interval = time.Second
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &NECOPriceCacheWorker{
		oracle:   oracle,
		store:    store,
		assets:   normalizeUniqueAssets(assets),
		interval: interval,
		ttl:      ttl,
		now:      time.Now,
	}
}

func (w *NECOPriceCacheWorker) PollOnce(ctx context.Context) ([]NECOCachedAssetPrice, error) {
	if w == nil || w.oracle == nil || w.store == nil {
		return nil, ErrNECOPriceOracleNotConfigured
	}
	now := time.Now()
	if w.now != nil {
		now = w.now()
	}
	entries := make([]NECOCachedAssetPrice, 0, len(w.assets))
	for _, asset := range w.assets {
		snapshot, err := w.oracle.AssetUSDPrice(ctx, asset)
		if err != nil {
			return entries, err
		}
		entry := cachedAssetPriceFromSnapshot(snapshot, now, w.ttl)
		if err := w.store.SaveAssetPrice(ctx, entry); err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (w *NECOPriceCacheWorker) Run(ctx context.Context) error {
	if _, err := w.PollOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := w.PollOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func cachedAssetPriceFromSnapshot(snapshot AssetPriceSnapshot, now time.Time, ttl time.Duration) NECOCachedAssetPrice {
	return NECOCachedAssetPrice{
		ID:           snapshot.ID,
		Asset:        normalizeAsset(snapshot.Asset),
		USDAsset:     defaultString(normalizeAsset(snapshot.USDAsset), "USD"),
		USDPrice:     snapshot.USDPrice,
		RouteName:    snapshot.RouteName,
		Source:       snapshot.Source,
		BlockNumber:  snapshot.BlockNumber,
		MinLiquidity: snapshot.MinLiquidity,
		Pairs:        append([]string(nil), snapshot.Pairs...),
		UpdatedAt:    now.Unix(),
		ExpiresAt:    now.Add(ttl).Unix(),
	}
}

type CachedAssetUSDOracle struct {
	store NECOAssetPriceCacheStore
	now   func() time.Time
}

func NewCachedAssetUSDOracle(store NECOAssetPriceCacheStore) *CachedAssetUSDOracle {
	return &CachedAssetUSDOracle{store: store, now: time.Now}
}

func (o *CachedAssetUSDOracle) AssetUSDPrice(ctx context.Context, asset string) (AssetPriceSnapshot, error) {
	if o == nil || o.store == nil {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	entry, ok, err := o.store.GetAssetPrice(ctx, asset)
	if err != nil {
		return AssetPriceSnapshot{}, err
	}
	if !ok {
		return AssetPriceSnapshot{}, ErrNECOPriceOracleNotConfigured
	}
	now := time.Now()
	if o.now != nil {
		now = o.now()
	}
	if !entry.Fresh(now) {
		return AssetPriceSnapshot{}, ErrNECOPriceStale
	}
	return entry.Snapshot(), nil
}

type memoryNECOPriceCacheStore struct {
	entries map[string]NECOCachedAssetPrice
}

func newMemoryNECOPriceCacheStore() *memoryNECOPriceCacheStore {
	return &memoryNECOPriceCacheStore{entries: map[string]NECOCachedAssetPrice{}}
}

func (s *memoryNECOPriceCacheStore) SaveAssetPrice(_ context.Context, entry NECOCachedAssetPrice) error {
	s.entries[normalizeAsset(entry.Asset)] = entry
	return nil
}

func (s *memoryNECOPriceCacheStore) GetAssetPrice(_ context.Context, asset string) (NECOCachedAssetPrice, bool, error) {
	entry, ok := s.entries[normalizeAsset(asset)]
	return entry, ok, nil
}

func normalizeUniqueAssets(assets []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(assets))
	for _, asset := range assets {
		normalized := normalizeAsset(asset)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

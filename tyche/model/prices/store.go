package prices

import (
	"context"
	"encoding/json"
	"ethgo/model"
	"ethgo/tyche"

	"github.com/garyburd/redigo/redis"
)

type RedisStore struct{}

func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) SaveAssetPrice(ctx context.Context, entry tyche.NECOCachedAssetPrice) error {
	_ = ctx
	var red = model.RedisPool.Get()
	defer red.Close()

	entry.Asset = normalizeAsset(entry.Asset)
	entry.USDAsset = normalizeAsset(entry.USDAsset)
	jsonValue, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	hashValues := assetPriceHashValues(entry)

	if err := red.Send("MULTI"); err != nil {
		return err
	}
	if err := red.Send("HSET", append([]interface{}{keys.Asset(entry.Asset)}, hashValues...)...); err != nil {
		return err
	}
	if err := red.Send("SET", keys.AssetJSON(entry.Asset), jsonValue); err != nil {
		return err
	}
	if err := red.Send("SADD", keys.Assets(), entry.Asset); err != nil {
		return err
	}
	if entry.RouteName != "" {
		if err := red.Send("HSET", append([]interface{}{keys.Route(entry.RouteName)}, hashValues...)...); err != nil {
			return err
		}
		if err := red.Send("SET", keys.RouteJSON(entry.RouteName), jsonValue); err != nil {
			return err
		}
		if err := red.Send("SADD", keys.Routes(), normalizeAsset(entry.RouteName)); err != nil {
			return err
		}
	}
	_, err = red.Do("EXEC")
	return err
}

func (s *RedisStore) GetAssetPrice(ctx context.Context, asset string) (tyche.NECOCachedAssetPrice, bool, error) {
	_ = ctx
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Asset(asset)))
	if err != nil {
		return tyche.NECOCachedAssetPrice{}, false, err
	}
	if len(values) == 0 {
		return tyche.NECOCachedAssetPrice{}, false, nil
	}
	entry, err := scanAssetPriceHash(values)
	if err != nil {
		return tyche.NECOCachedAssetPrice{}, false, err
	}
	return entry, true, nil
}

func assetPriceHashValues(entry tyche.NECOCachedAssetPrice) []interface{} {
	pairs, _ := json.Marshal(entry.Pairs)
	return []interface{}{
		"id", entry.ID,
		"asset", entry.Asset,
		"usdAsset", entry.USDAsset,
		"usdPrice", entry.USDPrice,
		"routeName", entry.RouteName,
		"source", entry.Source,
		"blockNumber", entry.BlockNumber,
		"minLiquidityUsd", entry.MinLiquidity,
		"pairs", string(pairs),
		"updatedAt", entry.UpdatedAt,
		"expiresAt", entry.ExpiresAt,
	}
}

func scanAssetPriceHash(values []interface{}) (tyche.NECOCachedAssetPrice, error) {
	var raw struct {
		ID           string `redis:"id"`
		Asset        string `redis:"asset"`
		USDAsset     string `redis:"usdAsset"`
		USDPrice     string `redis:"usdPrice"`
		RouteName    string `redis:"routeName"`
		Source       string `redis:"source"`
		BlockNumber  uint64 `redis:"blockNumber"`
		MinLiquidity string `redis:"minLiquidityUsd"`
		Pairs        string `redis:"pairs"`
		UpdatedAt    int64  `redis:"updatedAt"`
		ExpiresAt    int64  `redis:"expiresAt"`
	}
	if err := redis.ScanStruct(values, &raw); err != nil {
		return tyche.NECOCachedAssetPrice{}, err
	}
	var pairs []string
	if raw.Pairs != "" {
		if err := json.Unmarshal([]byte(raw.Pairs), &pairs); err != nil {
			return tyche.NECOCachedAssetPrice{}, err
		}
	}
	return tyche.NECOCachedAssetPrice{
		ID:           raw.ID,
		Asset:        raw.Asset,
		USDAsset:     raw.USDAsset,
		USDPrice:     raw.USDPrice,
		RouteName:    raw.RouteName,
		Source:       raw.Source,
		BlockNumber:  raw.BlockNumber,
		MinLiquidity: raw.MinLiquidity,
		Pairs:        pairs,
		UpdatedAt:    raw.UpdatedAt,
		ExpiresAt:    raw.ExpiresAt,
	}, nil
}

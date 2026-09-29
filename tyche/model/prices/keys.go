package prices

import (
	"ethgo/model"
	"fmt"
	"strings"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:price", model.RedisPool.Namespace)
}

func (k keygen) Asset(asset string) string {
	return fmt.Sprintf("%s:asset:%s:USD", k.Namespace(), normalizeAsset(asset))
}

func (k keygen) AssetJSON(asset string) string {
	return k.Asset(asset) + ":json"
}

func (k keygen) Route(route string) string {
	return fmt.Sprintf("%s:route:%s", k.Namespace(), normalizeAsset(route))
}

func (k keygen) RouteJSON(route string) string {
	return k.Route(route) + ":json"
}

func (k keygen) Assets() string {
	return fmt.Sprintf("%s:assets", k.Namespace())
}

func (k keygen) Routes() string {
	return fmt.Sprintf("%s:routes", k.Namespace())
}

func normalizeAsset(asset string) string {
	return strings.ToUpper(strings.TrimSpace(asset))
}

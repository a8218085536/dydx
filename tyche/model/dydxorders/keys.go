package dydxorders

import (
	"ethgo/model"
	"fmt"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:dydx_order", model.RedisPool.Namespace)
}

func (k keygen) Entity(id string) string {
	return fmt.Sprintf("%s:entity:%s", k.Namespace(), id)
}

func (k keygen) Pending() string {
	return fmt.Sprintf("%s:status:pending", k.Namespace())
}

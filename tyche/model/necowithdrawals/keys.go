package necowithdrawals

import (
	"ethgo/model"
	"fmt"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:neco:withdrawals", model.RedisPool.Namespace)
}

func (k keygen) Entity(withdrawalID string) string {
	return fmt.Sprintf("%s:%s", k.Namespace(), withdrawalID)
}

package necodeposits

import (
	"ethgo/model"
	"fmt"
	"strings"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:neco:deposits", model.RedisPool.Namespace)
}

func (k keygen) Event(eventID string) string {
	return fmt.Sprintf("%s:event:%s", k.Namespace(), eventID)
}

func (k keygen) Pending(address string) string {
	return fmt.Sprintf("%s:pending:%s", k.Namespace(), strings.ToLower(address))
}

func (k keygen) UserDeposits(userID string) string {
	return fmt.Sprintf("%s:user:%s", k.Namespace(), userID)
}

func (k keygen) StatusDeposits(status string) string {
	return fmt.Sprintf("%s:status:%s", k.Namespace(), status)
}

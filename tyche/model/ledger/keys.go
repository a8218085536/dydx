package ledger

import (
	"ethgo/model"
	"fmt"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:ledger", model.RedisPool.Namespace)
}

func (k keygen) Balance(userID, asset string) string {
	return fmt.Sprintf("%s:balance:%s:%s", k.Namespace(), userID, asset)
}

func (k keygen) Ref(refID string) string {
	return fmt.Sprintf("%s:ref:%s", k.Namespace(), refID)
}

func (k keygen) Deposit(id string) string {
	return fmt.Sprintf("%s:deposit:%s", k.Namespace(), id)
}

func (k keygen) Withdrawal(id string) string {
	return fmt.Sprintf("%s:withdrawal:%s", k.Namespace(), id)
}

func (k keygen) Journal() string {
	return fmt.Sprintf("%s:journal", k.Namespace())
}

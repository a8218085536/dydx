package walletbindings

import (
	"ethgo/model"
	"fmt"
	"strings"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:wallet_binding", model.RedisPool.Namespace)
}

func (k keygen) Entity(id string) string {
	return fmt.Sprintf("%s:entity:%s", k.Namespace(), id)
}

func (k keygen) NECOIndex(address string) string {
	return fmt.Sprintf("%s:index:neco:%s", k.Namespace(), strings.ToLower(address))
}

func (k keygen) DYDXIndex(address string) string {
	return fmt.Sprintf("%s:index:dydx:%s", k.Namespace(), strings.ToLower(address))
}

func (k keygen) UserIndex(userID string) string {
	return fmt.Sprintf("%s:index:user:%s", k.Namespace(), userID)
}

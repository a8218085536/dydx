package orders

import (
	"ethgo/model"
	"fmt"
)

type keygen struct {
}

var keys keygen

func (k *keygen) Namespace() string {
	return fmt.Sprintf("%s:order", model.RedisPool.Namespace)
}

func (k *keygen) NonceAt() string {
	return fmt.Sprintf("%v:nonce_at", model.RedisPool.Namespace)
}

func (k *keygen) Nonce(nonce uint64) string {
	return fmt.Sprintf("%v:nonce:%v", k.Namespace(), nonce)
}

func (k *keygen) Entity(id string) string {
	return fmt.Sprintf("%s:entity:%s", k.Namespace(), id)
}

func (k *keygen) EntityLock(id string) string {
	return fmt.Sprintf("%s:entity:%s:lock", k.Namespace(), id)
}

func (k *keygen) Pending() string {
	return fmt.Sprintf("%s:status:pending", k.Namespace())
}

func (k *keygen) Sent() string {
	return fmt.Sprintf("%s:status:sent", k.Namespace())
}

func (k *keygen) Succeed() string {
	return fmt.Sprintf("%s:status:succeed", k.Namespace())
}

func (k *keygen) Failed() string {
	return fmt.Sprintf("%s:status:failed", k.Namespace())
}

func (k *keygen) Error() string {
	return fmt.Sprintf("%s:status:error", k.Namespace())
}

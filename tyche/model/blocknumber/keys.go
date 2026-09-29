package blocknumber

import (
	"ethgo/model"
	"fmt"
)

type keygen struct {
}

var keys keygen

func (k *keygen) Namespace() string {
	return model.RedisPool.Namespace
}

func (k *keygen) BlockNumber() string {
	return fmt.Sprintf("%v:block_number", k.Namespace())
}

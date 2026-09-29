package orders

import (
	"ethgo/util"
	"sync"
)

var orderLocks []sync.Locker
var size = 8192

func init() {
	for i := 0; i < size; i++ {
		orderLocks = append(orderLocks, new(sync.Mutex))
	}
}

func getLocker(id string) sync.Locker {
	return orderLocks[util.HashStr(id)%uint32(size)]
}

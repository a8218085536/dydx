package tyche

import (
	"sync"
)

type NonceLimiter struct {
	l   sync.RWMutex
	max uint64
}

func NewNonceLimiter(max uint64) *NonceLimiter {
	return &NonceLimiter{max: max}
}

func (q *NonceLimiter) Distance(nonce uint64) int64 {
	q.l.RLock()
	defer q.l.RUnlock()
	return int64(nonce - q.max)
}

func (q *NonceLimiter) Update(nonce uint64) {
	q.l.Lock()
	defer q.l.Unlock()
	if nonce > q.max {
		q.max = nonce
	}
}

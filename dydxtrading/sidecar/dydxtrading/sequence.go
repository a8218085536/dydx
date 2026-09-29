//go:build cgo

package dydxtrading

import (
	"context"
	"sync"
)

type SequenceManager struct {
	upstream AccountProvider
	mu       sync.Mutex
	next     map[string]AccountInfo
}

func NewSequenceManager(upstream AccountProvider) *SequenceManager {
	return &SequenceManager{
		upstream: upstream,
		next:     make(map[string]AccountInfo),
	}
}

func (m *SequenceManager) Account(ctx context.Context, address string) (AccountInfo, error) {
	m.mu.Lock()
	if info, ok := m.next[address]; ok {
		reserved := info
		info.Sequence++
		m.next[address] = info
		m.mu.Unlock()
		return reserved, nil
	}
	m.mu.Unlock()

	info, err := m.upstream.Account(ctx, address)
	if err != nil {
		return AccountInfo{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if cached, ok := m.next[address]; ok {
		reserved := cached
		cached.Sequence++
		m.next[address] = cached
		return reserved, nil
	}
	reserved := info
	info.Sequence++
	m.next[address] = info
	return reserved, nil
}

func (m *SequenceManager) Release(address string, sequence uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.next[address]
	if !ok {
		return
	}
	if info.Sequence == sequence+1 {
		info.Sequence = sequence
		m.next[address] = info
	}
}

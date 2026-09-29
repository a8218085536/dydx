package tyche

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type MemoryDYDXRetainedCollateralStore struct {
	mu       sync.Mutex
	quantums map[string]string
}

func NewMemoryDYDXRetainedCollateralStore() *MemoryDYDXRetainedCollateralStore {
	return &MemoryDYDXRetainedCollateralStore{quantums: map[string]string{}}
}

func (s *MemoryDYDXRetainedCollateralStore) RetainedCollateralQuantums(ctx context.Context, dydxAccount string, subaccountNumber uint32) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.quantums[retainedCollateralKey(dydxAccount, subaccountNumber)]
	if strings.TrimSpace(value) == "" {
		return "0", nil
	}
	return value, nil
}

func (s *MemoryDYDXRetainedCollateralStore) SaveRetainedCollateralQuantums(ctx context.Context, dydxAccount string, subaccountNumber uint32, quantums string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quantums[retainedCollateralKey(dydxAccount, subaccountNumber)] = strings.TrimSpace(quantums)
	return nil
}

func retainedCollateralKey(dydxAccount string, subaccountNumber uint32) string {
	return fmt.Sprintf("%s:%d", strings.ToLower(strings.TrimSpace(dydxAccount)), subaccountNumber)
}

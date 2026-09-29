package necowithdrawals

import (
	"ethgo/tyche"
	"testing"
)

func requireNECOWithdrawalStore(tyche.NECOWithdrawalStore) {}

func TestRedisStoreImplementsNECOWithdrawalStore(t *testing.T) {
	requireNECOWithdrawalStore(NewRedisStore())
}

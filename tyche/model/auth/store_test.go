package auth

import (
	"ethgo/tyche"
	"testing"

	"github.com/garyburd/redigo/redis"
)

func requireAuthStore(tyche.AuthStore)           {}
func requireAuthUserLookup(tyche.AuthUserLookup) {}
func requireAuthChallengeNonceStore(tyche.AuthChallengeNonceStore) {}

func TestRedisStoreImplementsAuthStore(t *testing.T) {
	requireAuthStore(NewRedisStore())
	requireAuthUserLookup(NewRedisStore())
	requireAuthChallengeNonceStore(NewRedisStore())
}

func TestScanStructReadsAuthChallengeRedisFields(t *testing.T) {
	values := []interface{}{
		[]byte("necoAddress"), []byte("0xabc"),
		[]byte("nonce"), []byte("n"),
		[]byte("message"), []byte("hello"),
	}

	var challenge tyche.AuthChallenge
	if err := redis.ScanStruct(values, &challenge); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}

	if challenge.NECOAddress != "0xabc" || challenge.Nonce != "n" || challenge.Message != "hello" {
		t.Fatalf("challenge=%+v, want Redis fields populated", challenge)
	}
}

func TestScanStructReadsAuthUserRedisFields(t *testing.T) {
	values := []interface{}{
		[]byte("userId"), []byte("user-1"),
		[]byte("necoAddress"), []byte("0xabc"),
	}

	var user tyche.AuthUser
	if err := redis.ScanStruct(values, &user); err != nil {
		t.Fatalf("ScanStruct() error = %v", err)
	}

	if user.UserID != "user-1" || user.NECOAddress != "0xabc" {
		t.Fatalf("user=%+v, want Redis fields populated", user)
	}
}

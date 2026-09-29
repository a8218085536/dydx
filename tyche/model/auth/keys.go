package auth

import (
	"ethgo/model"
	"fmt"
	"strings"
)

type keygen struct{}

var keys keygen

func (k keygen) Namespace() string {
	return fmt.Sprintf("%s:auth", model.RedisPool.Namespace)
}

func (k keygen) Challenge(address string) string {
	return fmt.Sprintf("%s:challenge:%s", k.Namespace(), strings.ToLower(address))
}

func (k keygen) ChallengeNonce(address, nonce string) string {
	return fmt.Sprintf("%s:challenge:%s:nonce:%s", k.Namespace(), strings.ToLower(address), nonce)
}

func (k keygen) User(address string) string {
	return fmt.Sprintf("%s:user:%s", k.Namespace(), strings.ToLower(address))
}

func (k keygen) Session(token string) string {
	return fmt.Sprintf("%s:session:%s", k.Namespace(), token)
}

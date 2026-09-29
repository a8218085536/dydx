package auth

import (
	"context"
	"ethgo/model"
	"ethgo/tyche"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/google/uuid"
)

type RedisStore struct{}

func NewRedisStore() *RedisStore {
	return &RedisStore{}
}

func (s *RedisStore) SaveChallenge(ctx context.Context, challenge tyche.AuthChallenge) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	now := time.Now().Unix()
	if _, err := red.Do("HSET", keys.Challenge(challenge.NECOAddress),
		"necoAddress", challenge.NECOAddress,
		"nonce", challenge.Nonce,
		"message", challenge.Message,
		"createdAt", now,
		"updatedAt", now,
	); err != nil {
		return err
	}
	if _, err := red.Do("HSET", keys.ChallengeNonce(challenge.NECOAddress, challenge.Nonce),
		"necoAddress", challenge.NECOAddress,
		"nonce", challenge.Nonce,
		"message", challenge.Message,
		"createdAt", now,
		"updatedAt", now,
	); err != nil {
		return err
	}
	return nil
}

func (s *RedisStore) GetChallenge(ctx context.Context, necoAddress string) (tyche.AuthChallenge, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.Challenge(necoAddress)))
	if err != nil {
		return tyche.AuthChallenge{}, err
	}
	if len(values) == 0 {
		return tyche.AuthChallenge{}, tyche.ErrAuthChallengeNotFound
	}
	var challenge tyche.AuthChallenge
	if err := redis.ScanStruct(values, &challenge); err != nil {
		return tyche.AuthChallenge{}, err
	}
	return challenge, nil
}

func (s *RedisStore) GetChallengeByNonce(ctx context.Context, necoAddress, nonce string) (tyche.AuthChallenge, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.ChallengeNonce(necoAddress, nonce)))
	if err != nil {
		return tyche.AuthChallenge{}, err
	}
	if len(values) == 0 {
		return tyche.AuthChallenge{}, tyche.ErrAuthChallengeNotFound
	}
	var challenge tyche.AuthChallenge
	if err := redis.ScanStruct(values, &challenge); err != nil {
		return tyche.AuthChallenge{}, err
	}
	return challenge, nil
}

func (s *RedisStore) GetOrCreateUser(ctx context.Context, necoAddress string) (tyche.AuthUser, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	userKey := keys.User(necoAddress)
	values, err := redis.Values(red.Do("HGETALL", userKey))
	if err != nil {
		return tyche.AuthUser{}, err
	}
	if len(values) > 0 {
		return scanUser(values)
	}

	user := tyche.AuthUser{
		UserID:      uuid.NewString(),
		NECOAddress: necoAddress,
	}
	now := time.Now().Unix()
	_, err = red.Do("HSET", userKey,
		"userId", user.UserID,
		"necoAddress", user.NECOAddress,
		"createdAt", now,
		"updatedAt", now,
	)
	return user, err
}

func (s *RedisStore) SaveUser(ctx context.Context, user tyche.AuthUser) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	now := time.Now().Unix()
	_, err := red.Do("HSET", keys.User(user.NECOAddress),
		"userId", user.UserID,
		"necoAddress", user.NECOAddress,
		"updatedAt", now,
	)
	return err
}

func (s *RedisStore) GetUserByNECOAddress(ctx context.Context, necoAddress string) (tyche.AuthUser, bool, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	values, err := redis.Values(red.Do("HGETALL", keys.User(necoAddress)))
	if err != nil {
		return tyche.AuthUser{}, false, err
	}
	if len(values) == 0 {
		return tyche.AuthUser{}, false, nil
	}
	user, err := scanUser(values)
	if err != nil {
		return tyche.AuthUser{}, false, err
	}
	return user, true, nil
}

func (s *RedisStore) SaveSession(ctx context.Context, token, userID string) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("HSET", keys.Session(token),
		"token", token,
		"userId", userID,
		"createdAt", time.Now().Unix(),
	)
	return err
}

func scanUser(values []interface{}) (tyche.AuthUser, error) {
	var user tyche.AuthUser
	if err := redis.ScanStruct(values, &user); err != nil {
		return tyche.AuthUser{}, err
	}
	return user, nil
}

package redisx

import (
	"errors"
	"ethgo/model"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/google/uuid"
)

const (
	tolerance       = 500 // milliseconds
	millisPerSecond = 1000
	retainCmd       = `if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
    return "OK"
else
    return redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2])
end`
	releaseCmd = `if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end`
)

var ErrNotObtained = errors.New("redislock: not obtained")

// A RedisLock is a redis lock.
type RedisLock struct {
	seconds uint32
	key     string
	id      string
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewRedisLock returns a RedisLock.
func NewRedisLock(key string) *RedisLock {
	return &RedisLock{
		key: key,
		id:  uuid.New().String(),
	}
}

// Acquire acquires the lock.
func (rl *RedisLock) Acquire() (bool, error) {
	seconds := atomic.LoadUint32(&rl.seconds)
	red := model.RedisPool.Get()
	defer red.Close()

	var expireInMilliSeconds = strconv.Itoa(int(seconds)*millisPerSecond + tolerance)
	resp, err := redis.String(red.Do("Eval", retainCmd, 1, rl.key, rl.id, expireInMilliSeconds))

	if err == nil {
		return resp == "OK", nil
	} else if err == redis.ErrNil {
		return false, ErrNotObtained
	} else {
		return false, err
	}
}

// Release releases the lock.
func (rl *RedisLock) Release() (bool, error) {
	red := model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("Eval", releaseCmd, 1, rl.key, rl.id)
	if err == nil {
		return true, nil
	} else if err == redis.ErrNil {
		return true, nil
	} else {
		return false, err
	}
}

// SetExpire sets the expiration.
func (rl *RedisLock) SetExpire(seconds int) *RedisLock {
	atomic.StoreUint32(&rl.seconds, uint32(seconds))
	return rl
}

package model

import (
	"time"

	"github.com/garyburd/redigo/redis"
)

type redisPool struct {
	*redis.Pool
	Namespace string
}

var RedisPool *redisPool

func Init(c *Config) error {
	RedisPool = &redisPool{
		Namespace: c.Namespace,
		Pool: &redis.Pool{
			MaxIdle:     c.MaxIdle,
			MaxActive:   c.MaxActive,
			IdleTimeout: time.Duration(c.IdleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				return redis.DialURL(c.URI)
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				if time.Since(t) < time.Minute {
					return nil
				}
				_, err := c.Do("PING")
				return err
			},
		},
	}

	return ping()
}

func Dispose() {
	if RedisPool != nil {
		RedisPool.Close()
	}
}

func ping() error {
	var red = RedisPool.Get()
	defer red.Close()

	red.Send("PING")
	return red.Flush()
}

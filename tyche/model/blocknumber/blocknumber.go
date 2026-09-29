package blocknumber

import (
	"ethgo/model"

	"github.com/garyburd/redigo/redis"
)

func Get() (uint64, error) {
	var red = model.RedisPool.Get()
	defer red.Close()

	return redis.Uint64(red.Do("GET", keys.BlockNumber()))
}

func Set(value uint64) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("SET", keys.BlockNumber(), value)
	return err
}

func SetNX(value uint64) error {
	var red = model.RedisPool.Get()
	defer red.Close()

	_, err := red.Do("SETNX", keys.BlockNumber(), value+1)
	return err
}

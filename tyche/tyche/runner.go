package tyche

import (
	"context"
	"ethgo/eth"
)

type Runner interface {
	Init(context.Context) error
	Run(context.Context) error
}

type EVMTransactor interface {
	Call(context.Context, Caller) ([]interface{}, error)
	Transact(context.Context, string, Transactor) error
	SetAccount(string, string) error
}

type DYDXSubmitter interface {
	Submit(context.Context, DYDXIntent) error
}

type EVMRunner = Tyche

func NewRunner(backend eth.Backend, conf *Config) (Runner, error) {
	switch conf.ChainType {
	case ChainTypeDYDX:
		return NewDYDXRunner(conf), nil
	default:
		return New(backend, conf), nil
	}
}

package app

import (
	"context"
	"ethgo/tyche"
)

type nilDYDXSubmitter struct{}

func (nilDYDXSubmitter) Submit(context.Context, tyche.DYDXIntent) error {
	return tyche.ErrDYDXBroadcastNotConfigured
}

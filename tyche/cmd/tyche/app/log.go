package app

import (
	"ethgo/tyche"

	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func SetLogger(logger *zap.SugaredLogger) {
	log = logger
	tyche.SetLogger(logger)
}

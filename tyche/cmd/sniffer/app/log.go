package app

import (
	"ethgo/sniffer"

	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func SetLogger(logger *zap.SugaredLogger) {
	log = logger
	sniffer.SetLogger(log)
}

package sniffer

import (
	"ethgo/util/logx"

	"go.uber.org/zap"
)

var log *zap.SugaredLogger = logx.Default("[SNIFFER]")

func SetLogger(logger *zap.SugaredLogger) {
	log = logger
}

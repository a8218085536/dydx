package tyche

import (
	"ethgo/util/logx"

	"go.uber.org/zap"
)

var log *zap.SugaredLogger = logx.Default("[TYCHE]")

func SetLogger(logger *zap.SugaredLogger) {
	log = logger
}

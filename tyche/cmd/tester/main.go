package main

import (
	"encoding/json"
	"ethgo/sniffer"
	"ethgo/util/logx"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var log *zap.SugaredLogger

func initLogger() {
	log = logx.New(&logx.Config{
		Filemame:  "./logs/tester.log",
		Named:     "[TESTER]",
		Level:     "debug",
		LocalTime: true,
	})
	sniffer.SetLogger(log)
}

func main() {
	initLogger()

	log.Info("启动")
	defer log.Info("退出")

	app := gin.New()
	app.RedirectTrailingSlash = false

	app.POST("/event/dispatch", func(ctx *gin.Context) {

		var request = new(gin.H)
		if err := ctx.BindJSON(request); err != nil {
			ctx.JSON(http.StatusOK, gin.H{"code": http.StatusBadRequest, "Message": err.Error()})
			return
		}

		reply, _ := json.MarshalIndent(request, "", "  ")
		log.Debug(string(reply))

		ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "succ"})
	})

	app.POST("/event/error", func(ctx *gin.Context) {
		var request = new(gin.H)
		if err := ctx.BindJSON(request); err != nil {
			ctx.JSON(http.StatusOK, gin.H{"code": http.StatusBadRequest, "Message": err.Error()})
			return
		}

		reply, _ := json.MarshalIndent(request, "", "  ")
		log.Debugf("[ERROR]: %v", string(reply))

		ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "succ"})
	})

	app.POST("/event/succeed", func(ctx *gin.Context) {
		var request = new(gin.H)
		if err := ctx.BindJSON(request); err != nil {
			ctx.JSON(http.StatusOK, gin.H{"code": http.StatusBadRequest, "Message": err.Error()})
			return
		}

		reply, _ := json.MarshalIndent(request, "", "  ")
		log.Debugf("[SUCCEED]: %v", string(reply))

		ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "succ"})
	})

	app.POST("/event/failed", func(ctx *gin.Context) {
		var request = new(gin.H)
		if err := ctx.BindJSON(request); err != nil {
			ctx.JSON(http.StatusOK, gin.H{"code": http.StatusBadRequest, "Message": err.Error()})
			return
		}

		reply, _ := json.MarshalIndent(request, "", "  ")
		log.Debugf("[FAILED]: %v", string(reply))

		ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "succ"})
	})

	app.Run("0.0.0.0:8081")
}

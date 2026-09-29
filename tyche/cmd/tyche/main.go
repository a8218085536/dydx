package main

import (
	"context"
	"ethgo/cmd/tyche/app"
	"ethgo/util/logx"
	"os"
	"os/signal"
	"syscall"
)

func run(c *app.Config) {
	app := app.New(c)
	ctx, cancel := context.WithCancel(context.Background())
	if err := app.Init(ctx); err != nil {
		panic(err)
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
		<-ch
		defer cancel()
	}()

	if err := app.Run(ctx); err != nil {
		panic(err)
	}
}

// @title Tyche 服务
// @version 1.0
// @description Tyche 服务的目标是简化区块链 Dapp 的开发。
// @description
// @description 你不必关心, 甚至不必理解 gasLimit， gasPrice， nonce 等区块链相关的技术细节。我们通过类似支付系统的交互流程（请求/回调）， 帮助你快速构建自己的 Dapp 应用。
func main() {

	c, err := app.NewConfig("./tyche.toml")
	if err != nil {
		panic(err)
	}

	var log = logx.New(c.Logger)
	app.SetLogger(log)

	log.Info("启动")
	defer log.Info("退出")

	run(c)
}

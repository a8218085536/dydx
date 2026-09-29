package main

import (
	"context"
	"ethgo/cmd/sniffer/app"
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
		app.Close()
	}()

	if err := app.Run(ctx); err != nil {
		panic(err)
	}
}

func main() {
	c, err := app.NewConfig("./sniffer.toml")
	if err != nil {
		panic(err)
	}

	log := logx.New(c.Logger)
	app.SetLogger(log)

	log.Info("启动")
	defer log.Info("退出")

	run(c)
}

// 354  274

// 346  222

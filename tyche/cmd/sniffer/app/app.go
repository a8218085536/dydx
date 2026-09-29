package app

import (
	"context"
	"encoding/json"
	"errors"
	"ethgo/eth"
	"ethgo/model"
	"ethgo/sniffer"
	"ethgo/util"
	"fmt"
	"net/http"
)

type App struct {
	backend eth.Backend
	base    *sniffer.Sniffer
	conf    *Config
}

func New(conf *Config) *App {
	return &App{conf: conf}
}

func (app *App) Init(ctx context.Context) error {
	err := model.Init(app.conf.Redis)
	if err != nil {
		return err
	}

	app.backend, err = eth.New(app.conf.Backend)
	if err != nil {
		return err
	}

	app.base, err = sniffer.New(app.conf.Sniffer)
	if err != nil {
		return err
	}

	return nil
}

func (app *App) Close() {
	if app.backend != nil {
		defer app.backend.Close()
	}
	defer model.Dispose()
}

func (app *App) Run(ctx context.Context) error {
	app.base.SetEventHandler(app.dispatchEvent)
	return app.base.Run(ctx, app.backend)
}

func (app *App) dispatchEvent(event *sniffer.Event) error {

	log.Debugf("捕获事件: %v, ContractName=%v Address=%v ChainID=%v BlockNumber=%v, TxHash=%v, TxIndex=%v",
		event.Name, event.ContractName, event.Address, event.ChainID, event.BlockNumber, event.TxHash, event.TxIndex)

	body, err := util.Post(app.conf.Sniffer.Callback, event)
	if err != nil {
		return err
	}

	var res Response
	if err := json.Unmarshal(body, &res); err != nil {
		return err
	}

	log.Debugf("应答: %v", string(body))

	if res.Code != http.StatusOK {
		if res.Message == "" {
			res.Message = fmt.Sprintf("%v", res.Code)
		}
		return errors.New(res.Message)
	}

	return nil
}

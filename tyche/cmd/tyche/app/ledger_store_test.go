package app

import (
	"ethgo/model"
	"ethgo/tyche"
	"testing"
)

func TestNewLedgerServiceUsesRedisStore(t *testing.T) {
	app := &App{conf: &Config{
		Redis: &model.Config{Namespace: "test"},
		Tyche: appTestConfig(tyche.ChainTypeDYDX),
	}}

	ledger := app.newLedgerService()
	if ledger == nil {
		t.Fatal("newLedgerService() returned nil")
	}
}

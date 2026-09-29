package tyche

import (
	"context"
	"testing"
)

func TestNewRunnerSelectsEVMByDefault(t *testing.T) {
	runner, err := NewRunner(nil, minimalConfig())
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if _, ok := runner.(*EVMRunner); !ok {
		t.Fatalf("runner = %T, want *EVMRunner", runner)
	}
}

func TestNewRunnerSelectsDYDX(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = "dydx1exampleaccount"
	c.PrivateKey = "dydx-private-key-placeholder"
	c.EstimatorJS = ""

	runner, err := NewRunner(nil, c)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if _, ok := runner.(*DYDXRunner); !ok {
		t.Fatalf("runner = %T, want *DYDXRunner", runner)
	}
}

func TestDYDXRunnerInitDoesNotUseEVMBackend(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = "dydx1exampleaccount"
	c.PrivateKey = "dydx-private-key-placeholder"
	c.EstimatorJS = ""

	runner, err := NewRunner(nil, c)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if err := runner.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
}

func TestNewDYDXRunnerUsesHTTPBroadcasterConfig(t *testing.T) {
	c := minimalConfig()
	c.ChainType = ChainTypeDYDX
	c.Account = "dydx1sender"
	c.PrivateKey = "dydx-private-key-placeholder"
	c.EstimatorJS = ""
	c.DYDXBroadcastURL = "http://127.0.0.1:8090/dydx/broadcast"

	runner := NewDYDXRunner(c)
	broadcaster, ok := runner.broadcaster.(*DYDXHTTPBroadcaster)
	if !ok {
		t.Fatalf("broadcaster = %T, want *DYDXHTTPBroadcaster", runner.broadcaster)
	}
	if broadcaster.conf.URL != c.DYDXBroadcastURL {
		t.Fatalf("URL = %q, want %q", broadcaster.conf.URL, c.DYDXBroadcastURL)
	}
	if broadcaster.conf.Account != c.Account {
		t.Fatalf("Account = %q, want %q", broadcaster.conf.Account, c.Account)
	}
}

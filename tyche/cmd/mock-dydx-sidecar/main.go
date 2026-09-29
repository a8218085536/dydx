package main

import (
	"encoding/json"
	"errors"
	"ethgo/tyche"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

type config struct {
	Listen  string
	Sidecar tyche.MockDYDXSidecarConfig
}

type eventFile struct {
	Fills                 []tyche.TradingFillSyncRequest        `json:"fills"`
	OrderStatuses         []tyche.TradingOrderStatusSyncRequest `json:"orderStatuses"`
	Positions             []tyche.TradingPositionSyncRequest    `json:"positions"`
	NextFillsCursor       string                                `json:"nextFillsCursor"`
	NextOrderStatusCursor string                                `json:"nextOrderStatusCursor"`
	NextPositionsCursor   string                                `json:"nextPositionsCursor"`
}

func main() {
	conf, err := parseConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	sidecar := tyche.NewMockDYDXSidecar(conf.Sidecar)
	server := &http.Server{
		Addr:    conf.Listen,
		Handler: sidecar.Handler(),
	}
	log.Printf("mock dydx sidecar listening on %s", conf.Listen)
	log.Fatal(server.ListenAndServe())
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("mock-dydx-sidecar", flag.ContinueOnError)
	conf := config{}
	fs.StringVar(&conf.Listen, "listen", "127.0.0.1:8090", "listen address")
	fs.StringVar(&conf.Sidecar.SubmitStatus, "submit-status", tyche.TradingOrderStatusSubmitted, "submit status: submitted or rejected")
	fs.StringVar(&conf.Sidecar.RejectReason, "reject-reason", "", "reject reason returned with rejected submit status")
	fs.StringVar(&conf.Sidecar.TxHash, "tx-hash", "mock-dydx-tx", "mock tx hash")
	fs.StringVar(&conf.Sidecar.WalletAddressPrefix, "wallet-prefix", "dydx1mock", "mock dydx wallet address prefix")
	var eventsPath string
	fs.StringVar(&eventsPath, "events", "", "optional JSON file with fills, orderStatuses and positions")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	switch conf.Sidecar.SubmitStatus {
	case tyche.TradingOrderStatusSubmitted, tyche.TradingOrderStatusRejected:
	default:
		return config{}, fmt.Errorf("unsupported submit status %q", conf.Sidecar.SubmitStatus)
	}
	if eventsPath != "" {
		if err := loadEvents(eventsPath, &conf.Sidecar); err != nil {
			return config{}, err
		}
	}
	return conf, nil
}

func loadEvents(path string, sidecar *tyche.MockDYDXSidecarConfig) error {
	if path == "" {
		return errors.New("events path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var events eventFile
	if err := json.Unmarshal(raw, &events); err != nil {
		return err
	}
	sidecar.Fills = events.Fills
	sidecar.OrderStatuses = events.OrderStatuses
	sidecar.Positions = events.Positions
	sidecar.NextFillsCursor = events.NextFillsCursor
	sidecar.NextOrderStatusCursor = events.NextOrderStatusCursor
	sidecar.NextPositionsCursor = events.NextPositionsCursor
	return nil
}

package main

import (
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	conf, err := parseConfig([]string{})

	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if conf.Listen != "127.0.0.1:8090" {
		t.Fatalf("Listen = %q", conf.Listen)
	}
	if conf.Sidecar.TxHash != "mock-dydx-tx" {
		t.Fatalf("TxHash = %q", conf.Sidecar.TxHash)
	}
	if conf.Sidecar.WalletAddressPrefix != "dydx1mock" {
		t.Fatalf("WalletAddressPrefix = %q", conf.Sidecar.WalletAddressPrefix)
	}
}

func TestParseConfigAcceptsSubmitStatus(t *testing.T) {
	conf, err := parseConfig([]string{
		"-listen", "0.0.0.0:18090",
		"-submit-status", "rejected",
		"-reject-reason", "mock rejected",
		"-tx-hash", "tx-1",
	})

	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if conf.Listen != "0.0.0.0:18090" {
		t.Fatalf("Listen = %q", conf.Listen)
	}
	if conf.Sidecar.SubmitStatus != "rejected" || conf.Sidecar.RejectReason != "mock rejected" || conf.Sidecar.TxHash != "tx-1" {
		t.Fatalf("Sidecar = %+v", conf.Sidecar)
	}
}

func TestParseConfigRejectsUnsupportedStatus(t *testing.T) {
	_, err := parseConfig([]string{"-submit-status", "bad"})

	if err == nil {
		t.Fatal("expected error")
	}
}

//go:build cgo

package main

import (
	"errors"
	"flydex/dydxtrading/sidecar/dydxtrading"
	"strings"
	"testing"
	"time"
)

const testSystemMnemonic = "circle music snake select deal march this romance until often welcome rich staff trigger drip exit there reopen denial insect hockey just wealth process"

func TestEnvOrDefaultUsesFallback(t *testing.T) {
	t.Setenv("DYDX_TEST_EMPTY", "")

	if got := envOrDefault("DYDX_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault() = %q, want fallback", got)
	}
}

func TestValidateListenAddressAllowsLoopback(t *testing.T) {
	if err := validateListenAddress("127.0.0.1:8090"); err != nil {
		t.Fatalf("validateListenAddress(loopback) error = %v", err)
	}
	if err := validateListenAddress("localhost:8090"); err != nil {
		t.Fatalf("validateListenAddress(localhost) error = %v", err)
	}
}

func TestValidateListenAddressRejectsRemoteByDefault(t *testing.T) {
	err := validateListenAddress("0.0.0.0:8090")

	var listenErr *listenAddressError
	if !errors.As(err, &listenErr) {
		t.Fatalf("validateListenAddress(remote) error = %v, want listenAddressError", err)
	}
}

func TestValidateListenAddressAllowsRemoteWhenExplicitlyEnabled(t *testing.T) {
	t.Setenv("DYDX_ALLOW_REMOTE_LISTEN", "true")

	if err := validateListenAddress("0.0.0.0:8090"); err != nil {
		t.Fatalf("validateListenAddress(remote allowed) error = %v", err)
	}
}

func TestDurationEnvOrDefaultParsesDuration(t *testing.T) {
	t.Setenv("DYDX_TEST_DURATION", "250ms")

	if got := durationEnvOrDefault("DYDX_TEST_DURATION", time.Second); got != 250*time.Millisecond {
		t.Fatalf("durationEnvOrDefault() = %s, want 250ms", got)
	}
}

func TestBoolEnvParsesTruthyValues(t *testing.T) {
	t.Setenv("DYDX_TEST_BOOL", "true")

	if !boolEnv("DYDX_TEST_BOOL") {
		t.Fatal("boolEnv() = false, want true")
	}
}

func TestNewGasTopUpperAcceptsSystemMnemonicWithoutTopUp(t *testing.T) {
	t.Setenv("DYDX_SYSTEM_MNEMONIC", testSystemMnemonic)
	t.Setenv("DYDX_GAS_TOP_UP_ENABLED", "false")

	systemAddress, systemKey, gasTopUpper, err := newGasTopUpper("", nil, nil, nil, nil, dydxtrading.DefaultTxGasLimit, nil)

	if err != nil {
		t.Fatalf("newGasTopUpper() error = %v", err)
	}
	if gasTopUpper != nil {
		t.Fatalf("gasTopUpper = %T, want nil when disabled", gasTopUpper)
	}
	if systemKey == nil {
		t.Fatal("systemKey = nil, want configured private key for subaccount deposit")
	}
	if !strings.HasPrefix(systemAddress, "dydx1") {
		t.Fatalf("systemAddress = %q, want dydx bech32 address", systemAddress)
	}
}

func TestNewGasTopUpperRejectsAmbiguousSystemKeySources(t *testing.T) {
	t.Setenv("DYDX_SYSTEM_PRIVATE_KEY_HEX", "1b5884fab5c22aeffef369f3454076cec534c75a4ee71add1245e2c8342994a2")
	t.Setenv("DYDX_SYSTEM_MNEMONIC", testSystemMnemonic)

	_, _, _, err := newGasTopUpper("", nil, nil, nil, nil, dydxtrading.DefaultTxGasLimit, nil)

	if !errors.Is(err, dydxtrading.ErrSystemPrivateKeyAmbiguous) {
		t.Fatalf("newGasTopUpper() error = %v, want ErrSystemPrivateKeyAmbiguous", err)
	}
}

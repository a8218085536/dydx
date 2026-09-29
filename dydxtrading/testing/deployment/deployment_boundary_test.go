package deployment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneModuleHasPrivateDeployment(t *testing.T) {
	root := moduleRoot(t)
	goMod := readFile(t, root, "go.mod")
	if !strings.Contains(goMod, "module flydex/dydxtrading") {
		t.Fatalf("go.mod must define standalone flydex/dydxtrading module")
	}

	dockerfile := readFile(t, root, "Dockerfile")
	for _, want := range []string{
		"./cmd/dydxtrading",
		"/bin/dydxtrading",
		`ENTRYPOINT ["/bin/dydxtrading"]`,
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}

	compose := readFile(t, root, "docker-compose.yml")
	for _, want := range []string{
		"dydxtrading:",
		"DYDX_TRADING_LISTEN=${DYDX_TRADING_LISTEN:-127.0.0.1:8090}",
		"DYDX_REST_URL=",
		"DYDX_TX_GAS_LIMIT=",
		"DYDX_TX_FEE_AMOUNT=",
		"DYDX_PROXY_WALLET_KEYSTORE_PATH=",
		"DYDX_PROXY_WALLET_KEYSTORE_SECRET=",
		"TYCHE_URL=",
		"network_mode: host",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("docker-compose.yml missing %q", want)
		}
	}
	if strings.Contains(compose, "\n    ports:") || strings.Contains(compose, "8090:8090") {
		t.Fatal("standalone dydxtrading compose must not publish port 8090 by default")
	}
	if strings.Contains(compose, "DYDX_ALLOW_REMOTE_LISTEN=true") || strings.Contains(compose, "0.0.0.0:8090") {
		t.Fatal("standalone dydxtrading compose must keep loopback-only listening by default")
	}
	if strings.Contains(compose, "DYDX_PRIVATE_KEY_HEX") {
		t.Fatal("standalone dydxtrading compose must not configure a global trading private key")
	}
}

func TestProtocolChainDeploymentDoesNotStartStandaloneDYDXTrading(t *testing.T) {
	protocolRoot := filepath.Clean(filepath.Join(moduleRoot(t), "..", "protocol"))
	for _, file := range []string{
		"Dockerfile",
		"docker-compose.yml",
		filepath.Join("testing", "testnet-local", "Dockerfile"),
		filepath.Join("cmd", "dydxtrading"),
		filepath.Join("sidecar", "dydxtrading"),
	} {
		path := filepath.Join(protocolRoot, file)
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			t.Fatalf("protocol %s references standalone dydxtrading deployment", file)
		} else if err != nil && os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Fatalf("Stat(%s) error = %v", file, err)
		}
		body := readFile(t, protocolRoot, file)
		if strings.Contains(body, "cmd/dydxtrading") || strings.Contains(body, "Dockerfile.dydxtrading") {
			t.Fatalf("protocol %s references standalone dydxtrading deployment", file)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func readFile(t *testing.T, root, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return string(body)
}

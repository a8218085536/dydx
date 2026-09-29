package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateTycheScriptRefreshesNECOPriceCacheWhenInstalled(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "update-tyche-dydx-from-tmp.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	script := string(raw)

	for _, want := range []string{
		"PRICE_CACHE_SERVICE_NAME",
		"install-neco-price-cache-systemd.sh",
		"NECO_PRICE_CACHE_START_NOW=1",
		"systemctl list-unit-files \"$PRICE_CACHE_SERVICE_NAME\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("update script is missing %q", want)
		}
	}
}

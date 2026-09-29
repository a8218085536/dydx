package tyche

import (
	"testing"
)

func TestAssetMapperResolvesNECOTokenToDYDXAssetWithDecimals(t *testing.T) {
	mapper := NewAssetMapper([]AssetMapping{
		{
			NECOTokenAddress: "0x0000000000000000000000000000000000000ded",
			NECOAsset:        "DEDX",
			DYDXAsset:        "DEDX",
			Decimals:         18,
			MinUnit:          "1000000000000000000",
		},
	})

	mapping, ok := mapper.ByNECOToken("0x0000000000000000000000000000000000000DED")

	if !ok {
		t.Fatal("ByNECOToken() ok=false, want true")
	}
	if mapping.NECOAsset != "DEDX" || mapping.DYDXAsset != "DEDX" {
		t.Fatalf("mapping = %+v, want DEDX->DEDX", mapping)
	}
	if mapping.Decimals != 18 || mapping.MinUnit != "1000000000000000000" {
		t.Fatalf("mapping decimals/minUnit = %+v", mapping)
	}
}

func TestAssetMapperRejectsAmountBelowMinUnit(t *testing.T) {
	mapper := NewAssetMapper([]AssetMapping{
		{
			NECOAsset: "USDT",
			DYDXAsset: "USDT",
			Decimals:  6,
			MinUnit:   "1000000",
		},
	})

	err := mapper.ValidateNECOAmount("USDT", "999999")

	if err != ErrAssetAmountBelowMinUnit {
		t.Fatalf("ValidateNECOAmount() error = %v, want %v", err, ErrAssetAmountBelowMinUnit)
	}
}

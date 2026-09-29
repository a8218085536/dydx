package main

import (
	"ethgo/tyche"
	"reflect"
	"testing"
)

func TestPriceCacheAssetsFromConfigUsesMappingsAndRoutes(t *testing.T) {
	assets := priceCacheAssetsFromConfig(&tyche.Config{
		AssetMappings: []tyche.AssetMapping{
			{NECOAsset: "DEDX", EnabledForTrading: true},
			{NECOAsset: "NECO", EnabledForTrading: false},
		},
		PriceRoutes: []tyche.PriceRouteConfig{
			{Asset: "USDT"},
			{Asset: "dedx"},
		},
	}, "")

	want := []string{"DEDX", "USDT"}
	if !reflect.DeepEqual(assets, want) {
		t.Fatalf("assets = %+v, want %+v", assets, want)
	}
}

func TestPriceCacheAssetsFromFlagOverridesConfig(t *testing.T) {
	assets := priceCacheAssetsFromConfig(&tyche.Config{
		AssetMappings: []tyche.AssetMapping{{NECOAsset: "DEDX", EnabledForTrading: true}},
	}, "usdt, dedx")

	want := []string{"DEDX", "USDT"}
	if !reflect.DeepEqual(assets, want) {
		t.Fatalf("assets = %+v, want %+v", assets, want)
	}
}

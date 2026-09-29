package tyche

import (
	"errors"
	"math/big"
)

var ErrAssetAmountBelowMinUnit = errors.New("asset amount below min unit")

type AssetMapping struct {
	NECOTokenAddress  string `json:"necoTokenAddress" toml:"necoTokenAddress"`
	NECOAsset         string `json:"necoAsset" toml:"necoAsset"`
	DYDXAsset         string `json:"dydxAsset" toml:"dydxAsset"`
	Decimals          uint8  `json:"decimals" toml:"decimals"`
	MinUnit           string `json:"minUnit" toml:"minUnit"`
	PriceRoute        string `json:"priceRoute,omitempty" toml:"priceRoute"`
	EnabledForTrading bool   `json:"enabledForTrading" toml:"enabledForTrading"`
}

type AssetMapper struct {
	byToken map[string]AssetMapping
	byAsset map[string]AssetMapping
}

func NewAssetMapper(mappings []AssetMapping) *AssetMapper {
	mapper := &AssetMapper{
		byToken: map[string]AssetMapping{},
		byAsset: map[string]AssetMapping{},
	}
	for _, mapping := range mappings {
		if mapping.NECOTokenAddress != "" {
			mapper.byToken[normalizeAddress(mapping.NECOTokenAddress)] = mapping
		}
		if mapping.NECOAsset != "" {
			mapper.byAsset[normalizeAsset(mapping.NECOAsset)] = mapping
		}
	}
	return mapper
}

func (m *AssetMapper) ByNECOToken(address string) (AssetMapping, bool) {
	mapping, ok := m.byToken[normalizeAddress(address)]
	return mapping, ok
}

func (m *AssetMapper) ByNECOAsset(asset string) (AssetMapping, bool) {
	mapping, ok := m.byAsset[normalizeAsset(asset)]
	return mapping, ok
}

func (m *AssetMapper) ValidateNECOAmount(asset, amount string) error {
	mapping, ok := m.byAsset[asset]
	if !ok || mapping.MinUnit == "" {
		return nil
	}
	value, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return errors.New("invalid asset amount")
	}
	minUnit, ok := new(big.Int).SetString(mapping.MinUnit, 10)
	if !ok {
		return errors.New("invalid asset min unit")
	}
	if value.Cmp(minUnit) < 0 {
		return ErrAssetAmountBelowMinUnit
	}
	return nil
}

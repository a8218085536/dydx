package tyche

import (
	"context"
	"errors"
	"math/big"
)

var ErrTradingBalanceAssetNotConfigured = errors.New("trading balance asset not configured")

type NECOTokenBalanceReader interface {
	BalanceOf(context.Context, string, string) (string, error)
}

type TradingBalanceOverviewRequest struct {
	UserID      string `json:"userId"`
	NECOAddress string `json:"necoAddress"`
	Asset       string `json:"asset"`
}

type TradingBalanceOverview struct {
	UserID              string `json:"userId"`
	NECOAddress         string `json:"necoAddress"`
	Asset               string `json:"asset"`
	NECOTokenAddress    string `json:"necoTokenAddress"`
	LedgerAvailable     string `json:"ledgerAvailable"`
	LedgerFrozen        string `json:"ledgerFrozen"`
	NECOWalletAvailable string `json:"necoWalletAvailable"`
	DisplayAvailable    string `json:"displayAvailable"`
}

type TradingBalanceService struct {
	ledger   *LedgerService
	reader   NECOTokenBalanceReader
	mapper   *AssetMapper
	mappings []AssetMapping
}

func NewTradingBalanceService(ledger *LedgerService, reader NECOTokenBalanceReader, mappings []AssetMapping) *TradingBalanceService {
	copiedMappings := append([]AssetMapping(nil), mappings...)
	return &TradingBalanceService{
		ledger:   ledger,
		reader:   reader,
		mapper:   NewAssetMapper(copiedMappings),
		mappings: copiedMappings,
	}
}

func (s *TradingBalanceService) Overview(ctx context.Context, request TradingBalanceOverviewRequest) (TradingBalanceOverview, error) {
	asset := request.Asset
	mapping, ok := s.mappingForBalanceAsset(asset)
	if !ok || mapping.NECOTokenAddress == "" {
		return TradingBalanceOverview{}, ErrTradingBalanceAssetNotConfigured
	}
	asset = mapping.NECOAsset
	ledgerBalance, err := s.ledger.GetBalance(ctx, request.UserID, asset)
	if err != nil {
		return TradingBalanceOverview{}, err
	}
	walletAvailable := "0"
	if s.reader != nil && request.NECOAddress != "" {
		walletAvailable, err = s.reader.BalanceOf(ctx, mapping.NECOTokenAddress, request.NECOAddress)
		if err != nil {
			return TradingBalanceOverview{}, err
		}
	}
	displayAvailable, err := addUnsignedAmounts(ledgerBalance.Available, walletAvailable)
	if err != nil {
		return TradingBalanceOverview{}, err
	}
	return TradingBalanceOverview{
		UserID:              request.UserID,
		NECOAddress:         request.NECOAddress,
		Asset:               asset,
		NECOTokenAddress:    mapping.NECOTokenAddress,
		LedgerAvailable:     ledgerBalance.Available,
		LedgerFrozen:        ledgerBalance.Frozen,
		NECOWalletAvailable: walletAvailable,
		DisplayAvailable:    displayAvailable,
	}, nil
}

func (s *TradingBalanceService) mappingForBalanceAsset(asset string) (AssetMapping, bool) {
	if asset != "" {
		return s.mapper.ByNECOAsset(asset)
	}
	for _, mapping := range s.mappings {
		if mapping.EnabledForTrading && mapping.NECOAsset != "" && mapping.NECOTokenAddress != "" {
			return mapping, true
		}
	}
	for _, mapping := range s.mappings {
		if mapping.NECOAsset != "" && mapping.NECOTokenAddress != "" {
			return mapping, true
		}
	}
	return AssetMapping{}, false
}

func addUnsignedAmounts(leftValue, rightValue string) (string, error) {
	left, ok := parseLedgerAmount(leftValue)
	if !ok {
		return "", errors.New("invalid left amount")
	}
	right, ok := parseLedgerAmount(rightValue)
	if !ok {
		return "", errors.New("invalid right amount")
	}
	return new(big.Int).Add(left, right).String(), nil
}

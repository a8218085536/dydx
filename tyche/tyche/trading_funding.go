package tyche

import (
	"context"
	"errors"
)

var (
	ErrTradingFundingNotConfigured      = errors.New("trading funding is not configured")
	ErrTradingFundingWalletInsufficient = errors.New("trading funding wallet balance insufficient")
)

type NECOWalletFundingConfig struct {
	DepositAddress string         `json:"depositAddress" toml:"depositAddress"`
	AssetMappings  []AssetMapping `json:"assetMappings" toml:"assetMappings"`
}

type NECOWalletFundingPreparer struct {
	reader NECOTokenBalanceReader
	conf   NECOWalletFundingConfig
	mapper *AssetMapper
}

func NewNECOWalletFundingPreparer(reader NECOTokenBalanceReader, conf NECOWalletFundingConfig) *NECOWalletFundingPreparer {
	return &NECOWalletFundingPreparer{
		reader: reader,
		conf:   conf,
		mapper: NewAssetMapper(conf.AssetMappings),
	}
}

func (p *NECOWalletFundingPreparer) PrepareFunding(ctx context.Context, request TradingFundingPrepareRequest) (TradingFundingPrepareResult, error) {
	if p == nil || p.conf.DepositAddress == "" || request.NECOAddress == "" {
		return TradingFundingPrepareResult{}, ErrTradingFundingNotConfigured
	}
	mapping, ok := p.mapper.ByNECOAsset(request.ChargeAsset)
	if !ok || mapping.NECOTokenAddress == "" {
		return TradingFundingPrepareResult{}, ErrTradingBalanceAssetNotConfigured
	}
	missing := defaultString(request.MissingAmount, request.ChargeAmount)
	if _, ok := parseLedgerAmount(missing); !ok {
		return TradingFundingPrepareResult{}, ErrTradingQuoteInvalidAmount
	}
	walletBalance := ""
	if p.reader != nil {
		balance, err := p.reader.BalanceOf(ctx, mapping.NECOTokenAddress, request.NECOAddress)
		if err != nil {
			return TradingFundingPrepareResult{}, err
		}
		walletBalance = balance
		if enough, err := unsignedAmountAtLeast(balance, missing); err != nil {
			return TradingFundingPrepareResult{}, err
		} else if !enough {
			return TradingFundingPrepareResult{}, ErrTradingFundingWalletInsufficient
		}
	}
	return TradingFundingPrepareResult{
		Status:          TradingFundingStatusWalletActionRequired,
		Action:          TradingFundingActionTransfer,
		Asset:           request.ChargeAsset,
		Amount:          missing,
		TokenAddress:    mapping.NECOTokenAddress,
		FromAddress:     request.NECOAddress,
		ToAddress:       p.conf.DepositAddress,
		Reason:          "transfer DEDX before submitting dydx order",
		WalletBalance:   walletBalance,
		LedgerAvailable: request.LedgerAvailable,
	}, nil
}

func unsignedAmountAtLeast(balanceValue, requiredValue string) (bool, error) {
	balance, ok := parseLedgerAmount(balanceValue)
	if !ok {
		return false, errors.New("invalid wallet balance")
	}
	required, ok := parseLedgerAmount(requiredValue)
	if !ok {
		return false, errors.New("invalid required amount")
	}
	return balance.Cmp(required) >= 0, nil
}

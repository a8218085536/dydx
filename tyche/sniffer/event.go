package sniffer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Event struct {
	Address      common.Address         `json:"address"`
	ContractName string                 `json:"contractName"`
	ChainID      *big.Int               `json:"chainId"`
	Data         map[string]interface{} `json:"data"`
	BlockHash    common.Hash            `json:"blockHash"`
	BlockNumber  string                 `json:"blockNumber"`
	Name         string                 `json:"name"`
	TxIndex      string                 `json:"txIndex"`
	BlockNum     int64                  `json:"blockNum" validate:"required"` // 区块高度
	TxHash       string                 `json:"txHash" validate:"required"`   // 交易hash
	From         string                 `json:"from" validate:"required"`     // 发送地址
	To           string                 `json:"to" validate:"required"`       // 接收地址
	Value        float64                `json:"value" validate:"required"`    // 交易数量
	Decimals     int                    `json:"decimals" validate:"required"` // 精度
}

type EventHandler func(*Event) error

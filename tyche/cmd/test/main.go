package main

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// func main() {
// 	// 连接以太坊节点
// 	client, err := ethclient.Dial("https://polygon-mumbai.infura.io/v3/ee1d61ff21434b3a881fe98ff30c5587")
// 	if err != nil {
// 		panic(err)
// 	}

// 	// 合约地址（以太坊主网上的 Uniswap 合约）
// 	contractAddress := "0x42172a0a87857b77B08c80F182bE5118E273d753"

// 	// 创建一个查询过滤器
// 	query := ethereum.FilterQuery{
// 		Addresses: []common.Address{common.HexToAddress(contractAddress)},
// 	}

// 	// 获取所有日志
// 	logs, err := client.FilterLogs(context.Background(), query)
// 	if err != nil {
// 		panic(err)
// 	}

// 	// 统计日志数量即为该合约的总交易数
// 	fmt.Printf("Total transactions of contract %v: %s\n", contractAddress, fmt.Sprint((len(logs))))
// }

// func main() {
// 	// 创建一个以太坊连接客户端实例
// 	client, err := ethclient.Dial("https://rpc.ankr.com/eth")
// 	if err != nil {
// 		log.Fatalf("Failed to connect to the Ethereum network: %v", err)
// 	}

// 	// 获取以太坊区块数量
// 	blockNumber, err := client.BlockNumber(context.Background())
// 	if err != nil {
// 		log.Fatalf("Failed to get block number: %v", err)
// 	}

// 	// 获取最新区块并计算ETH总额
// 	latestBlock, err := client.BlockByNumber(context.Background(), big.NewInt(int64(blockNumber)))
// 	if err != nil {
// 		log.Fatalf("Failed to get latest block: %v", err)
// 	}

// 	totalETH := big.NewInt(0)
// 	for _, tx := range latestBlock.Transactions() {
// 		value := tx.Value()
// 		totalETH.Add(totalETH, value)
// 	}

// 	fmt.Printf("Total ETH on the Ethereum network: %v", totalETH)
// }

func main() {
	client, err := ethclient.Dial("https://rpc.ankr.com/eth")
	if err != nil {
		fmt.Println("Failed to connect to the Ethereum client:", err)
		return
	}

	// Replace with the address you want to query
	addr := common.HexToAddress("0xE92d1A43df510F82C66382592a047d288f85226f")

	balance, err := client.BalanceAt(context.Background(), addr, nil)
	if err != nil {
		fmt.Println("Failed to retrieve account balance:", err)
		return
	}

	// Get the total supply of ETH
	totalSupply, err := client.PendingBalanceAt(context.Background(), common.HexToAddress("0x0"))
	if err != nil {
		fmt.Println("Failed to retrieve total ETH supply:", err)
		return
	}

	fraction := new(big.Float).Quo(new(big.Float).SetInt(balance), new(big.Float).SetInt(totalSupply))
	percentage := new(big.Float).Mul(fraction, big.NewFloat(100))

	fmt.Printf("Balance: %v ETH (%v%%)\n", balance, percentage)
}

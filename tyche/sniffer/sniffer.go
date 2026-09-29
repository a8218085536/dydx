package sniffer

import (
	"context"
	"ethgo/eth"
	"ethgo/model/blocknumber"
	"ethgo/util/ethx"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type Sniffer struct {
	chainID      *big.Int
	conf         *Config
	contracts    map[common.Address]*ethx.Contract
	handler      EventHandler
	addresses    []common.Address
	filterTopics []common.Hash
}

func defaultEventHandler(*Event) error {
	panic("请注册 EventHandler")
}

func New(conf *Config) (*Sniffer, error) {

	sf := &Sniffer{
		conf:         conf,
		handler:      defaultEventHandler,
		contracts:    make(map[common.Address]*ethx.Contract),
		addresses:    make([]common.Address, 0),
		filterTopics: make([]common.Hash, 0),
	}

	for _, v := range conf.Contracts {
		var address = common.HexToAddress(v.Addr)
		contract, err := ethx.NewContract(address, v.ABI)
		if err != nil {
			return nil, err
		}

		var eventIDs = make(map[string]common.Hash)
		for k, v := range contract.Events {
			eventIDs[strings.ToLower(k)] = v.ID
		}

		var filterTopics = make([]common.Hash, 0)
		for _, eventName := range v.Events {
			id, ok := eventIDs[strings.ToLower(eventName)]
			if !ok {
				return nil, ErrNoEvent
			}
			filterTopics = append(filterTopics, id)
		}

		sf.addresses = append(sf.addresses, address)
		sf.filterTopics = append(sf.filterTopics, filterTopics...)
		sf.contracts[address] = contract
	}

	return sf, nil
}

func (s *Sniffer) SetEventHandler(handler EventHandler) {
	if handler == nil {
		handler = defaultEventHandler
	}
	s.handler = handler
}

func (s *Sniffer) Run(ctx context.Context, backend eth.Backend) error {
	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return err
	}

	latest, err := backend.BlockNumber(ctx)
	if err != nil {
		return err
	}

	if err := blocknumber.SetNX(latest); err != nil {
		return err
	}

	s.chainID = chainID
	s.run(ctx, backend)
	return nil
}

func (s *Sniffer) run(ctx context.Context, backend eth.Backend) {
	log.Info("开始侦听")
	defer log.Info("结束侦听")

	for {

		goto QUERY

	WAIT:
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}

	QUERY:
		// Beginning of the queried range.
		// 获取本地最新块
		fromBlockNumber, err := blocknumber.Get()
		if err != nil {
			log.With(err).Error("Failed to blocknumber.Get")
			goto WAIT
		}

		// End of the range.
		//获取当前安全块
		toBlockNumber, err := s.getSecurityBlockNumber(ctx, backend)
		if err != nil {
			log.With(err).Error("Failed to getSecurityBlockNumber")
			goto WAIT
		}

		if fromBlockNumber > toBlockNumber {
			// Is latest block number.
			// 如果本地块号大于安全块号，则表示本地已经是最新块，进入等待状态。
			goto WAIT
		}

		// Clipping block number range.
		// 如果查询的块数超过了配置文件中指定的块数，则将结束块号调整并限制查询块数。
		blockCnt := toBlockNumber - fromBlockNumber + 1
		if blockCnt > s.conf.NumberOfBlocks {
			toBlockNumber = fromBlockNumber + s.conf.NumberOfBlocks - 1
		}

		log.Debugf("起始块: %d, 结束块: %d", fromBlockNumber, toBlockNumber)

		// Executes a filter query.
		// 执行日志筛选操作，从区块中抽取感兴趣的日志信息。
		logs, err := s.filterLogs(ctx, backend, fromBlockNumber, toBlockNumber)
		if err != nil {
			log.With(err).Error("Failed to filterLogs")
			goto WAIT
		}

		// Handle all logs.
		// 处理抽取到的日志信息，并在处理过程中出现错误则进入等待状态。
		if err := s.handleLogs(ctx, logs); err != nil {
			log.With(err).Error("Failed to handleLogs")
			goto WAIT
		}

		// Update local block number in redis.
		// 将本地块号更新为安全块号的下一个块号。
		blocknumber.Set(toBlockNumber + 1)
		goto QUERY
	}
}

func (s *Sniffer) getSecurityBlockNumber(ctx context.Context, backend eth.Backend) (uint64, error) {
	latestBlockNumber, err := backend.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}

	securityHeight := s.conf.SecrityHeight
	if latestBlockNumber < securityHeight {
		return 0, fmt.Errorf("no blocks")
	}

	return latestBlockNumber - securityHeight, nil
}

func (s *Sniffer) filterLogs(ctx context.Context, backend eth.Backend, fromBlockNumber uint64, toBlockNumber uint64) ([]types.Log, error) {

	filterQuery := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlockNumber),
		ToBlock:   new(big.Int).SetUint64(toBlockNumber),
		Addresses: s.addresses,
		Topics:    [][]common.Hash{s.filterTopics},
	}

	return backend.FilterLogs(ctx, filterQuery)
}

func (s *Sniffer) handleLogs(ctx context.Context, logs []types.Log) error {

	// 处理所有的日志消息
	for _, v := range logs {
		event := new(Event)
		// 反序列化日志消息
		if err := s.unpackLog(v, event); err != nil {
			log.Panic(err)
		}

		// 处理反序列化后的事件
		if err := s.handleEvent(ctx, event); err != nil {
			return err
		}
		// 在应用程序关闭时，可以取消所有正在进行的处理任务
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

func (s *Sniffer) unpackLog(l types.Log, out *Event) error {

	contract := s.contracts[l.Address]
	event, err := contract.EventByID(l.Topics[0])
	if err != nil {
		return err
	}

	out.ContractName = contract.Name
	out.ChainID = s.chainID
	out.Name = event.Name
	out.Data = make(map[string]interface{})

	err = contract.UnpackLogIntoMap(out.Data, out.Name, l)
	if err != nil {
		return err
	}

	out.Address = l.Address
	out.BlockHash = l.BlockHash
	out.TxHash = l.TxHash.Hex()
	out.BlockNumber = strconv.FormatUint(l.BlockNumber, 10)
	out.TxIndex = strconv.FormatUint(uint64(l.TxIndex), 10)

	return nil
}

func (s *Sniffer) handleEvent(ctx context.Context, event *Event) error {

	for {
		err := s.handler(event)
		if err == nil {
			return nil
		}

		log.Warn(err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

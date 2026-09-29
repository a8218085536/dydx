package tyche

import (
	"context"
	"crypto/ecdsa"
	"ethgo/eth"
	"ethgo/model/orders"
	"ethgo/tyche/gaslimit"
	"ethgo/tyche/gasprice"
	"ethgo/util/ethx"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/garyburd/redigo/redis"
)

type Tyche struct {
	account               common.Address
	backend               eth.Backend
	contracts             map[common.Address]*ethx.Contract
	chainID               *big.Int
	conf                  *Config
	signer                *eth.Signer
	key                   *ecdsa.PrivateKey
	limiter               *NonceLimiter
	suggestNonceKeepalive time.Duration
}

func New(backend eth.Backend, conf *Config) *Tyche {
	return &Tyche{
		backend:               backend,
		conf:                  conf,
		suggestNonceKeepalive: 0,
	}
}

func (t *Tyche) Init(ctx context.Context) error {
	var c = t.conf
	var account = common.HexToAddress(c.Account)
	var backend = t.backend
	var contracts = map[common.Address]*ethx.Contract{}
	for _, v := range c.Contracts {
		contract, err := ethx.NewContract(common.HexToAddress(v.Addr), v.ABI)
		if err != nil {
			return err
		}
		contracts[contract.Address] = contract
	}

	key, err := crypto.HexToECDSA(c.PrivateKey)
	if err != nil {
		return err
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return err
	}
	signer, err := eth.NewSigner(key, chainID)
	if err != nil {
		return err
	}

	latestNonceAt, err := t.backend.NonceAt(ctx, account, nil)
	if err != nil {
		return err
	}
	limiter := NewNonceLimiter(latestNonceAt)

	pendingNonceAt, err := t.backend.PendingNonceAt(ctx, account)
	if err != nil {
		return err
	}

	localNonceAt, err := orders.NonceAt()
	switch err {
	case nil:
		if localNonceAt < pendingNonceAt {
			log.Panicf("The local nonce is less than pending nonce: %v, %v", localNonceAt, pendingNonceAt)
		}
	case redis.ErrNil:
		if latestNonceAt != pendingNonceAt {
			log.Panicf("The latest nonce is not equal to pending nonce:, %v, %v", latestNonceAt, pendingNonceAt)
		}

		if err := orders.Init(pendingNonceAt); err != nil {
			return err
		}
	default:
		return err
	}

	estimator, err := gasprice.NewJSEstimator(c.EstimatorJS, chainID)
	if err != nil {
		return err
	}

	if err := gasprice.Init(ctx, backend, estimator, c.GasPriceUpdateInterval); err != nil {
		return err
	}

	if err := gaslimit.Init(ctx, backend); err != nil {
		return err
	}

	t.account = account
	t.chainID = chainID
	t.contracts = contracts
	t.limiter = limiter
	t.key = key
	t.signer = signer
	t.suggestNonceKeepalive = 0
	return nil
}

func (t *Tyche) SetAccount(Account string, KEY string) error {
	ctx, _ := context.WithCancel(context.Background())
	var c = t.conf
	var account = common.HexToAddress(Account)
	var backend = t.backend
	var contracts = map[common.Address]*ethx.Contract{}
	for _, v := range c.Contracts {
		contract, err := ethx.NewContract(common.HexToAddress(v.Addr), v.ABI)
		if err != nil {
			return err
		}
		contracts[contract.Address] = contract
	}

	key, err := crypto.HexToECDSA(KEY)
	if err != nil {
		return err
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return err
	}
	signer, err := eth.NewSigner(key, chainID)
	if err != nil {
		return err
	}

	latestNonceAt, err := t.backend.NonceAt(ctx, account, nil)
	if err != nil {
		return err
	}
	limiter := NewNonceLimiter(latestNonceAt)

	pendingNonceAt, err := t.backend.PendingNonceAt(ctx, account)
	if err != nil {
		return err
	}

	if err := orders.Init(pendingNonceAt); err != nil {
		return err
	}

	estimator, err := gasprice.NewJSEstimator(c.EstimatorJS, chainID)
	if err != nil {
		return err
	}

	if err := gasprice.Init(ctx, backend, estimator, c.GasPriceUpdateInterval); err != nil {
		return err
	}

	if err := gaslimit.Init(ctx, backend); err != nil {
		return err
	}
	t.account = account
	t.chainID = chainID
	t.contracts = contracts
	t.limiter = limiter
	t.key = key
	t.signer = signer
	t.suggestNonceKeepalive = 0
	return nil
}

func (t *Tyche) Run(ctx context.Context) error {
	return t.run(ctx)
}

func (t *Tyche) run(ctx context.Context) error {
	log.Info("开始侦听")
	defer log.Info("结束侦听")

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.watchPending(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.watchSent(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.watchSucceed(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.watchFailed(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.watchError(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.watchNonce(ctx)
	}()

	wg.Wait()
	return nil
}

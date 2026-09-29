package app

import (
	"bytes"
	"context"
	"ethgo/tyche"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestNECOProcessWithdrawalAPI(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	withdrawal := createFrozenWithdrawalForNECOAPITest(t, ledger)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	worker, err := tyche.NewNECOWithdrawalWorker(ledger, tyche.NewMemoryNECOWithdrawalStore(), &appFakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}, tyche.NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	app := &App{necoWithdrawal: worker}

	rec := postJSON(t, app, "/tyche/api/neco/withdrawal/process", map[string]interface{}{
		"withdrawalId": withdrawal.ID,
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.NECOWithdrawalStatusBroadcasted)) {
		t.Fatalf("body=%s, want broadcasted", rec.Body.String())
	}
}

func TestNECOSyncWithdrawalReceiptAPI(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	withdrawal := createFrozenWithdrawalForNECOAPITest(t, ledger)
	settlements := tyche.NewMemoryTradingSettlementStore()
	settlement, _, err := settlements.CreateIfAbsent(context.Background(), tyche.TradingSettlement{
		UserID:             withdrawal.UserID,
		CloseOrderID:       "close-order-1",
		LedgerWithdrawalID: withdrawal.ID,
		SettlementAsset:    withdrawal.Asset,
		SettlementAmount:   withdrawal.Amount,
		Status:             tyche.TradingSettlementStatusNECOPayoutBroadcasted,
	})
	if err != nil {
		t.Fatalf("CreateIfAbsent() error = %v", err)
	}
	store := tyche.NewMemoryNECOWithdrawalStore()
	if err := store.Save(context.Background(), tyche.NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		TxHash:       "0xtx1",
		Status:       tyche.NECOWithdrawalStatusBroadcasted,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	worker, err := tyche.NewNECOWithdrawalWorker(ledger, store, &appFakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
		receipt:  &types.Receipt{Status: types.ReceiptStatusSuccessful},
	}, tyche.NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	app := &App{necoWithdrawal: worker, tradingSettlements: settlements}

	rec := postJSON(t, app, "/tyche/api/neco/withdrawal/sync-receipt", map[string]interface{}{
		"withdrawalId": withdrawal.ID,
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.NECOWithdrawalStatusPaid)) {
		t.Fatalf("body=%s, want paid", rec.Body.String())
	}
	updated, ok, err := settlements.GetByID(context.Background(), settlement.ID)
	if err != nil || !ok {
		t.Fatalf("GetByID() settlement=%+v ok=%v err=%v", updated, ok, err)
	}
	if updated.Status != tyche.TradingSettlementStatusNECOPayoutPaid {
		t.Fatalf("settlement status = %q, want paid", updated.Status)
	}
}

func TestNECOSyncWithdrawalReceiptProcessesFrozenWithdrawalWhenRecordMissing(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	withdrawal := createFrozenWithdrawalForNECOAPITest(t, ledger)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	backend := &appFakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
	}
	worker, err := tyche.NewNECOWithdrawalWorker(ledger, tyche.NewMemoryNECOWithdrawalStore(), backend, tyche.NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	app := &App{necoWithdrawal: worker}

	rec := postJSON(t, app, "/tyche/api/neco/withdrawal/sync-receipt", map[string]interface{}{
		"withdrawalId": withdrawal.ID,
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.NECOWithdrawalStatusBroadcasted)) {
		t.Fatalf("body=%s, want broadcasted", rec.Body.String())
	}
	if backend.sent == nil {
		t.Fatalf("expected missing NECO withdrawal record to be processed")
	}
}

func TestAppStartupNECOWithdrawalRepairProcessesFrozenWithdrawals(t *testing.T) {
	ledger := tyche.NewLedgerService(tyche.NewMemoryLedgerStore())
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "100",
		RefID:  "deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := ledger.FreezeWithdrawal(context.Background(), tyche.LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "DEDX",
		Amount:    "40",
		ToAddress: "0x0000000000000000000000000000000000000aaa",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	hotWallet := crypto.PubkeyToAddress(key.PublicKey)
	store := tyche.NewMemoryNECOWithdrawalStore()
	if err := store.Save(context.Background(), tyche.NECOWithdrawalRecord{
		WithdrawalID: withdrawal.ID,
		TxHash:       "0xtx1",
		Status:       tyche.NECOWithdrawalStatusBroadcasted,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	backend := &appFakeNECOWithdrawalBackend{
		chainID:  big.NewInt(9001),
		gasPrice: big.NewInt(2),
		nonce:    7,
		receipt:  &types.Receipt{Status: types.ReceiptStatusSuccessful},
	}
	worker, err := tyche.NewNECOWithdrawalWorker(ledger, store, backend, tyche.NECOWalletConfig{
		HotWalletAddress: hotWallet.Hex(),
		HotWalletKey:     crypto.FromECDSA(key),
		TokenContracts: map[string]string{
			"DEDX": "0x0000000000000000000000000000000000000ded",
		},
		GasLimit: 60000,
	})
	if err != nil {
		t.Fatalf("NewNECOWithdrawalWorker() error = %v", err)
	}
	app := &App{necoWithdrawal: worker}

	app.runNECOWithdrawalStartupRepair(context.Background())

	balance, err := ledger.GetBalance(context.Background(), "user-1", "DEDX")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Available != "60" || balance.Frozen != "0" {
		t.Fatalf("balance = %+v, want startup repair to clear frozen withdrawal with paid receipt", balance)
	}
}

func TestNECOSyncDepositReceiptAPI(t *testing.T) {
	authStore := tyche.NewMemoryAuthStore()
	user, err := authStore.GetOrCreateUser(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}
	ledgerStore := tyche.NewMemoryLedgerStore()
	depositService := tyche.NewNECODepositService(tyche.NewMemoryNECODepositStore(), tyche.NewLedgerService(ledgerStore), authStore, tyche.NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []tyche.NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	})
	transferLog := appNecoERC20TransferLog(
		"0x0000000000000000000000000000000000000ded",
		"0x0000000000000000000000000000000000000001",
		"0x0000000000000000000000000000000000000abc",
		big.NewInt(100),
		100,
		3,
		"0xtx1",
	)
	app := &App{
		conf: &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		necoDepositReceiptSync: tyche.NewNECODepositReceiptSyncer(&appFakeNECODepositBackend{
			chainID:     big.NewInt(9001),
			blockNumber: 120,
			receipt: &types.Receipt{
				Status:      types.ReceiptStatusSuccessful,
				BlockNumber: big.NewInt(100),
				Logs:        []*types.Log{&transferLog},
			},
		}, depositService, tyche.NECODepositWatcherConfig{
			DepositAddress:   "0x0000000000000000000000000000000000000abc",
			TokenAddresses:   []string{"0x0000000000000000000000000000000000000ded"},
			MinConfirmations: 12,
		}),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	rec := postJSONWithAPIKey(t, app, "/tyche/api/neco/deposit/sync-receipt", "secret", map[string]interface{}{
		"txHash": "0xtx1",
	})

	assertResponseCode(t, rec, http.StatusOK)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.NECODepositStatusCredited)) {
		t.Fatalf("body=%s, want credited deposit", rec.Body.String())
	}
	balance, _ := ledgerStore.GetBalance(context.Background(), user.UserID, "DEDX")
	if balance.Available != "100" {
		t.Fatalf("available = %q, want 100", balance.Available)
	}
}

func TestNECOPriceQuoteAPIQuotesDEDXToUSDT(t *testing.T) {
	app := &App{
		conf: &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		necoPriceQuotes: tyche.NewNECOPriceQuoteService(appStaticAssetUSDOracle{
			snapshots: map[string]tyche.AssetPriceSnapshot{
				"DEDX": {
					ID:           "dedx-price-1",
					Asset:        "DEDX",
					USDAsset:     "USD",
					USDPrice:     "4",
					RouteName:    "DEDX-NECO-USD",
					Source:       "v2_pair:DEDX-NECO-USD",
					BlockNumber:  123,
					MinLiquidity: "1000",
					Pairs:        []string{"dedx-neco-pair", "neco-usdt-pair"},
				},
				"USDT": {
					ID:        "usdt-price-1",
					Asset:     "USDT",
					USDAsset:  "USD",
					USDPrice:  "1",
					RouteName: "USDT-USD",
					Source:    "stable_usd:USDT-USD",
				},
			},
		}),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	rec := postJSONWithAPIKey(t, app, "/tyche/api/neco/price/quote", "secret", map[string]interface{}{
		"fromAsset": "DEDX",
		"toAsset":   "USDT",
		"amount":    "2.5",
	})

	assertResponseCode(t, rec, http.StatusOK)
	body := rec.Body.String()
	for _, want := range []string{
		`"fromAsset":"DEDX"`,
		`"toAsset":"USDT"`,
		`"fromAmount":"2.5"`,
		`"toAmount":"10"`,
		`"price":"4"`,
		`"priceRoute":"DEDX-NECO-USD"`,
		`"priceSnapshotId":"dedx-price-1"`,
	} {
		if !bytes.Contains([]byte(body), []byte(want)) {
			t.Fatalf("body=%s, want %s", body, want)
		}
	}
}

func TestNECOPriceQuoteAPIRejectsMissingInternalKey(t *testing.T) {
	app := &App{
		conf:            &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		necoPriceQuotes: tyche.NewNECOPriceQuoteService(appStaticAssetUSDOracle{}),
	}
	app.conf.Tyche.InternalAPIKey = "secret"

	rec := postJSONWithAPIKey(t, app, "/tyche/api/neco/price/quote", "", map[string]interface{}{
		"fromAsset": "DEDX",
		"toAsset":   "USDT",
	})

	assertResponseCode(t, rec, http.StatusUnauthorized)
}

func TestNECOPriceQuoteAPIDefaultsAmountToOne(t *testing.T) {
	app := newAppWithStaticNECOPriceQuotes(t)

	rec := postJSONWithAPIKey(t, app, "/tyche/api/neco/price/quote", "secret", map[string]interface{}{
		"fromAsset": "DEDX",
		"toAsset":   "USDT",
	})

	assertResponseCode(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"fromAmount":"1"`)) || !bytes.Contains([]byte(body), []byte(`"toAmount":"4"`)) {
		t.Fatalf("body=%s, want default amount quote", body)
	}
}

func TestNECOPriceQuoteAPIRejectsInvalidAmount(t *testing.T) {
	app := newAppWithStaticNECOPriceQuotes(t)

	rec := postJSONWithAPIKey(t, app, "/tyche/api/neco/price/quote", "secret", map[string]interface{}{
		"fromAsset": "DEDX",
		"toAsset":   "USDT",
		"amount":    "0",
	})

	assertResponseCode(t, rec, http.StatusBadRequest)
	if !bytes.Contains(rec.Body.Bytes(), []byte(tyche.ErrTradingQuoteInvalidAmount.Error())) {
		t.Fatalf("body=%s, want invalid amount error", rec.Body.String())
	}
}

func TestAppNewNECODepositServiceUsesConfiguredTokens(t *testing.T) {
	app := &App{
		conf:   &Config{Tyche: appTestConfig(tyche.ChainTypeEVM)},
		ledger: tyche.NewLedgerService(tyche.NewMemoryLedgerStore()),
	}
	app.conf.Tyche.NECODeposit = tyche.NECODepositConfig{
		DepositAddress:   "0x0000000000000000000000000000000000000abc",
		MinConfirmations: 12,
		Tokens: []tyche.NECOTokenConfig{
			{Address: "0x0000000000000000000000000000000000000ded", Asset: "DEDX"},
		},
	}

	service := app.newNECODepositService()

	if service == nil {
		t.Fatal("newNECODepositService() = nil")
	}
}

func createFrozenWithdrawalForNECOAPITest(t *testing.T, ledger *tyche.LedgerService) tyche.LedgerWithdrawal {
	t.Helper()
	if _, err := ledger.CreditMockDeposit(context.Background(), tyche.LedgerDepositRequest{
		UserID: "user-1",
		Asset:  "DEDX",
		Amount: "100",
		RefID:  "deposit-1",
	}); err != nil {
		t.Fatalf("CreditMockDeposit() error = %v", err)
	}
	withdrawal, err := ledger.FreezeWithdrawal(context.Background(), tyche.LedgerWithdrawalRequest{
		UserID:    "user-1",
		Asset:     "DEDX",
		Amount:    "40",
		ToAddress: "0x0000000000000000000000000000000000000aaa",
	})
	if err != nil {
		t.Fatalf("FreezeWithdrawal() error = %v", err)
	}
	return withdrawal
}

func newAppWithStaticNECOPriceQuotes(t *testing.T) *App {
	t.Helper()
	app := &App{
		conf: &Config{Tyche: appTestConfig(tyche.ChainTypeDYDX)},
		necoPriceQuotes: tyche.NewNECOPriceQuoteService(appStaticAssetUSDOracle{
			snapshots: map[string]tyche.AssetPriceSnapshot{
				"DEDX": {
					ID:           "dedx-price-1",
					Asset:        "DEDX",
					USDAsset:     "USD",
					USDPrice:     "4",
					RouteName:    "DEDX-NECO-USD",
					Source:       "v2_pair:DEDX-NECO-USD",
					BlockNumber:  123,
					MinLiquidity: "1000",
					Pairs:        []string{"dedx-neco-pair", "neco-usdt-pair"},
				},
				"USDT": {
					ID:        "usdt-price-1",
					Asset:     "USDT",
					USDAsset:  "USD",
					USDPrice:  "1",
					RouteName: "USDT-USD",
					Source:    "stable_usd:USDT-USD",
				},
			},
		}),
	}
	app.conf.Tyche.InternalAPIKey = "secret"
	return app
}

type appFakeNECOWithdrawalBackend struct {
	chainID  *big.Int
	gasPrice *big.Int
	nonce    uint64
	sent     *types.Transaction
	receipt  *types.Receipt
}

func (b *appFakeNECOWithdrawalBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return b.chainID, nil
}

func (b *appFakeNECOWithdrawalBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return b.nonce, nil
}

func (b *appFakeNECOWithdrawalBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return b.gasPrice, nil
}

func (b *appFakeNECOWithdrawalBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	b.sent = tx
	return nil
}

func (b *appFakeNECOWithdrawalBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return b.receipt, nil
}

type appFakeNECODepositBackend struct {
	chainID     *big.Int
	blockNumber uint64
	receipt     *types.Receipt
}

func (b *appFakeNECODepositBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return b.chainID, nil
}

func (b *appFakeNECODepositBackend) BlockNumber(ctx context.Context) (uint64, error) {
	return b.blockNumber, nil
}

func (b *appFakeNECODepositBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return b.receipt, nil
}

func appNecoERC20TransferLog(token, from, to string, amount *big.Int, blockNumber uint64, logIndex uint, txHash string) types.Log {
	return types.Log{
		Address:     common.HexToAddress(token),
		Topics:      []common.Hash{tyche.ERC20TransferTopic(), common.HexToHash(from), common.HexToHash(to)},
		Data:        common.LeftPadBytes(amount.Bytes(), 32),
		BlockNumber: blockNumber,
		Index:       logIndex,
		TxHash:      common.HexToHash(txHash),
	}
}

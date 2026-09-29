package tyche

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	NECODepositStatusPending  = "pending"
	NECODepositStatusCredited = "credited"
)

var (
	ErrNECODepositNotConfirmed      = errors.New("neco deposit not confirmed")
	ErrNECODepositUnsupportedToken  = errors.New("neco deposit unsupported token")
	ErrNECODepositWrongDestination  = errors.New("neco deposit wrong destination")
	ErrNECODepositDuplicateEvent    = errors.New("neco deposit duplicate event")
	ErrNECODepositNotFound          = errors.New("neco deposit not found")
	ErrNECODepositUserLookupMissing = errors.New("neco deposit user lookup missing")
	ErrNECODepositTxHashRequired    = errors.New("neco deposit tx hash is required")
	ErrNECODepositReceiptNotFound   = errors.New("neco deposit receipt not found")
	ErrNECODepositReceiptFailed     = errors.New("neco deposit transaction failed")
	ErrNECODepositNoMatchingLog     = errors.New("neco deposit receipt has no matching transfer to deposit address")
)

type NECOTokenConfig struct {
	Address string `json:"address" toml:"address"`
	Asset   string `json:"asset" toml:"asset"`
}

type NECODepositConfig struct {
	DepositAddress   string            `json:"depositAddress" toml:"depositAddress"`
	MinConfirmations uint64            `json:"minConfirmations" toml:"minConfirmations"`
	StartBlock       uint64            `json:"startBlock" toml:"startBlock"`
	MaxBlockRange    uint64            `json:"maxBlockRange" toml:"maxBlockRange"`
	Tokens           []NECOTokenConfig `json:"tokens" toml:"tokens"`
	AssetMappings    []AssetMapping    `json:"assetMappings" toml:"assetMappings"`
}

type NECOTokenTransfer struct {
	ChainID       string `json:"chainId"`
	TokenAddress  string `json:"tokenAddress"`
	From          string `json:"from"`
	To            string `json:"to"`
	Amount        string `json:"amount"`
	TxHash        string `json:"txHash"`
	LogIndex      uint   `json:"logIndex"`
	Confirmations uint64 `json:"confirmations"`
}

type NECODeposit struct {
	ID           string `json:"id" redis:"id"`
	EventID      string `json:"eventId" redis:"eventId"`
	ChainID      string `json:"chainId" redis:"chainId"`
	TokenAddress string `json:"tokenAddress" redis:"tokenAddress"`
	Asset        string `json:"asset" redis:"asset"`
	From         string `json:"from" redis:"from"`
	To           string `json:"to" redis:"to"`
	Amount       string `json:"amount" redis:"amount"`
	TxHash       string `json:"txHash" redis:"txHash"`
	LogIndex     uint   `json:"logIndex" redis:"logIndex"`
	UserID       string `json:"userId" redis:"userId"`
	Status       string `json:"status" redis:"status"`
}

type NECODepositStore interface {
	GetByEventID(context.Context, string) (NECODeposit, bool, error)
	Save(context.Context, NECODeposit) error
	PendingByNECOAddress(context.Context, string) ([]NECODeposit, error)
	MarkCredited(context.Context, string, string) (NECODeposit, error)
	ListByUser(context.Context, string) ([]NECODeposit, error)
	ListByStatus(context.Context, string) ([]NECODeposit, error)
}

type AuthUserLookup interface {
	GetUserByNECOAddress(context.Context, string) (AuthUser, bool, error)
}

type NECODepositService struct {
	store  NECODepositStore
	ledger *LedgerService
	users  AuthUserLookup
	conf   NECODepositConfig
	tokens map[string]string
}

func NewNECODepositService(store NECODepositStore, ledger *LedgerService, users AuthUserLookup, conf NECODepositConfig) *NECODepositService {
	tokens := map[string]string{}
	for _, token := range conf.Tokens {
		tokens[normalizeAddress(token.Address)] = token.Asset
	}
	for _, mapping := range conf.AssetMappings {
		if !mapping.EnabledForTrading {
			continue
		}
		tokens[normalizeAddress(mapping.NECOTokenAddress)] = mapping.NECOAsset
	}
	return &NECODepositService{store: store, ledger: ledger, users: users, conf: conf, tokens: tokens}
}

func (s *NECODepositService) ProcessTransfer(ctx context.Context, transfer NECOTokenTransfer) (NECODeposit, error) {
	eventID := NECODepositEventID(transfer)
	if existing, ok, err := s.store.GetByEventID(ctx, eventID); err != nil {
		return NECODeposit{}, err
	} else if ok {
		return existing, nil
	}
	if transfer.Confirmations < s.conf.MinConfirmations {
		return NECODeposit{}, ErrNECODepositNotConfirmed
	}
	if !strings.EqualFold(transfer.To, s.conf.DepositAddress) {
		return NECODeposit{}, ErrNECODepositWrongDestination
	}
	asset, ok := s.tokens[normalizeAddress(transfer.TokenAddress)]
	if !ok {
		return NECODeposit{}, ErrNECODepositUnsupportedToken
	}

	deposit := NECODeposit{
		ID:           eventID,
		EventID:      eventID,
		ChainID:      transfer.ChainID,
		TokenAddress: transfer.TokenAddress,
		Asset:        asset,
		From:         transfer.From,
		To:           transfer.To,
		Amount:       transfer.Amount,
		TxHash:       transfer.TxHash,
		LogIndex:     transfer.LogIndex,
		Status:       NECODepositStatusPending,
	}
	user, ok, err := s.lookupUser(ctx, transfer.From)
	if err != nil {
		return NECODeposit{}, err
	}
	if ok {
		deposit.UserID = user.UserID
		deposit.Status = NECODepositStatusCredited
	}
	if err := s.store.Save(ctx, deposit); err != nil {
		return NECODeposit{}, err
	}
	if deposit.Status == NECODepositStatusCredited {
		if _, err := s.ledger.CreditMockDeposit(ctx, LedgerDepositRequest{
			UserID: deposit.UserID,
			Asset:  deposit.Asset,
			Amount: deposit.Amount,
			RefID:  deposit.EventID,
		}); err != nil {
			return NECODeposit{}, err
		}
	}
	return deposit, nil
}

func (s *NECODepositService) ClaimPending(ctx context.Context, userID, necoAddress string) ([]NECODeposit, error) {
	pending, err := s.store.PendingByNECOAddress(ctx, necoAddress)
	if err != nil {
		return nil, err
	}
	claimed := make([]NECODeposit, 0, len(pending))
	for _, deposit := range pending {
		if _, err := s.ledger.CreditMockDeposit(ctx, LedgerDepositRequest{
			UserID: userID,
			Asset:  deposit.Asset,
			Amount: deposit.Amount,
			RefID:  deposit.EventID,
		}); err != nil {
			return nil, err
		}
		credited, err := s.store.MarkCredited(ctx, deposit.EventID, userID)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, credited)
	}
	return claimed, nil
}

func (s *NECODepositService) ListDepositsByUser(ctx context.Context, userID string) ([]NECODeposit, error) {
	if s == nil || s.store == nil {
		return nil, ErrNECODepositNotFound
	}
	return s.store.ListByUser(ctx, userID)
}

func (s *NECODepositService) ListDepositsByStatus(ctx context.Context, status string) ([]NECODeposit, error) {
	if s == nil || s.store == nil {
		return nil, ErrNECODepositNotFound
	}
	return s.store.ListByStatus(ctx, status)
}

func (s *NECODepositService) lookupUser(ctx context.Context, necoAddress string) (AuthUser, bool, error) {
	if s.users == nil {
		return AuthUser{}, false, nil
	}
	user, ok, err := s.users.GetUserByNECOAddress(ctx, necoAddress)
	if err != nil {
		return AuthUser{}, false, err
	}
	return user, ok, nil
}

func NECODepositEventID(transfer NECOTokenTransfer) string {
	return fmt.Sprintf("%s:%s:%s:%d", transfer.ChainID, normalizeAddress(transfer.TokenAddress), strings.ToLower(transfer.TxHash), transfer.LogIndex)
}

func normalizeAddress(address string) string {
	return strings.ToLower(address)
}

type MemoryNECODepositStore struct {
	byEvent map[string]NECODeposit
}

func NewMemoryNECODepositStore() *MemoryNECODepositStore {
	return &MemoryNECODepositStore{byEvent: map[string]NECODeposit{}}
}

func (s *MemoryNECODepositStore) GetByEventID(ctx context.Context, eventID string) (NECODeposit, bool, error) {
	deposit, ok := s.byEvent[eventID]
	return deposit, ok, nil
}

func (s *MemoryNECODepositStore) Save(ctx context.Context, deposit NECODeposit) error {
	if _, ok := s.byEvent[deposit.EventID]; ok {
		return ErrNECODepositDuplicateEvent
	}
	s.byEvent[deposit.EventID] = deposit
	return nil
}

func (s *MemoryNECODepositStore) PendingByNECOAddress(ctx context.Context, necoAddress string) ([]NECODeposit, error) {
	var deposits []NECODeposit
	for _, deposit := range s.byEvent {
		if strings.EqualFold(deposit.From, necoAddress) && deposit.Status == NECODepositStatusPending {
			deposits = append(deposits, deposit)
		}
	}
	return deposits, nil
}

func (s *MemoryNECODepositStore) MarkCredited(ctx context.Context, eventID, userID string) (NECODeposit, error) {
	deposit, ok := s.byEvent[eventID]
	if !ok {
		return NECODeposit{}, ErrNECODepositNotFound
	}
	deposit.UserID = userID
	deposit.Status = NECODepositStatusCredited
	s.byEvent[eventID] = deposit
	return deposit, nil
}

func (s *MemoryNECODepositStore) ListByUser(ctx context.Context, userID string) ([]NECODeposit, error) {
	deposits := make([]NECODeposit, 0)
	for _, deposit := range s.byEvent {
		if deposit.UserID == userID {
			deposits = append(deposits, deposit)
		}
	}
	return deposits, nil
}

func (s *MemoryNECODepositStore) ListByStatus(ctx context.Context, status string) ([]NECODeposit, error) {
	deposits := make([]NECODeposit, 0)
	for _, deposit := range s.byEvent {
		if deposit.Status == status {
			deposits = append(deposits, deposit)
		}
	}
	return deposits, nil
}

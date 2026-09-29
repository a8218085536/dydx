//go:build cgo

package dydxtrading

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	appconfig "github.com/dydxprotocol/v4-chain/protocol/app/config"
)

const proxyWalletStoreVersion = 1

var (
	ErrProxyWalletUserIDRequired      = errors.New("dydx proxy wallet userId is required")
	ErrProxyWalletKeystorePathMissing = errors.New("dydx proxy wallet keystore path is required")
	ErrProxyWalletKeystoreSecretWeak  = errors.New("dydx proxy wallet keystore secret must be at least 32 characters")
	ErrProxyWalletAddressRequired     = errors.New("dydx proxy wallet address is required")
)

type EncryptedFileProxyWalletStoreConfig struct {
	Path          string
	Secret        string
	AddressPrefix string
	Now           func() time.Time
}

type EncryptedFileProxyWalletStore struct {
	conf EncryptedFileProxyWalletStoreConfig
	key  [32]byte
	mu   sync.Mutex
}

type proxyWalletFile struct {
	Version      int                          `json:"version"`
	Wallets      map[string]proxyWalletRecord `json:"wallets"`
	AddressIndex map[string]string            `json:"addressIndex"`
}

type proxyWalletRecord struct {
	UserID              string `json:"userId"`
	NECOAddress         string `json:"necoAddress,omitempty"`
	DYDXAddress         string `json:"dydxAddress"`
	EncryptedPrivateKey string `json:"encryptedPrivateKey"`
	CreatedAt           int64  `json:"createdAt"`
}

func NewEncryptedFileProxyWalletStore(conf EncryptedFileProxyWalletStoreConfig) (*EncryptedFileProxyWalletStore, error) {
	if strings.TrimSpace(conf.Path) == "" {
		return nil, ErrProxyWalletKeystorePathMissing
	}
	if len(conf.Secret) < 32 {
		return nil, ErrProxyWalletKeystoreSecretWeak
	}
	if conf.AddressPrefix == "" {
		conf.AddressPrefix = appconfig.Bech32PrefixAccAddr
	}
	if conf.Now == nil {
		conf.Now = time.Now
	}
	return &EncryptedFileProxyWalletStore{
		conf: conf,
		key:  sha256.Sum256([]byte(conf.Secret)),
	}, nil
}

func (s *EncryptedFileProxyWalletStore) CreateOrGetProxyWallet(ctx context.Context, req ProxyWalletCreateRequest) (ProxyWallet, error) {
	if err := ctx.Err(); err != nil {
		return ProxyWallet{}, err
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		return ProxyWallet{}, ErrProxyWalletUserIDRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return ProxyWallet{}, err
	}
	if record, ok := file.Wallets[userID]; ok {
		return record.wallet(), nil
	}

	privateKey := secp256k1.GenPrivKey()
	address, err := AddressFromPrivateKey(privateKey, s.conf.AddressPrefix)
	if err != nil {
		return ProxyWallet{}, err
	}
	encrypted, err := s.encrypt(privateKey.Bytes())
	if err != nil {
		return ProxyWallet{}, err
	}
	record := proxyWalletRecord{
		UserID:              userID,
		NECOAddress:         strings.TrimSpace(req.NECOAddress),
		DYDXAddress:         address,
		EncryptedPrivateKey: encrypted,
		CreatedAt:           s.conf.Now().Unix(),
	}
	file.Wallets[userID] = record
	file.AddressIndex[normalizeAddress(address)] = userID
	if err := s.saveLocked(file); err != nil {
		return ProxyWallet{}, err
	}
	return record.wallet(), nil
}

func (s *EncryptedFileProxyWalletStore) PrivateKeyForAddress(ctx context.Context, address string) (cryptotypes.PrivKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, ErrProxyWalletAddressRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	userID, ok := file.AddressIndex[normalizeAddress(address)]
	if !ok {
		return nil, ErrProxyWalletNotFound
	}
	record, ok := file.Wallets[userID]
	if !ok {
		return nil, ErrProxyWalletNotFound
	}
	keyBytes, err := s.decrypt(record.EncryptedPrivateKey)
	if err != nil {
		return nil, err
	}
	return PrivateKeyFromHex(hex.EncodeToString(keyBytes))
}

func (s *EncryptedFileProxyWalletStore) loadLocked() (proxyWalletFile, error) {
	file := proxyWalletFile{
		Version:      proxyWalletStoreVersion,
		Wallets:      map[string]proxyWalletRecord{},
		AddressIndex: map[string]string{},
	}
	raw, err := os.ReadFile(s.conf.Path)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return proxyWalletFile{}, err
	}
	if len(raw) == 0 {
		return file, nil
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return proxyWalletFile{}, err
	}
	if file.Wallets == nil {
		file.Wallets = map[string]proxyWalletRecord{}
	}
	if file.AddressIndex == nil {
		file.AddressIndex = map[string]string{}
		for userID, record := range file.Wallets {
			file.AddressIndex[normalizeAddress(record.DYDXAddress)] = userID
		}
	}
	if file.Version == 0 {
		file.Version = proxyWalletStoreVersion
	}
	return file, nil
}

func (s *EncryptedFileProxyWalletStore) saveLocked(file proxyWalletFile) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.conf.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".proxy-wallets-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, s.conf.Path)
}

func (s *EncryptedFileProxyWalletStore) encrypt(plain []byte) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *EncryptedFileProxyWalletStore) decrypt(value string) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, fmt.Errorf("encrypted private key payload is too short")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (r proxyWalletRecord) wallet() ProxyWallet {
	return ProxyWallet{
		UserID:      r.UserID,
		NECOAddress: r.NECOAddress,
		DYDXAddress: r.DYDXAddress,
		CreatedAt:   r.CreatedAt,
	}
}

func AddressFromPrivateKey(privateKey cryptotypes.PrivKey, prefix string) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("missing private key")
	}
	if prefix == "" {
		prefix = appconfig.Bech32PrefixAccAddr
	}
	return sdk.Bech32ifyAddressBytes(prefix, privateKey.PubKey().Address())
}

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

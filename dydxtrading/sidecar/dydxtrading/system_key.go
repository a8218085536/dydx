//go:build cgo

package dydxtrading

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

const DefaultSystemWalletHDPath = "m/44'/118'/0'/0/0"

var ErrSystemPrivateKeyAmbiguous = errors.New("configure either DYDX_SYSTEM_PRIVATE_KEY_HEX or DYDX_SYSTEM_MNEMONIC, not both")

type SystemPrivateKeyConfig struct {
	Hex             string
	Mnemonic        string
	HDPath          string
	BIP39Passphrase string
}

func SystemPrivateKeyFromConfig(conf SystemPrivateKeyConfig) (cryptotypes.PrivKey, bool, error) {
	keyHex := strings.TrimSpace(conf.Hex)
	mnemonic := normalizeMnemonic(conf.Mnemonic)
	if keyHex != "" && mnemonic != "" {
		return nil, false, ErrSystemPrivateKeyAmbiguous
	}
	if keyHex != "" {
		key, err := PrivateKeyFromHex(keyHex)
		return key, true, err
	}
	if mnemonic == "" {
		return nil, false, nil
	}
	key, err := PrivateKeyFromMnemonic(mnemonic, conf.BIP39Passphrase, conf.HDPath)
	return key, true, err
}

func PrivateKeyFromMnemonic(mnemonic, bip39Passphrase, hdPath string) (cryptotypes.PrivKey, error) {
	mnemonic = normalizeMnemonic(mnemonic)
	if mnemonic == "" {
		return nil, fmt.Errorf("missing system mnemonic")
	}
	if strings.TrimSpace(hdPath) == "" {
		hdPath = DefaultSystemWalletHDPath
	}
	derived, err := hd.Secp256k1.Derive()(mnemonic, bip39Passphrase, hdPath)
	if err != nil {
		return nil, fmt.Errorf("derive system private key from mnemonic: %w", err)
	}
	if len(derived) != secp256k1.PrivKeySize {
		return nil, fmt.Errorf("invalid derived system private key length %d", len(derived))
	}
	return hd.Secp256k1.Generate()(derived), nil
}

func normalizeMnemonic(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

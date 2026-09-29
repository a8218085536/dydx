//go:build cgo

package dydxtrading

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

const testSystemMnemonic = "circle music snake select deal march this romance until often welcome rich staff trigger drip exit there reopen denial insect hockey just wealth process"

func TestSystemPrivateKeyFromConfigDerivesMnemonicWithDefaultPath(t *testing.T) {
	key, configured, err := SystemPrivateKeyFromConfig(SystemPrivateKeyConfig{
		Mnemonic: testSystemMnemonic,
	})

	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, "1b5884fab5c22aeffef369f3454076cec534c75a4ee71add1245e2c8342994a2", hex.EncodeToString(key.Bytes()))
}

func TestSystemPrivateKeyFromConfigDerivesMnemonicWithPassphrase(t *testing.T) {
	key, configured, err := SystemPrivateKeyFromConfig(SystemPrivateKeyConfig{
		Mnemonic:        testSystemMnemonic,
		BIP39Passphrase: "test",
	})

	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, "d5f925b9472b3793e839eae7722e5cef6766a91fad55cc42abea5e457ed5f648", hex.EncodeToString(key.Bytes()))
}

func TestSystemPrivateKeyFromConfigRejectsHexAndMnemonicTogether(t *testing.T) {
	_, _, err := SystemPrivateKeyFromConfig(SystemPrivateKeyConfig{
		Hex:      "1b5884fab5c22aeffef369f3454076cec534c75a4ee71add1245e2c8342994a2",
		Mnemonic: testSystemMnemonic,
	})

	require.ErrorIs(t, err, ErrSystemPrivateKeyAmbiguous)
}

func TestSystemPrivateKeyFromConfigReturnsUnconfigured(t *testing.T) {
	key, configured, err := SystemPrivateKeyFromConfig(SystemPrivateKeyConfig{})

	require.NoError(t, err)
	require.False(t, configured)
	require.Nil(t, key)
}

func TestPrivateKeyFromMnemonicRejectsInvalidMnemonic(t *testing.T) {
	_, err := PrivateKeyFromMnemonic("not a valid mnemonic", "", "")

	require.Error(t, err)
}

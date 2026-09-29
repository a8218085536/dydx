package tyche

import (
	"context"
	"errors"
)

var ErrWalletBindingVerifierNotConfigured = errors.New("wallet binding verifier is not configured")

type UnconfiguredWalletBindingVerifier struct{}

func (UnconfiguredWalletBindingVerifier) VerifyNECO(context.Context, WalletBinding, string) error {
	return ErrWalletBindingVerifierNotConfigured
}

func (UnconfiguredWalletBindingVerifier) VerifyDYDX(context.Context, WalletBinding, string) error {
	return ErrWalletBindingVerifierNotConfigured
}

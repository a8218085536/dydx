package tyche

import "context"

type CompositeWalletBindingVerifier struct {
	NECO WalletBindingVerifier
	DYDX WalletBindingVerifier
}

func (v CompositeWalletBindingVerifier) VerifyNECO(ctx context.Context, binding WalletBinding, signature string) error {
	if v.NECO == nil {
		return ErrWalletBindingVerifierNotConfigured
	}
	return v.NECO.VerifyNECO(ctx, binding, signature)
}

func (v CompositeWalletBindingVerifier) VerifyDYDX(ctx context.Context, binding WalletBinding, signature string) error {
	if v.DYDX == nil {
		return ErrWalletBindingVerifierNotConfigured
	}
	return v.DYDX.VerifyDYDX(ctx, binding, signature)
}

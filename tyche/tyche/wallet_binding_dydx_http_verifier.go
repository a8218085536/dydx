package tyche

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type DYDXSignatureHTTPVerifierConfig struct {
	URL string
}

type DYDXSignatureHTTPVerifier struct {
	conf   DYDXSignatureHTTPVerifierConfig
	client *http.Client
}

func NewDYDXSignatureHTTPVerifier(conf DYDXSignatureHTTPVerifierConfig) *DYDXSignatureHTTPVerifier {
	return &DYDXSignatureHTTPVerifier{
		conf:   conf,
		client: http.DefaultClient,
	}
}

func (v *DYDXSignatureHTTPVerifier) VerifyNECO(context.Context, WalletBinding, string) error {
	return ErrWalletBindingVerifierNotConfigured
}

func (v *DYDXSignatureHTTPVerifier) VerifyDYDX(ctx context.Context, binding WalletBinding, signature string) error {
	if v.conf.URL == "" {
		return ErrWalletBindingVerifierNotConfigured
	}

	raw, err := json.Marshal(map[string]interface{}{
		"address":   binding.DYDXAddress,
		"message":   binding.Message,
		"signature": signature,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.conf.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Valid bool `json:"valid"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != http.StatusOK {
		if result.Msg == "" {
			result.Msg = fmt.Sprintf("signature verifier returned code %d", result.Code)
		}
		return errors.New(result.Msg)
	}
	if !result.Data.Valid {
		return ErrWalletBindingSignatureInvalid
	}
	return nil
}

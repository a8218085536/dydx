//go:build !cgo

package dydxtrading

const CGORequired = "dYdX native trading sidecar requires CGO_ENABLED=1 and a C compiler for secp256k1 signing"

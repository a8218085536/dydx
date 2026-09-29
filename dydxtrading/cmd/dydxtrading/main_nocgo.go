//go:build !cgo

package main

import "log"

func main() {
	log.Fatal("dYdX trading sidecar requires CGO_ENABLED=1 and a C compiler for secp256k1 signing")
}

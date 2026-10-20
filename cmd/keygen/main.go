//go:build !wasm

package main

import (
	"fmt"
	"os"

	"webtyp.com/base64"
	"webtyp.com/crypto/asym"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			fmt.Println("Usage: keygen")
			fmt.Println("Generates a P-256 key pair for IAM consumer clients.")
			os.Exit(0)
		}
	}

	privBytes, pubBytes, err := asym.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate key pair: %v\n", err)
		os.Exit(1)
	}

	privB64 := base64.URLEncode(privBytes)
	pubB64 := base64.URLEncode(pubBytes)

	fmt.Printf("IAM_CLIENT_PRIVATE_KEY=%s\n", privB64)
	fmt.Printf("public_key=%s\n", pubB64)

	fmt.Fprintln(os.Stderr, "Store the private key as a secret of the consumer; paste the public key in the iam panel.")
	os.Exit(0)
}

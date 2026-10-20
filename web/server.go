//go:build !wasm

package main

import (
	"log"

	"github.com/veltylabs/iam/config"
)

func main() {
	if err := config.RunServer(); err != nil {
		log.Fatal(err)
	}
}

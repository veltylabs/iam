//go:build !wasm

package main

import (
	"os"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/goflare/devserver"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/server/httpd"
	"github.com/tinywasm/storage/mem"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

const (
	DevPort      = "8080"
	DevPublicDir = "web/public"
)

func main() {
	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		fmt.Println("unixid:", err)
		os.Exit(1)
	}

	backend, err := config.NewLocalBackend(db, ids)
	if err != nil {
		fmt.Println("backend:", err)
		os.Exit(1)
	}
	// Local development uses simulated identities with no Google secrets.
	// Production (edge/main.go) uses only Google OAuth and fails fast without them.

	port := lookupArg("server_port")
	if port == "" {
		port = DevPort
	}
	publicDir := lookupArg("server_public_dir")
	if publicDir == "" {
		publicDir = DevPublicDir
	}

	srv := devserver.New(httpd.Config{
		Port:      port,
		PublicDir: publicDir,
		Authn:     backend.Auth.Authenticate(),
	})
	routes.Register(srv.Router(), db, backend.Auth, backend.RBAC, backend.JWTSecret)

	if err := srv.ListenAndServe(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func lookupArg(key string) string {
	prefix := "-" + key + "="
	args := os.Args[1:]
	for i, arg := range args {
		if fmt.HasPrefix(arg, prefix) {
			return fmt.Convert(arg).TrimPrefix(prefix).String()
		}
		if arg == "-"+key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

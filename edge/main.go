//go:build wasm

package main

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/goflare/cloudflare"
	"github.com/tinywasm/goflare/d1"
	"github.com/tinywasm/goflare/edge"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

func main() {
	db, err := d1.NewEdge(routes.BindingD1)
	if err != nil {
		fmt.Println("d1:", err)
		return
	}

	ids, err := unixid.NewUnixID()
	if err != nil {
		fmt.Println("unixid:", err)
		return
	}

	backend, err := config.NewProductionBackend(db, ids, cloudflare.Env)
	if err != nil {
		fmt.Println("backend:", err)
		return
	}

	r := edge.NewRouter(edge.Config{
		Authn: backend.Auth.Authenticate(),
	})
	routes.Register(r, db, backend.RBAC, backend.JWTSecret)
	edge.Serve(r)
}

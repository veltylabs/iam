//go:build wasm

package main

import (
	"webtyp.com/cloudflare/d1"
	"webtyp.com/cloudflare/edge"
	"webtyp.com/fmt"
	"github.com/veltylabs/iam/config"
)

func main() {
	db, err := d1.NewEdge(config.BindingD1)
	if err != nil {
		fmt.Println("d1:", err)
		return
	}

	ids, err := config.NewIDs()
	if err != nil {
		fmt.Println("ids:", err)
		return
	}

	backend, err := config.NewProductionBackend(db, ids)
	if err != nil {
		fmt.Println("backend:", err)
		return
	}

	r := edge.NewRouter(edge.Config{
		Authn:     backend.Auth.Authenticate(),
		Authorize: config.Authorizer(backend.RBAC),
	})
	config.RegisterRoutes(r, backend, config.PanelAdminList())
	edge.Serve(r)
}

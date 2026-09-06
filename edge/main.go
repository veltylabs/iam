//go:build wasm

package main

import (
	"webtyp.com/cloudflare/d1"
	"webtyp.com/cloudflare/edge"
	"webtyp.com/fmt"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

func main() {
	db, err := d1.NewEdge(routes.BindingD1)
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
		Authn: backend.Auth.Authenticate(),
	})
	routes.Register(r, db, backend.Auth, backend.RBAC, backend.JWTSecret, config.PanelAdminList(), ids, backend.PanelOrigin)
	edge.Serve(r)
}

//go:build wasm

package main

import (
	"syscall/js"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/goflare/cloudflare"
	"github.com/tinywasm/goflare/d1"
	"github.com/tinywasm/goflare/edge"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

func now() float64 {
	return js.Global().Get("Date").Call("now").Float()
}

func main() {
	t0 := now()

	db, err := d1.NewEdge(routes.BindingD1)
	if err != nil {
		fmt.Println("d1:", err)
		return
	}
	fmt.Println("timing: d1.NewEdge", now()-t0, "ms")

	ids, err := unixid.NewUnixID()
	if err != nil {
		fmt.Println("unixid:", err)
		return
	}
	fmt.Println("timing: unixid.NewUnixID", now()-t0, "ms")

	t1 := now()
	backend, err := config.NewProductionBackend(db, ids, cloudflare.Env)
	if err != nil {
		fmt.Println("backend:", err)
		return
	}
	fmt.Println("timing: NewProductionBackend total", now()-t1, "ms (cumulative", now()-t0, ")")

	r := edge.NewRouter(edge.Config{
		Authn: backend.Auth.Authenticate(),
	})
	routes.Register(r, db, backend.Auth, backend.RBAC, backend.JWTSecret)
	fmt.Println("timing: router+register", now()-t0, "ms")
	edge.Serve(r)
}

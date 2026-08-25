package tests

import (
	"testing"

	"github.com/tinywasm/router/mock"
	"github.com/veltylabs/iam/routes"
)

// TestRegister_MountsAuthRoutes is the regression test for the defect this
// plan fixes: Register never called authMod.MountAPI(r), so /oauth/google,
// /oauth/callback/google and /logout 404'd in every environment since Stage
// 4 shipped. mock.Router enforces the same routing contract the real
// implementations do (method matching, closed-by-default access) — it is
// not a recorder, so a route that Invoke reaches here reaches it in
// production too.
func TestRegister_MountsAuthRoutes(t *testing.T) {
	backend := setupBackend(t)

	r := &mock.Router{}
	routes.Register(r, backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret)

	cases := []struct {
		method, path string
	}{
		{"GET", "/oauth/google"},
		{"GET", "/oauth/callback/google"},
		{"POST", "/logout"},
	}
	for _, c := range cases {
		found := false
		for _, info := range r.Routes() {
			if info.Method == c.method && info.Path == c.path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s %s is not registered — Register() never mounted authMod's routes", c.method, c.path)
		}
	}
}

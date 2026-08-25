---
PLAN: "fix!: Register() mounts authMod's own routes — OAuth login has 404'd since Stage 4"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `veltylabs/iam`: `/oauth/google` and `/logout` have never been reachable

## Investigated against `tinywasm/app-releases/docs/CONSTRUCTION_HARNESS.md`

The instruction behind this plan was to find where route-mounting fails the
harness and fix it "definitively in the API or the way of registration."
Having read `authority.Module`'s actual source, the honest finding is: **the
library is not the problem.** `authority.Module` already implements exactly
the harness's own documented pattern —

```go
// router/router.go
type APIModule interface {
	model.ModuleNaming
	MountAPI(r Router)
}
```

```go
// tinywasm/auth/authority/mount.go
func (m *Module) MountAPI(r router.Router) {
	r.Post(auth.PathLogout, func(ctx router.Context) { ... }).Public()
	for _, auth := range m.authenticators {
		auth.Mount(r)
	}
}
```

`MountAPI` is correct, typed, and already does exactly what login needs:
mounts `/logout`, then lets every enabled authenticator (the Google
`oauth2.Authenticator` this repo constructs in `config/auth.go` /
`config/auth_local.go`) mount its own `/oauth/<provider>` and
`/oauth/callback/<provider>` routes.

The defect is in **this repo**: [`routes/routes.go`](../routes/routes.go)'s
`Register` never calls it.

```go
func Register(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte) {
	r.Get(PathHealth, Health(db)).Public()
	r.Post(PathToken, Token(db, authMod, rbacSvc, secret)).Authenticated()
	r.Post(PathUsersResolve, ResolveUser(db, authMod)).Public()
}
```

`config/auth.go`'s `NewProductionAuth` builds `authMod` with OAuth fully
enabled —

```go
authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{g},
	oauth2.WithRedirectValidator(isVeltyDomain),
))
```

— and `config/auth_local.go`'s `NewLocalAuth` does the same for local dev
(mock Google + a LAN mode). Both `edge/main.go` (production) and
`web/server.go` (local dev) call `routes.Register` as their **only** routing
entry point. Since `Register` never calls `authMod.MountAPI(r)`, `/oauth/*`
and `/logout` have 404'd in every environment, in every deploy, since Stage
4 shipped — silently: nothing errors, nothing panics, the routes simply do
not exist. No test caught it because no test in this repo ever calls
`routes.Register` at all (`grep -rln "routes.Register" tests/` is empty
today) — every existing test drives a handler function directly.

This is precisely `CONSTRUCTION_HARNESS.md`'s own category of defect: *"Things
you 'have to remember.' Any mandatory step the author must remember to call
… that is a hole in the harness; close it with types or a single path, not
with prose."* The harness rule applies here to the **call site**, not the
library: nothing forced `Register` to mount every module it holds.

## Decision: composition root

Two candidates were weighed; composition root is the one to build. The
one-line alternative is kept below only as the rejected baseline this
decision improves on.

### Rejected: the one-line fix

```go
func Register(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte) {
	authMod.MountAPI(r)

	r.Get(PathHealth, Health(db)).Public()
	r.Post(PathToken, Token(db, authMod, rbacSvc, secret)).Authenticated()
	r.Post(PathUsersResolve, ResolveUser(db, authMod)).Public()
}
```

Correct, minimal, fixes both environments at once (both call `Register`).
Its weakness is exactly what caused the bug: it is still one call a future
edit to this function could delete or a future second `APIModule` (see
below) could be added without.

### Chosen: composition root

Same fix, structured as the single list every `router.APIModule` this repo
owns must appear in — the exact pattern `CONSTRUCTION_HARNESS.md` itself
documents as "the assembly pattern":

```go
// Register monta todas las rutas de iam en el router.
//
// modules is the single, exhaustive list of everything this server mounts
// through router.APIModule. It is intentionally the ONLY place iam calls
// MountAPI: a module with routes to serve belongs in this slice, never
// invoked ad hoc elsewhere, so adding a module here is the only way its
// routes reach a caller — and leaving it out is the only way they don't.
// That is what closes the hole this plan exists for: authMod's OAuth routes
// (/oauth/google, /oauth/callback/google) and /logout were built and
// enabled (config/auth.go, config/auth_local.go) but never mounted, so
// every login attempt 404'd, silently, since Stage 4 — MountAPI existed and
// was correct, nothing ever called it.
func Register(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte) {
	modules := []router.APIModule{authMod}
	for _, m := range modules {
		m.MountAPI(r)
	}

	r.Get(PathHealth, Health(db)).Public()
	r.Post(PathToken, Token(db, authMod, rbacSvc, secret)).Authenticated()
	r.Post(PathUsersResolve, ResolveUser(db, authMod)).Public()
}
```

Costs four lines over Option A. Buys: the next module this repo grows that
implements `router.APIModule` (a plausible future — `rbacSvc` does not
implement it today, `grep -rn "func.*MountAPI" $(go env
GOMODCACHE)/github.com/tinywasm/rbac@*/*.go` is empty, but nothing rules out
an admin CRUD surface later) is mounted by construction the moment it is
added to `modules`, not by remembering a new call. This is not
speculative scope — it is the same pattern
`tinywasm/app-releases/docs/CONSTRUCTION_HARNESS.md` itself gives as the
canonical example of a composition root, applied here instead of invented.

Decided: composition root. It costs almost nothing over the one-line fix and
is the one that actually closes the harness gap for the *next* module, not
just this one.

## Why the fix does not belong in `tinywasm/auth`

Considered and rejected: could `authority.New`/`Enable` mount its own routes
automatically, closing this for every consumer at once? No —
`NewProductionAuth` (`config/auth.go`) builds `authMod` in stages
(`authority.New` → `rbac.New` → `authMod.Enable(oauth2.New(...))` →
`authMod.SetStrategy(...)`) and returns it **before** a `router.Router`
exists: `edge/main.go` only constructs the router afterward, because
`edge.NewRouter(edge.Config{Authn: backend.Auth.Authenticate()})` needs a
method value off the already-built `backend.Auth`. Threading a `router.Router`
into `NewProductionAuth`/`authority.New` would require restructuring that
ordering across every consumer of `tinywasm/auth` and `goflare/edge`, to fix
a defect that exists only in this repo's `Register`. `MountAPI` staying a
separate, explicit call — made from the one composition root that already
holds both `authMod` and `r` at the right point in the sequence — is the
correctly-scoped fix.

## Stage 1 — mount `authMod` in `Register`

Apply the composition-root version to
[`routes/routes.go`](../routes/routes.go), exactly as shown above.

## Stage 2 — prove it, the way nothing did before

New file `tests/register_test.go`:

```go
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
```

Confirm `setupBackend`'s existing fixture (`tests/setup_test.go`) already
sets the env vars `NewProductionAuth` needs (`GOOGLE_CLIENT_ID`,
`GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL`, `JWT_SECRET`) before reusing
it — it does, verified when this plan was written. If `mock.Router`'s
constructor or `Routes()` signature differs from what is shown here in the
version this repo depends on, adapt the test to match the real API — do not
change the production code to fit an imagined test API.

## Stage 3 — documentation

[`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md): if it lists `Register`'s
responsibilities or iam's login flow, update it to state that `Register` is
the sole composition root — the only place any `router.APIModule` this
service owns gets mounted — so a future contributor adding a module knows
where it belongs instead of hand-registering routes elsewhere.

## Acceptance criteria

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./tests/...` — full suite green, including the new
      `TestRegister_MountsAuthRoutes`.
- [ ] `grep -n "MountAPI" routes/routes.go` → present, called from `Register`.
- [ ] Manually confirmed (or by a `TestRegister_MountsAuthRoutes`-equivalent
      exercise) that `GET /oauth/google` no longer 404s against a
      `mock.Router`-composed `Register`.

| Stage | File(s) | Done when |
|---|---|---|
| 1 | `routes/routes.go` | `Register` mounts every `router.APIModule` it holds via the `modules` slice (composition root) |
| 2 | `tests/register_test.go` | New test proves `/oauth/google`, `/oauth/callback/google`, `/logout` are reachable |
| 3 | `docs/ARCHITECTURE.md` | States `Register` as the sole composition root, if the doc describes routing at all |

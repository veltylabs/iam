---
PLAN: "fix!: migrate the schema at deploy time — main() stops paying 8.5s per cold start"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **BLOCKED — do not start until `tinywasm/auth` and `tinywasm/rbac` have
> published their `Migrate` plans** (`auth/docs/PLAN.md`,
> `rbac/docs/PLAN.md`). Both must be released before this repo can drop
> schema work from startup; bumping to those versions is Stage 1's first
> action. `tinywasm/goflare` v0.5.22 (`NewD1Migrator`) and `tinywasm/ddl`
> v0.0.12 (`Execer`) are already published.

# Plan — `veltylabs/iam`: the cold start stops reconciling the schema

## Why — this is where the measured win lands

`iam` answers cold-start requests in **1.4–10.4 s**. Measured, not
estimated: `main()` was instrumented with `Date.now()` deltas, deployed, and
the instrumentation reverted (this repo's two `debug:` commits, 2026-08-25):

```
timing: d1.NewEdge                0 ms
timing: unixid.NewUnixID          0 ms
timing: NewProductionBackend   8531 ms  /  10407 ms
```

All of it is schema reconciliation: `NewProductionAuth` builds
`authority.New` (5 models) and `rbac.New` (4 models), then
`NewProductionBackend` calls `initProjectSchema` (1 model) — ~10 models,
each at least one D1 round trip, on **every isolate cold start**. Cloudflare
recycles isolates constantly, so users pay it repeatedly.

Everything upstream is now in place to fix it:

| Piece | Published | What it gives |
|---|---|---|
| `tinywasm/ddl` v0.0.12 | ✅ | `ddl.Execer` — a DDL-only connection is expressible |
| `tinywasm/goflare` v0.5.22 | ✅ | `NewD1Migrator` — a `ddl.Execer` over D1's HTTP API, from CI |
| `tinywasm/auth` | ⏳ this plan's blocker | `authority.Migrate(conn, compiler)`; `New` stops running DDL |
| `tinywasm/rbac` | ⏳ this plan's blocker | `rbac.Migrate(conn, compiler)`; `New` stops running DDL |

This plan is the consumer that turns all of it into the actual latency win.

## Stage 1 — bump, and stop migrating at startup

Bump `tinywasm/auth` and `tinywasm/rbac` to the versions that export
`Migrate`. Then:

**`config/projects.go`** — replace `initProjectSchema` with an exported
`MigrateProjects`, matching the shape the two libraries now use:

```go
// MigrateProjects reconciles the schema this service owns (Project).
//
// Deliberately not called from NewProductionBackend: schema reconciliation
// is deploy-time work. Doing it per process start cost ~10 D1 round trips
// on every isolate cold start (8.5–10.4 s measured). cmd/migrate calls this
// once, from CI.
func MigrateProjects(conn ddl.Execer, ddlCompiler ddl.Compiler) error {
	return ddl.New(conn, ddlCompiler).Sync(&Project{})
}
```

The old function silently returned `nil` when the `ddl.Compiler` assertion
failed. That swallow goes away on purpose — the compiler is now a required
argument, so a caller who cannot supply one gets a compile error instead of
a migration that quietly did nothing.

**`config/backend.go`** — delete the `initProjectSchema(db)` call and its
error branch from `NewProductionBackend`. Nothing else in that function
changes.

Do **not** add a "migrate if needed" check, an env flag, or a lazy
once-per-isolate guard. The point is that the Worker performs no schema I/O
at all.

## Stage 2 — the migration binary

New file `cmd/migrate/main.go` (`//go:build !wasm` — it runs on the CI
host, never in a Worker):

```go
//go:build !wasm

// Command migrate reconciles iam's database schema. It runs once per
// deploy, from CI, against D1's HTTP API — never inside the Worker, whose
// main() must not do schema I/O (see docs/PLAN.md history: that cost
// 8.5–10.4 s per cold start).
package main

import (
	"fmt"
	"os"

	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/goflare"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/sqlt"
	"github.com/veltylabs/iam/config"
)

func main() {
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	databaseID := os.Getenv("D1_DATABASE_ID")
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")

	conn, err := goflare.NewD1Migrator(accountID, databaseID, apiToken)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	compiler := sqlt.NewCompiler()

	// Order matters only in that every package owns disjoint tables; each
	// call topologically sorts its own models internally.
	steps := []struct {
		name string
		run  func() error
	}{
		{"authority", func() error { return authority.Migrate(conn, compiler) }},
		{"rbac", func() error { return rbac.Migrate(conn, compiler) }},
		{"projects", func() error { return config.MigrateProjects(conn, compiler) }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			fmt.Fprintln(os.Stderr, "migrate:", s.name+":", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "migrate:", s.name, "ok")
	}
}
```

Per the ecosystem's CLI contract (skill: core-principles — "AI-Consumable
CLIs"): diagnostics go to **stderr**, exit `0` on success and non-zero on
failure. There is nothing to print on stdout.

Verify `sqlt.NewCompiler()` is the right compiler for D1 before trusting
this: `tinywasm/cloudflare/d1`'s own `NewEdge` adapter uses exactly that, on
the grounds that D1 *is* SQLite.

## Stage 3 — the CI step

`.github/workflows/deploy.yml`: add a migration step **before** "Deploy with
goflare", guarded by the same condition so it only runs on a real
deployment:

```yaml
      - name: Migrate D1 schema
        if: github.ref == 'refs/heads/main' && github.event_name == 'push'
        env:
          D1_DATABASE_ID: ${{ secrets.D1_DATABASE_ID }}
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
        run: go run ./cmd/migrate
```

All three secrets already exist in this workflow (the deploy step uses
them) — no new repository secrets are needed.

**Migrate before deploy, not after**, and let a failure stop the job: a
Worker deployed against a schema that failed to reconcile would fail at its
first query, in production, with no clue why. `goflare deploy` already runs
a post-deploy probe that would then fail confusingly.

## Stage 4 — tests

`tests/` builds its fixtures against an in-memory DB, and those fixtures
relied on `authority.New`/`rbac.New`/`initProjectSchema` creating tables.
That no longer happens, so **every fixture must migrate explicitly** or its
first query fails.

Find them: `grep -rn "NewProductionBackend\|NewProductionAuth\|setupBackend\|setup(" tests/*.go`.
`tests/setup_test.go` is the shared fixture (`setup`, `setupBackend`,
`setupLocal`) — fix it there rather than at each call site. For an
in-memory DB the three calls are:

```go
	conn := db.RawConn()
	compiler, ok := conn.(ddl.Compiler)
	if !ok {
		t.Fatal("in-memory conn does not compile DDL")
	}
	if err := authority.Migrate(conn, compiler); err != nil { t.Fatal(err) }
	if err := rbac.Migrate(conn, compiler); err != nil { t.Fatal(err) }
	if err := config.MigrateProjects(conn, compiler); err != nil { t.Fatal(err) }
```

Add one new test — `TestProductionBackend_DoesNotMigrate` — that builds
`NewProductionBackend` against a DB with **no** tables and asserts it
returns without error. That is the regression guard for the whole plan: if
someone reintroduces schema work in the constructor, this test fails.

## Stage 5 — documentation

`docs/ARCHITECTURE.md`: state that schema reconciliation happens at deploy
time via `cmd/migrate`, that `NewProductionBackend` assumes the schema
exists, and why (the cold-start measurement). `docs/DEPLOY.md`: add the
migration step to the deploy sequence.

## Verify the win — do not skip this

The whole plan exists for one number. After deploying, confirm it moved:

```bash
curl -s -o /dev/null -w "%{time_total}\n" https://iam.velty.cl/api/health
```

and read `wallTimeMs`/`cpuTimeMs` from Workers Observability for those
requests. Expect cold-start wall time to drop from seconds to well under
a second. **Observability is reset by every `goflare deploy`** — re-enable
it (`PATCH /accounts/{id}/workers/scripts/iam/settings` with
`{"observability":{"enabled":true,"head_sampling_rate":1}}`) before
measuring, or the query returns nothing.

If the number does *not* move, stop and report it rather than closing the
plan: the measurement is the acceptance criterion, not the code change.

## Acceptance criteria

- [ ] `go build ./...`, `GOOS=js GOARCH=wasm go build ./...`, `go vet ./...` clean.
- [ ] `go test ./tests/...` green, including `TestProductionBackend_DoesNotMigrate`.
- [ ] `grep -rn "initProjectSchema" --include="*.go" .` → empty.
- [ ] `grep -rn "Migrate" config/backend.go edge/main.go` → empty (neither
      the backend constructor nor the Worker entry point migrates).
- [ ] `go run ./cmd/migrate` against the real D1 succeeds (CI proves this).
- [ ] Cold-start wall time measured after deploy, and reported.

| Stage | File(s) | Done when |
|---|---|---|
| 1 | `config/projects.go`, `config/backend.go` | `MigrateProjects` exported; `NewProductionBackend` does no schema I/O |
| 2 | `cmd/migrate/main.go` | One binary migrates authority + rbac + projects over `NewD1Migrator` |
| 3 | `.github/workflows/deploy.yml` | Migration runs before deploy, on the same guard, with existing secrets |
| 4 | `tests/setup_test.go`, new regression test | Fixtures migrate explicitly; constructor proven not to |
| 5 | `docs/ARCHITECTURE.md`, `docs/DEPLOY.md` | Deploy-time migration documented |

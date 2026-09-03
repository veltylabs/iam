//go:build !wasm

// Command migrate reconciles iam's database schema. It runs once per
// deploy, from CI, against D1's HTTP API — never inside the Worker, whose
// main() must not do schema I/O (see config.MigrateSchema: ~10 D1 round
// trips on every isolate cold start).
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
		{"projects", func() error { return config.MigrateSchema(conn, compiler) }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			fmt.Fprintln(os.Stderr, "migrate:", s.name+":", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "migrate:", s.name, "ok")
	}
}

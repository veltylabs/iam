//go:build !wasm

package main

import (
	"os"

	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/server/httpd"
	"github.com/tinywasm/sqlt"
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

	conn := db.RawConn()
	compiler := sqlt.NewCompiler()
	if err := authority.Migrate(conn, compiler); err != nil {
		fmt.Println("migrate: authority:", err)
		os.Exit(1)
	}
	if err := rbac.Migrate(conn, compiler); err != nil {
		fmt.Println("migrate: rbac:", err)
		os.Exit(1)
	}
	if err := config.MigrateSchema(conn, compiler); err != nil {
		fmt.Println("migrate: schema:", err)
		os.Exit(1)
	}

	backend, err := config.NewLocalBackend(db, ids)
	if err != nil {
		fmt.Println("backend:", err)
		os.Exit(1)
	}

	// En local, la identidad mock (config.LocalScenarios[0]) entra al panel sin
	// configurar IAM_ADMIN_EMAILS. Sobrescribible con -server_admin_emails.
	adminEmails := config.PanelAdminList()
	if len(adminEmails) == 0 {
		if a := env.Arg("server_admin_emails"); a != "" {
			adminEmails = []string{a}
		} else {
			adminEmails = []string{config.LocalScenarios[0].Email}
		}
	}

	port := env.Arg("server_port")
	if port == "" {
		port = DevPort
	}
	publicDir := env.Arg("server_public_dir")
	if publicDir == "" {
		publicDir = DevPublicDir
	}

	srv := httpd.New(httpd.Config{
		Port:           port,
		PublicDir:      publicDir,
		Authn:          backend.Auth.Authenticate(),
		NoCache:        true,
		RoutesEndpoint: true,
	})
	routes.Register(srv.Router(), db, backend.Auth, backend.RBAC, backend.JWTSecret, adminEmails, ids)

	srv.Router().PublicDir("", publicDir)

	if err := srv.ListenAndServe(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

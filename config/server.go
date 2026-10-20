//go:build !wasm

package config

import (
	"webtyp.com/env"
	"webtyp.com/orm"
	"webtyp.com/server/httpd"
	"webtyp.com/storage/mem"
)

const (
	DevPort      = "8080"
	DevPublicDir = "web/public"
)

// RunServer is the local development server. webtyp's generator sees it and
// writes web/server.go as `config.RunServer()`.
func RunServer() error {
	db := orm.New(mem.New())
	ids, err := NewIDs()
	if err != nil {
		return err
	}

	backend, err := NewLocalBackend(db, ids)
	if err != nil {
		return err
	}

	adminEmails := PanelAdminList()
	if len(adminEmails) == 0 {
		if a := env.Arg("server_admin_emails"); a != "" {
			adminEmails = []string{a}
		} else {
			adminEmails = []string{LocalScenarios[0].Email}
		}
	}

	s := httpd.New(httpd.Config{
		Port:      DevPort,
		PublicDir: DevPublicDir,
		Gzip:      true,
		Health:    true,
		TLS:       httpd.TLSConfig{PlainHTTP: false},
		Authn:     backend.Auth.Authenticate(),
		Authorize: Authorizer(backend.RBAC),
	})

	RegisterRoutes(s.Router(), backend, adminEmails)
	return s.ListenAndServe()
}

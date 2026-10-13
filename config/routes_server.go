//go:build !wasm

package config

import (
	"webtyp.com/env"
	"webtyp.com/fmt"
	"webtyp.com/orm"
	"webtyp.com/router"
	"webtyp.com/sqlt"
	"webtyp.com/storage/mem"
)

// registerLocal monta todas las rutas del servicio iam sobre el router provisto para el servidor local.
func registerLocal(r router.Router) {
	db := orm.New(mem.New())
	ids, err := NewIDs()
	if err != nil {
		fmt.Println("ids:", err)
		return
	}

	conn := db.RawConn()
	compiler := sqlt.NewCompiler()
	if err := MigrateAll(conn, compiler); err != nil {
		fmt.Println("migrate:", err)
		return
	}

	backend, err := NewLocalBackend(db, ids)
	if err != nil {
		fmt.Println("backend:", err)
		return
	}

	adminEmails := PanelAdminList()
	if len(adminEmails) == 0 {
		if a := env.Arg("server_admin_emails"); a != "" {
			adminEmails = []string{a}
		} else {
			adminEmails = []string{LocalScenarios[0].Email}
		}
	}

	RegisterRoutes(r, db, backend.Auth, backend.RBAC, backend.JWTSecret, adminEmails, ids, backend.PanelOrigin)
}

package config

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/rbac"
)

// MigrateAll reconcilia el esquema completo que iam necesita, en orden de
// propiedad: identidad (`authority`), roles (`rbac`) y lo propio
// (`MigrateSchema`: proyectos y auditoría). Cada paquete posee tablas
// disjuntas y ordena sus modelos internamente.
//
// Es el único sitio que enumera las tres migraciones: `web/server.go` (dev
// local) y los tests lo llaman en vez de repetir el trío. `cmd/migrate`
// (deploy contra D1) mantiene sus pasos nombrados a propósito, para que el
// log del deploy diga qué parte falló.
func MigrateAll(conn ddl.Execer, compiler ddl.Compiler) error {
	if err := authority.Migrate(conn, compiler); err != nil {
		return err
	}
	if err := rbac.Migrate(conn, compiler); err != nil {
		return err
	}
	return MigrateSchema(conn, compiler)
}

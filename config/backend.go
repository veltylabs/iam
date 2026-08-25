package config

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/env"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
)

// Backend agrupa los módulos de dominio que comparten la misma base y
// generador de IDs — es el único lugar donde se orquesta
// NewProductionAuth/NewLocalAuth + initProjectSchema, así que tanto
// edge/main.go (D1 real) como web/server.go (memoria) y los tests lo
// llaman con la DB inyectada, y la lógica se prueba una vez.
type Backend struct {
	Auth *authority.Module
	RBAC *rbac.Service
	DB   *orm.DB
	// JWTSecret firma tanto la cookie de identidad (Etapa 4) como el token
	// de autorización project-scoped (Etapa 3, ver routes.Token) — un solo
	// secreto HS256 para todo iam.
	JWTSecret []byte
}

// NewProductionBackend inicializa identidad, RBAC y el esquema de
// proyectos para producción (Google OAuth + cookie SSO cross-dominio).
func NewProductionBackend(db *orm.DB, ids model.IDGenerator) (*Backend, error) {
	authMod, rbacSvc, err := NewProductionAuth(db, ids)
	if err != nil {
		return nil, err
	}
	if err := initProjectSchema(db); err != nil {
		return nil, err
	}
	// NewProductionAuth ya validó que EnvJWTSecret existe (falla antes de
	// llegar aquí si falta) — releerla es una lectura trivial, no lógica
	// duplicada.
	return &Backend{Auth: authMod, RBAC: rbacSvc, DB: db, JWTSecret: []byte(env.Get(EnvJWTSecret))}, nil
}

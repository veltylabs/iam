package config

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
)

// Backend agrupa los módulos de dominio que comparten la misma base y
// generador de IDs — es el único lugar donde se orquesta
// NewProductionAuth/NewLocalAuth, así que tanto
// edge/main.go (D1 real) como web/server.go (memoria) y los tests lo
// llaman con la DB inyectada, y la lógica se prueba una vez.
type Backend struct {
	Auth *authority.Module
	RBAC *rbac.Service
	DB   *orm.DB
	IDs  model.IDGenerator
	// JWTSecret firma tanto la cookie de identidad (Etapa 4) como el token
	// de autorización project-scoped (Etapa 3, ver routes.Token) — un solo
	// secreto HS256 para todo iam.
	JWTSecret []byte
	// PanelOrigin es el origen exacto desde el que se sirve el panel
	// (EnvPanelOrigin). Las mutaciones de /api/admin/* sólo aceptan
	// peticiones de este origen (ver modules/admin/origin.go).
	PanelOrigin string
}

// NewProductionBackend inicializa identidad, RBAC y el esquema de
// proyectos para producción (Google OAuth + cookie SSO cross-dominio).
func NewProductionBackend(db *orm.DB, ids model.IDGenerator) (*Backend, error) {
	authMod, rbacSvc, err := NewProductionAuth(db, ids)
	if err != nil {
		return nil, err
	}
	// NewProductionAuth ya validó que EnvJWTSecret existe (falla antes de
	// llegar aquí si falta) — releerla es una lectura trivial, no lógica
	// duplicada.
	panelOrigin := env.Get(EnvPanelOrigin)
	if panelOrigin == "" {
		return nil, fmt.Errf("iam: missing required environment variable %s", EnvPanelOrigin)
	}
	return &Backend{Auth: authMod, RBAC: rbacSvc, DB: db, IDs: ids, JWTSecret: []byte(env.Get(EnvJWTSecret)), PanelOrigin: panelOrigin}, nil
}

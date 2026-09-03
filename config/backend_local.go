//go:build !wasm

package config

import (
	"github.com/tinywasm/env"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
)

// LocalJWTSecret es el secreto HS256 fijo de desarrollo/tests, donde no
// hay variables de entorno de producción. Nunca se usa en producción:
// NewProductionAuth exige EnvJWTSecret y falla rápido si falta.
var LocalJWTSecret = []byte("iam-local-dev-secret-do-not-use-in-prod")

// NewLocalBackend inicializa el mismo motor para desarrollo, sin Google ni
// dominio de cookie compartido (ver NewLocalAuth — !wasm only, igual que
// esta función: nunca compila dentro del Worker de producción).
func NewLocalBackend(db *orm.DB, ids model.IDGenerator) (*Backend, error) {
	authMod, rbacSvc, err := NewLocalAuth(db, ids)
	if err != nil {
		return nil, err
	}
	// Sólo para desarrollo: si IAM_PANEL_ORIGIN está fijada se respeta,
	// si no se usa el origen del servidor local.
	panelOrigin := env.Get(EnvPanelOrigin)
	if panelOrigin == "" {
		panelOrigin = LocalPanelOrigin
	}
	return &Backend{Auth: authMod, RBAC: rbacSvc, DB: db, IDs: ids, JWTSecret: LocalJWTSecret, PanelOrigin: panelOrigin}, nil
}

package config

import (
	"webtyp.com/access"
	"webtyp.com/auth/authority"
	"webtyp.com/auth/clientassertion"
	"webtyp.com/env"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/rbac"
)

// Backend agrupa los módulos de dominio que comparten la misma base y
// generador de IDs — es el único lugar donde se orquesta
// NewProductionAuth/NewLocalAuth.
type Backend struct {
	Access    *access.Module
	Auth      *authority.Module
	RBAC      *rbac.Service
	DB        *orm.DB
	IDs       model.IDGenerator
	JWTSecret []byte
	Origin    string
}

// NewProductionBackend inicializa identidad, RBAC, política y machine clients para producción.
func NewProductionBackend(db *orm.DB, ids model.IDGenerator) (*Backend, error) {
	panelOrigin := env.Get(EnvPanelOrigin)
	if panelOrigin == "" {
		return nil, fmt.Errf("iam: missing required environment variable %s", EnvPanelOrigin)
	}

	authMod, rbacSvc, err := NewProductionAuth(db, ids)
	if err != nil {
		return nil, err
	}

	accessMod, err := access.New(access.Deps{
		DB:        db,
		IDs:       ids,
		Users:     authMod,
		Roles:     rbacSvc,
		ProjectID: IAMProjectID,
		Assurance: model.AssuranceMultiFactor,
	})
	if err != nil {
		return nil, err
	}

	adminEmails := PanelAdminList()
	if err := EnsurePolicy(rbacSvc, authMod, accessMod, adminEmails); err != nil {
		return nil, err
	}

	ca, err := clientassertion.New(db, ProjectKeys{DB: db}, panelOrigin, authMod.Notify)
	if err != nil {
		return nil, err
	}
	authMod.Enable(ca)

	return &Backend{
		Access:    accessMod,
		Auth:      authMod,
		RBAC:      rbacSvc,
		DB:        db,
		IDs:       ids,
		JWTSecret: []byte(env.Get(EnvJWTSecret)),
		Origin:    panelOrigin,
	}, nil
}

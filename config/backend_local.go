//go:build !wasm

package config

import (
	"webtyp.com/access"
	"webtyp.com/auth/clientassertion"
	"webtyp.com/env"
	"webtyp.com/model"
	"webtyp.com/orm"
)

// LocalJWTSecret es el secreto HS256 fijo de desarrollo/tests, donde no
// hay variables de entorno de producción.
var LocalJWTSecret = []byte("iam-local-dev-secret-do-not-use-in-prod")

// NewLocalBackend inicializa el mismo motor para desarrollo.
func NewLocalBackend(db *orm.DB, ids model.IDGenerator) (*Backend, error) {
	panelOrigin := env.Get(EnvPanelOrigin)
	if panelOrigin == "" {
		panelOrigin = LocalPanelOrigin
	}

	authMod, rbacSvc, err := NewLocalAuth(db, ids)
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
	if len(adminEmails) == 0 {
		if a := env.Arg("server_admin_emails"); a != "" {
			adminEmails = []string{a}
		} else {
			adminEmails = []string{LocalScenarios[0].Email}
		}
	}
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
		JWTSecret: LocalJWTSecret,
		Origin:    panelOrigin,
	}, nil
}

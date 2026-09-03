//go:build !wasm

package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/auth/local"
	"github.com/tinywasm/auth/oauth2"
	googlemock "github.com/tinywasm/auth/oauth2/provider/google/mock"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/user"
)

// LocalScenarios son identidades de desarrollo determinísticas para probar
// iam en aislamiento, sin secretos de Google. Sin roles asignados: cada
// test decide qué rol/permiso probar (ver bootstrap_test.go).
var LocalScenarios = []local.Scenario{
	{ID: user.SubjectID("local-admin"), Name: "Admin Local", Email: "admin@iam.local"},
	{ID: user.SubjectID("local-viewer"), Name: "Viewer Local", Email: "viewer@iam.local"},
}

// NewLocalAuth arma el motor de identidad+RBAC para desarrollo, sin Google.
func NewLocalAuth(db *orm.DB, ids model.IDGenerator) (*authority.Module, *rbac.Service, error) {
	return NewLocalAuthWithScenarios(db, ids, LocalScenarios)
}

// NewLocalAuthWithScenarios permite a los tests inyectar escenarios propios.
func NewLocalAuthWithScenarios(db *orm.DB, ids model.IDGenerator, scenarios []local.Scenario) (*authority.Module, *rbac.Service, error) {
	authMod, err := authority.New(db, auth.Config{
		IDs:        ids,
		CookieName: CookieSession,
		TokenTTL:   TTLSession,
		TrustProxy: true,
	})
	if err != nil {
		return nil, nil, err
	}
	rbacSvc, err := rbac.New(db)
	if err != nil {
		return nil, nil, err
	}
	for _, sc := range scenarios {
		if _, err := authMod.GetOrCreateSubject(sc.ID, sc.Email, sc.Name, sc.Avatar); err != nil {
			return nil, nil, err
		}
	}
	// El mock simula a Google, que siempre entrega emails verificados: sin
	// EmailVerified el flujo nuevo de auth rechaza el login cuando el email
	// ya existe (protección anti-takeover) y el escenario local no entraría.
	mockUser := auth.OAuthUserInfo{ID: string(scenarios[0].ID), Email: scenarios[0].Email, Name: scenarios[0].Name, Avatar: scenarios[0].Avatar, EmailVerified: true}
	mockProv := &googlemock.MockProvider{User: mockUser}
	authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{mockProv},
		oauth2.WithRedirectValidator(isVeltyDomain),
	))
	loc := local.New(scenarios, authMod, authMod, local.WithAfterLogin("/"))
	authMod.Enable(loc)
	return authMod, rbacSvc, nil
}

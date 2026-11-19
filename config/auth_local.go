//go:build !wasm

package config

import (
	"webtyp.com/auth"
	"webtyp.com/auth/authority"
	"webtyp.com/auth/local"
	"webtyp.com/auth/oauth2"
	googlemock "webtyp.com/auth/oauth2/provider/google/mock"
	"webtyp.com/auth/passkey"
	"webtyp.com/auth/session/cookie"
	"webtyp.com/events/inprocess"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/rbac"
	"webtyp.com/securityevent"
	"webtyp.com/user"
)

// LocalScenarios son identidades de desarrollo determinísticas para probar
// iam en aislamiento, sin secretos de Google.
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
	broker := &inprocess.Broker{}
	report, err := securityevent.New(broker)
	if err != nil {
		return nil, nil, err
	}
	authMod, err := authority.New(db, auth.Config{
		IDs:        ids,
		TrustProxy: true,
		Events:     broker,
	})
	if err != nil {
		return nil, nil, err
	}

	rbacSvc, err := rbac.New(db)
	if err != nil {
		return nil, nil, err
	}

	authMod.Enable(oauth2.New(authMod, authMod, authMod, report,
		[]auth.OAuthProvider{&googlemock.MockProvider{User: auth.OAuthUserInfo{ID: "local-admin", Email: "admin@iam.local", Name: "Admin Local"}}},
		oauth2.WithRedirectValidator(isVeltyDomain),
	))
	loc := local.New(scenarios, authMod, authMod, local.WithAfterLogin("/"))
	authMod.Enable(loc)

	sso, err := cookie.New(authMod, cookie.Options{
		Name:       CookieSession,
		Domain:     SSOCookieDomain,
		Policy:     auth.SessionPolicy{Absolute: SSOSessionAbsolute, Idle: SSOSessionIdle, Assurance: model.AssuranceSingleFactor},
		TrustProxy: true,
	})
	if err != nil {
		return nil, nil, err
	}
	authMod.SetStrategy(sso)

	stepUp, err := cookie.New(authMod, cookie.Options{
		Name:       CookieAdmin,
		Policy:     auth.SessionPolicy{Absolute: AdminSessionAbsolute, Idle: AdminSessionIdle, Assurance: model.AssuranceMultiFactor},
		TrustProxy: true,
	})
	if err != nil {
		return nil, nil, err
	}
	authMod.SetStepUpStrategy(stepUp)

	pk, err := passkey.New(db, ids, authMod, report, LocalPanelOrigin)
	if err != nil {
		return nil, nil, err
	}
	authMod.Enable(pk)

	return authMod, rbacSvc, nil
}

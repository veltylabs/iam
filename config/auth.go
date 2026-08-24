package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/auth/oauth2"
	"github.com/tinywasm/auth/oauth2/provider/google"
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
)

const (
	CookieSession         = "iam_session"
	TTLSession            = 86400 * 7
	EnvGoogleClientID     = "GOOGLE_CLIENT_ID"
	EnvGoogleClientSecret = "GOOGLE_CLIENT_SECRET"
	EnvGoogleRedirectURL  = "GOOGLE_REDIRECT_URL"
)

// NewProductionAuth arma el motor de identidad+RBAC para producción (Google
// OAuth). read resuelve las variables de Google — el servidor local inyecta
// osenv.Reader(), un futuro edge/main.go inyectará un Reader sobre el
// binding de Cloudflare; este paquete nunca importa "os" directamente (ver
// AGENTS.md Restricción #3). Falla rápido si falta cualquier variable:
// arrancar con OAuth roto en silencio es peor que no arrancar. No crea
// roles ni permisos — eso es política de cada app consumidora (ver
// bootstrap.go para el mecanismo genérico de asignación por email).
func NewProductionAuth(db *orm.DB, ids model.IDGenerator, read env.Reader) (*authority.Module, *rbac.Service, error) {
	clientID := read(EnvGoogleClientID)
	if clientID == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientID)
	}
	clientSecret := read(EnvGoogleClientSecret)
	if clientSecret == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientSecret)
	}
	redirectURL := read(EnvGoogleRedirectURL)
	if redirectURL == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleRedirectURL)
	}

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

	g := &google.GoogleProvider{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
	}
	authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{g}))
	return authMod, rbacSvc, nil
}

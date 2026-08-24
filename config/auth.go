package config

import (
	"os"

	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/auth/oauth2"
	"github.com/tinywasm/auth/oauth2/provider/google"
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
// OAuth). Falla rápido si falta cualquier variable: arrancar con OAuth roto
// en silencio es peor que no arrancar. No crea roles ni permisos — eso es
// política de cada app consumidora (ver bootstrap.go para el mecanismo
// genérico de asignación por email).
func NewProductionAuth(db *orm.DB, ids model.IDGenerator) (*authority.Module, *rbac.Service, error) {
	clientID := os.Getenv(EnvGoogleClientID)
	if clientID == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientID)
	}
	clientSecret := os.Getenv(EnvGoogleClientSecret)
	if clientSecret == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientSecret)
	}
	redirectURL := os.Getenv(EnvGoogleRedirectURL)
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

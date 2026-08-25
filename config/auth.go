package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/auth/oauth2"
	"github.com/tinywasm/auth/oauth2/provider/google"
	sessionjwt "github.com/tinywasm/auth/session/jwt"
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
	EnvJWTSecret          = "JWT_SECRET"

	// SSOCookieDomain es el dominio padre bajo el que la cookie de
	// identidad se comparte entre subdominios (ver ARCHITECTURE.md §7):
	// iam.velty.cl, misitio.velty.cl, etc. leen la MISMA sesión.
	SSOCookieDomain = ".velty.cl"
	// SSOSessionTTL: 7 días — ver ARCHITECTURE.md §7.
	SSOSessionTTL = 86400 * 7
)

// NewProductionAuth arma el motor de identidad+RBAC para producción (Google
// OAuth). Lee env directamente via env.Get (auto-tag !wasm=os+.env, wasm=context.env);
// este paquete nunca importa "os" directamente (ver AGENTS.md Restricción #3).
// Falla rápido si falta cualquier variable: arrancar con OAuth roto en silencio
// es peor que no arrancar. No crea roles ni permisos — eso es política de cada
// app consumidora (ver bootstrap.go para el mecanismo genérico de asignación por email).
func NewProductionAuth(db *orm.DB, ids model.IDGenerator) (*authority.Module, *rbac.Service, error) {
	clientID := env.Get(EnvGoogleClientID)
	if clientID == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientID)
	}
	clientSecret := env.Get(EnvGoogleClientSecret)
	if clientSecret == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientSecret)
	}
	redirectURL := env.Get(EnvGoogleRedirectURL)
	if redirectURL == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleRedirectURL)
	}
	jwtSecret := env.Get(EnvJWTSecret)
	if jwtSecret == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvJWTSecret)
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
	// WithRedirectValidator: un consumidor (misitio, futuros proyectos)
	// arranca el login con /oauth/google?redirect_uri=<su URL> para que,
	// tras loguearse en iam, el navegador vuelva a él en vez de quedarse en
	// iam.velty.cl. isVeltyDomain es el único guardián contra un
	// open-redirect — nunca aceptes un dominio fuera de *.velty.cl.
	authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{g},
		oauth2.WithRedirectValidator(isVeltyDomain),
	))

	// Sesión SSO: cookie de identidad compartida entre *.velty.cl (ver
	// ARCHITECTURE.md §7). Mismo secreto que IssueAuthToken (Etapa 3) usa
	// para firmar el token de autorización — un solo secreto HS256 para
	// todo iam, no dos (ver ARCHITECTURE.md §7, "no confundas este secreto
	// con client_secret de un proyecto").
	strategy, err := sessionjwt.New([]byte(jwtSecret), SSOSessionTTL, authMod, authMod)
	if err != nil {
		return nil, nil, err
	}
	strategy.WithDomain(SSOCookieDomain)
	authMod.SetStrategy(strategy)

	return authMod, rbacSvc, nil
}

// isVeltyDomain reports whether url's host is velty.cl or a subdomain of
// it — the same criterion misitio's consumer-facing dominio uses. Used ONLY
// to validate a post-login redirect_uri; never relax this to a broader
// pattern (no "*.example.com" for a third party) without re-reading
// ARCHITECTURE.md §3.2's SSO scope decision.
func isVeltyDomain(url string) bool {
	const prefix = "https://"
	if !fmt.HasPrefix(url, prefix) {
		return false
	}
	host := url[len(prefix):]
	if idx := fmt.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}
	return host == SSOCookieDomain[1:] || fmt.HasSuffix(host, SSOCookieDomain)
}

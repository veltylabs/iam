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

const (
	httpsPrefix = "https://"
	// hostTerminators son los caracteres que terminan el host en una URL tal
	// como la interpreta un navegador. Cortar sólo en "/" — lo que hacía la
	// versión anterior — deja pasar https://evil.com#.velty.cl: el sufijo
	// ".velty.cl" queda dentro del "host" para esta función y fuera de él
	// para el navegador, que va a evil.com. La lista es exhaustiva a propósito:
	// "/" termina el path, "?" la query, "#" el fragmento y "\" lo normaliza
	// el navegador a "/" antes de parsear.
	hostTerminators = "/?#\\"
	// userInfoSep separa userinfo@host. Todo lo que está ANTES es del
	// atacante: https://x.velty.cl@evil.com apunta a evil.com.
	userInfoSep = '@'
)

// isVeltyDomain reports whether url's host is velty.cl or a subdomain of
// it — the same criterion misitio's consumer-facing dominio uses. Used ONLY
// to validate a post-login redirect_uri; never relax this to a broader
// pattern (no "*.example.com" for a third party) without re-reading
// ARCHITECTURE.md §3.2's SSO scope decision.
func isVeltyDomain(url string) bool {
	// Un byte de control embebido cambia cómo parsea el navegador y nunca
	// aparece en una URL legítima.
	for i := 0; i < len(url); i++ {
		if url[i] < 0x20 || url[i] == 0x7F {
			return false
		}
	}
	// Prefijo en minúscula exacta: un redirect_uri legítimo de este
	// ecosistema siempre lo escribe así.
	if !fmt.HasPrefix(url, httpsPrefix) {
		return false
	}
	host := url[len(httpsPrefix):]
	// Cortar en el primer terminador de host.
	end := len(host)
	for i := 0; i < len(host); i++ {
		c := host[i]
		isTerm := false
		for j := 0; j < len(hostTerminators); j++ {
			if c == hostTerminators[j] {
				isTerm = true
				break
			}
		}
		if isTerm {
			end = i
			break
		}
	}
	host = host[:end]
	// Quedarse con lo que está después del último '@'.
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == userInfoSep {
			host = host[i+1:]
			break
		}
	}
	if host == "" {
		return false
	}
	// Los hosts son case-insensitive.
	host = fmt.ToLower(host)
	return host == SSOCookieDomain[1:] || fmt.HasSuffix(host, SSOCookieDomain)
}

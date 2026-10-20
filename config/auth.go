package config

import (
	"unsafe"

	"webtyp.com/auth"
	"webtyp.com/auth/authority"
	"webtyp.com/auth/oauth2"
	"webtyp.com/auth/oauth2/provider/google"
	"webtyp.com/auth/passkey"
	"webtyp.com/auth/session/cookie"
	"webtyp.com/env"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/rbac"
	"webtyp.com/webauthn/server"
)

const (
	CookieSession = "iam_session"
	CookieAdmin   = "iam_admin"

	EnvGoogleClientID    = "GOOGLE_CLIENT_ID"
	EnvGoogleSecret      = "GOOGLE_CLIENT_SECRET"
	EnvGoogleRedirectURL = "GOOGLE_REDIRECT_URL"
	EnvJWTSecret         = "JWT_SECRET"

	// SSOCookieDomain es el dominio padre bajo el que la cookie de
	// identidad se comparte entre subdominios (ver ARCHITECTURE.md §7):
	// iam.velty.cl, misitio.velty.cl, etc. leen la MISMA sesión.
	SSOCookieDomain = ".velty.cl"

	SSOSessionAbsolute   = 12 * 3600
	SSOSessionIdle       = 30 * 60
	AdminSessionAbsolute = 8 * 3600
	AdminSessionIdle     = 15 * 60
)

// OriginHost extracts the host from origin (stripping scheme, port, trailing slash).
func OriginHost(origin string) string {
	if fmt.HasPrefix(origin, "https://") {
		origin = origin[8:]
	} else if fmt.HasPrefix(origin, "http://") {
		origin = origin[7:]
	}
	for i := 0; i < len(origin); i++ {
		if origin[i] == ':' || origin[i] == '/' {
			return origin[:i]
		}
	}
	return origin
}

func originHost(origin string) string {
	return OriginHost(origin)
}

// NewProductionAuth arma el motor de identidad+RBAC para producción (Google OAuth).
func NewProductionAuth(db *orm.DB, ids model.IDGenerator) (*authority.Module, *rbac.Service, error) {
	clientID := env.Get(EnvGoogleClientID)
	if clientID == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientID)
	}
	sec := env.Get(EnvGoogleSecret)
	if sec == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleSecret)
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
		TrustProxy: true,
	})
	if err != nil {
		return nil, nil, err
	}

	rbacSvc, err := rbac.New(db)
	if err != nil {
		return nil, nil, err
	}

	type googleProv struct {
		ClientID string
		Secret   string
		Redirect string
	}
	gp := googleProv{ClientID: clientID, Secret: sec, Redirect: redirectURL}
	g := (*google.GoogleProvider)(unsafe.Pointer(&gp))
	authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{g},
		oauth2.WithRedirectValidator(isVeltyDomain),
	))

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

	origin := env.Get(EnvPanelOrigin)
	if origin != "" {
		rp := server.RelyingParty{ID: OriginHost(origin), Origin: origin}
		pk, err := passkey.New(db, ids, authMod, rp)
		if err != nil {
			return nil, nil, err
		}
		authMod.Enable(pk)
	}

	return authMod, rbacSvc, nil
}

const (
	httpsPrefix = "https://"
	hostTerminators = "/?#\\"
	userInfoSep = '@'
)

// isVeltyDomain reports whether url's host is velty.cl or a subdomain of it.
func isVeltyDomain(url string) bool {
	for i := 0; i < len(url); i++ {
		if url[i] < 0x20 || url[i] == 0x7F {
			return false
		}
	}
	if !fmt.HasPrefix(url, httpsPrefix) {
		return false
	}
	host := url[len(httpsPrefix):]
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
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == userInfoSep {
			host = host[i+1:]
			break
		}
	}
	if host == "" {
		return false
	}
	host = fmt.ToLower(host)
	return host == SSOCookieDomain[1:] || fmt.HasSuffix(host, SSOCookieDomain)
}

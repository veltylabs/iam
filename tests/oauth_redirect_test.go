package tests

import (
	"testing"

	"github.com/tinywasm/router/mock"
)

// forwardOAuthCookies pasa al callback las cookies que /oauth/google emitió:
// oauth_redirect (el destino validado) y oauth_nonce (el state+nonce que el
// callback exige desde tinywasm/auth v0.0.23).
func forwardOAuthCookies(t *testing.T, from, to *mock.Context) {
	t.Helper()
	for _, name := range []string{"oauth_redirect", "oauth_nonce"} {
		if c, ok := from.Cookie(name); ok {
			to.SetCookie(c)
		}
	}
}

// startLogin arranca el flujo OAuth con el redirect_uri dado y devuelve el
// contexto del start (con sus cookies y su Location).
func startLogin(t *testing.T, redirectURI string) *mock.Context {
	t.Helper()
	_, authMod, _, _ := setupLocal(t) // mock OAuth provider: no network calls

	r := &mock.Router{}
	authMod.MountAPI(r)

	start := &mock.Context{InPath: "/oauth/google?redirect_uri=" + redirectURI}
	r.Invoke("GET", "/oauth/google", start)
	return start
}

// TestOAuthRedirect_ToAllowedConsumer proves iam's OAuth wiring
// returns the browser to a *.velty.cl consumer that started the login with
// ?redirect_uri=, and TestOAuthRedirect_RejectsForeignDomain proves a
// domain outside *.velty.cl never becomes the redirect target (the
// open-redirect guard — see config/auth.go's isVeltyDomain).
func TestOAuthRedirect_ToAllowedConsumer(t *testing.T) {
	_, authMod, _, _ := setupLocal(t) // mock OAuth provider: no network calls

	r := &mock.Router{}
	authMod.MountAPI(r)

	start := &mock.Context{InPath: "/oauth/google?redirect_uri=https://misitio.velty.cl/panel"}
	r.Invoke("GET", "/oauth/google", start)
	redirectCookie, ok := start.Cookie("oauth_redirect")
	if !ok || redirectCookie.Value != "https://misitio.velty.cl/panel" {
		t.Fatalf("oauth_redirect cookie: got %+v, ok=%v", redirectCookie, ok)
	}

	callback := &mock.Context{InPath: start.GetHeader("Location")}
	forwardOAuthCookies(t, start, callback)
	r.Invoke("GET", "/oauth/callback/google", callback)

	if got := callback.GetHeader("Location"); got != "https://misitio.velty.cl/panel" {
		t.Errorf("Location: got %q, want the validated redirect_uri", got)
	}
}

func TestOAuthRedirect_RejectsForeignDomain(t *testing.T) {
	_, authMod, _, _ := setupLocal(t)

	r := &mock.Router{}
	authMod.MountAPI(r)

	start := &mock.Context{InPath: "/oauth/google?redirect_uri=https://evil.example.com/steal"}
	r.Invoke("GET", "/oauth/google", start)
	if _, ok := start.Cookie("oauth_redirect"); ok {
		t.Fatal("oauth_redirect cookie set for a domain outside *.velty.cl")
	}
}

// TestIsVeltyDomain_RejectsFragmentBypass: el navegador termina el host en
// '#', así que https://evil.com#.velty.cl va a evil.com — el guardián tiene
// que ver lo mismo que el navegador.
func TestIsVeltyDomain_RejectsFragmentBypass(t *testing.T) {
	start := startLogin(t, "https://evil.com#.velty.cl")
	if _, ok := start.Cookie("oauth_redirect"); ok {
		t.Fatal("oauth_redirect cookie set for fragment bypass https://evil.com#.velty.cl")
	}
}

// TestIsVeltyDomain_RejectsBackslashBypass: el navegador normaliza '\' a '/'
// antes de parsear, así que https://evil.com\.velty.cl va a evil.com.
func TestIsVeltyDomain_RejectsBackslashBypass(t *testing.T) {
	start := startLogin(t, `https://evil.com\.velty.cl`)
	if _, ok := start.Cookie("oauth_redirect"); ok {
		t.Fatal(`oauth_redirect cookie set for backslash bypass https://evil.com\.velty.cl`)
	}
}

// TestIsVeltyDomain_RejectsQueryBypass: '?' también termina el host para el
// navegador.
func TestIsVeltyDomain_RejectsQueryBypass(t *testing.T) {
	start := startLogin(t, "https://evil.com?x.velty.cl")
	if _, ok := start.Cookie("oauth_redirect"); ok {
		t.Fatal("oauth_redirect cookie set for query bypass https://evil.com?x.velty.cl")
	}
}

// TestIsVeltyDomain_RejectsUserInfoBypass: todo lo anterior a '@' es
// userinfo del atacante — https://x.velty.cl@evil.com apunta a evil.com.
func TestIsVeltyDomain_RejectsUserInfoBypass(t *testing.T) {
	start := startLogin(t, "https://x.velty.cl@evil.com")
	if _, ok := start.Cookie("oauth_redirect"); ok {
		t.Fatal("oauth_redirect cookie set for userinfo bypass https://x.velty.cl@evil.com")
	}
}

// TestIsVeltyDomain_RejectsControlChars: un \n embebido cambia cómo parsea
// el navegador y nunca aparece en una URL legítima.
func TestIsVeltyDomain_RejectsControlChars(t *testing.T) {
	start := startLogin(t, "https://evil.com\n.velty.cl")
	if _, ok := start.Cookie("oauth_redirect"); ok {
		t.Fatal("oauth_redirect cookie set for control-char bypass")
	}
}

// TestIsVeltyDomain_RejectsSuffixLookalike: ni el dominio pelado ajeno ni un
// subdominio de un dominio ajeno pasan.
func TestIsVeltyDomain_RejectsSuffixLookalike(t *testing.T) {
	for _, uri := range []string{"https://notvelty.cl", "https://velty.cl.evil.com"} {
		start := startLogin(t, uri)
		if _, ok := start.Cookie("oauth_redirect"); ok {
			t.Fatalf("oauth_redirect cookie set for lookalike %s", uri)
		}
	}
}

// TestIsVeltyDomain_AcceptsRealConsumers: el dominio apex, un subdominio con
// path y un subdominio anidado siguen pasando.
func TestIsVeltyDomain_AcceptsRealConsumers(t *testing.T) {
	for _, uri := range []string{"https://velty.cl", "https://misitio.velty.cl/panel", "https://a.b.velty.cl"} {
		start := startLogin(t, uri)
		c, ok := start.Cookie("oauth_redirect")
		if !ok || c.Value != uri {
			t.Fatalf("oauth_redirect cookie: got %+v, ok=%v for legitimate %s", c, ok, uri)
		}
	}
}

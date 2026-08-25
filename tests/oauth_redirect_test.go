package tests

import (
	"testing"

	"github.com/tinywasm/router/mock"
)

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
	callback.SetCookie(redirectCookie)
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

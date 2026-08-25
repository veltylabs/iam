package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	tinyjwt "github.com/tinywasm/jwt"
	"github.com/veltylabs/iam/client"
)

func signTestClaims() (string, error) {
	claims := tinyjwt.NewScopedClaims("user-1", "misitio", []string{"editor"}, 300)
	return tinyjwt.Sign([]byte("test-secret-32-bytes-long-0000000"), claims)
}

// TestFetchAuthzToken_ForwardsCookieAndReturnsClaims proves the
// server-to-server contract end to end against a real HTTP server (not a
// mock): the SSO cookie value is forwarded as a Cookie header, project_id
// and client_secret travel in the body, and a 200 response's token comes
// back decoded into Claims.
func TestFetchAuthzToken_ForwardsCookieAndReturnsClaims(t *testing.T) {
	var gotCookie, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// A token with Sub/Aud/Scope — this test only cares that
		// DecodeUnverified round-trips what iam sent, not that it re-signs it.
		w.Write([]byte(`{"token":"` + testToken(t) + `","email":"u@test.com","name":"U Test","avatar":"https://x/a.png"}`))
	}))
	defer srv.Close()

	id, err := client.FetchAuthzToken(srv.URL, "misitio", "the-secret", "cookie-value-123")
	if err != nil {
		t.Fatalf("FetchAuthzToken: %v", err)
	}
	if gotCookie != client.SSOCookieName+"=cookie-value-123" {
		t.Errorf("Cookie header: got %q", gotCookie)
	}
	if !containsAll(gotBody, `"project_id":"misitio"`, `"client_secret":"the-secret"`) {
		t.Errorf("body did not carry project_id/client_secret: %q", gotBody)
	}
	if id.Claims.Sub != "user-1" || id.Claims.Aud != "misitio" {
		t.Errorf("claims: got %+v", id.Claims)
	}
	if len(id.Claims.Scope) != 1 || id.Claims.Scope[0] != "editor" {
		t.Errorf("scope: got %+v", id.Claims.Scope)
	}
	if id.Email != "u@test.com" || id.Name != "U Test" || id.Avatar != "https://x/a.png" {
		t.Errorf("profile: got %+v", id)
	}
}

// TestFetchAuthzToken_NoCookie proves a caller that forwards no SSO cookie
// value never makes the network call — there is nothing to ask iam.
func TestFetchAuthzToken_NoCookie(t *testing.T) {
	_, err := client.FetchAuthzToken("http://localhost:1", "misitio", "secret", "")
	if err != client.ErrNoSSOSession {
		t.Errorf("got %v, want ErrNoSSOSession", err)
	}
}

// TestFetchAuthzToken_Unauthorized proves iam's 401/403 collapse to the
// SAME ErrNoSSOSession as no-cookie-at-all — an expired/forged SSO session
// is "treat as anonymous", not a distinguishable error a caller needs to
// branch on differently.
func TestFetchAuthzToken_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	_, err := client.FetchAuthzToken(srv.URL, "misitio", "secret", "expired-cookie")
	if err != client.ErrNoSSOSession {
		t.Errorf("got %v, want ErrNoSSOSession", err)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// testToken signs a real token with tinywasm/jwt so this test exercises
// the exact wire shape client.FetchAuthzToken decodes.
func testToken(t *testing.T) string {
	t.Helper()
	tok, err := signTestClaims()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

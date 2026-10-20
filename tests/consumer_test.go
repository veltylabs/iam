//go:build !wasm

package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"webtyp.com/crypto/asym"
	tinyjwt "webtyp.com/jwt"
	"webtyp.com/router"
	"webtyp.com/router/mock"
	"webtyp.com/time"
	"github.com/veltylabs/iam/client"
)

func testKeys(t *testing.T) ([]byte, []byte) {
	t.Helper()
	pub, priv, err := asym.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return pub, priv
}

func TestNewRejectsEmptyConfig(t *testing.T) {
	_, priv := testKeys(t)
	if _, err := client.New(client.Config{BaseURL: "", ProjectID: "p", PrivateKey: priv}); err == nil || err.Error() != client.ErrMsgMissingBaseURL {
		t.Errorf("missing BaseURL: got %v, want %q", err, client.ErrMsgMissingBaseURL)
	}
	if _, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "", PrivateKey: priv}); err == nil || err.Error() != client.ErrMsgMissingProjectID {
		t.Errorf("missing ProjectID: got %v, want %q", err, client.ErrMsgMissingProjectID)
	}
	if _, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "p", PrivateKey: nil}); err == nil || err.Error() != client.ErrMsgMissingPrivateKey {
		t.Errorf("missing PrivateKey: got %v, want %q", err, client.ErrMsgMissingPrivateKey)
	}
}

func TestAuthnAnonymousWithoutCookie(t *testing.T) {
	_, priv := testKeys(t)
	consumer, err := client.New(client.Config{BaseURL: "http://localhost:1", ProjectID: "proj-1", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := func(ctx router.Context) {
		called = true
		if ctx.UserID() != "" {
			t.Errorf("userID: got %q, want empty", ctx.UserID())
		}
		if _, ok := client.FromContext(ctx); ok {
			t.Error("expected no identity in context")
		}
	}
	handler := consumer.Authn()(next)
	ctx := &mock.Context{}
	handler(ctx)
	if !called {
		t.Error("next handler not called")
	}
}

func TestAuthnAnonymousWhenIAMRejects(t *testing.T) {
	_, priv := testKeys(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "proj-1", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	next := func(ctx router.Context) {
		called = true
		if ctx.UserID() != "" {
			t.Errorf("userID: got %q, want empty (IAM rejected)", ctx.UserID())
		}
	}
	handler := consumer.Authn()(next)
	ctx := &mock.Context{}
	ctx.SetCookie(router.Cookie{Name: client.SSOCookieName, Value: "bad-cookie"})
	handler(ctx)
	if !called {
		t.Error("next handler not called when IAM rejects")
	}
}

func TestIdentityRoundTrip(t *testing.T) {
	ctx := &mock.Context{}
	id := client.Identity{
		Claims: tinyjwt.Claims{
			Sub:   "user-123",
			Exp:   9999999999,
			Iat:   1000,
			Aud:   "proj-1",
			Scope: []string{"editor", "admin"},
		},
		Email:  "a@test.com",
		Name:   "Alice",
		Avatar: "https://x/avatar.png",
	}
	client.SetIdentity(ctx, id)
	got, ok := client.FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned not ok")
	}
	if got.Claims.Sub != id.Claims.Sub || got.Claims.Exp != id.Claims.Exp || got.Claims.Iat != id.Claims.Iat || got.Claims.Aud != id.Claims.Aud {
		t.Errorf("claims mismatch: got %+v want %+v", got.Claims, id.Claims)
	}
	if len(got.Claims.Scope) != 2 || got.Claims.Scope[0] != "editor" || got.Claims.Scope[1] != "admin" {
		t.Errorf("scope mismatch: got %+v", got.Claims.Scope)
	}
	if got.Email != id.Email || got.Name != id.Name || got.Avatar != id.Avatar {
		t.Errorf("profile mismatch: got %+v want %+v", got, id)
	}
}

func TestFromContextWithoutIdentity(t *testing.T) {
	ctx := &mock.Context{}
	if _, ok := client.FromContext(ctx); ok {
		t.Error("expected ok=false for clean context")
	}
}

func TestScopeDeniesWhenAbsent(t *testing.T) {
	_, priv := testKeys(t)
	consumer, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "p", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := consumer.Scope("unknown-user"); ok {
		t.Error("expected ok=false for unknown user")
	}
}

func TestScopeDeniesWhenExpired(t *testing.T) {
	pastExp := time.Now()/1e9 - 1000
	claims := tinyjwt.Claims{
		Sub:   "user-exp",
		Exp:   pastExp,
		Iat:   pastExp - 100,
		Aud:   "proj-1",
		Scope: []string{"editor"},
	}
	token, err := tinyjwt.Sign([]byte("test-secret-32-bytes-long-0000000"), claims)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"token":"` + token + `","email":"e@test.com","name":"E","avatar":""}`))
	}))
	defer srv.Close()

	_, priv := testKeys(t)
	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "proj-1", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &mock.Context{}
	ctx.SetCookie(router.Cookie{Name: client.SSOCookieName, Value: "cookie-val"})
	handler := consumer.Authn()(func(ctx router.Context) {})
	handler(ctx)

	if _, ok := consumer.Scope("user-exp"); ok {
		t.Error("expected Scope to deny expired entry, got ok=true")
	}
	_ = io.Discard
}

func TestScopeOverwritesSameUser(t *testing.T) {
	makeToken := func(scope []string) string {
		claims := tinyjwt.Claims{
			Sub:   "user-over",
			Exp:   time.Now()/1e9 + 3600,
			Iat:   time.Now() / 1e9,
			Aud:   "proj-1",
			Scope: scope,
		}
		tok, err := tinyjwt.Sign([]byte("test-secret-32-bytes-long-0000000"), claims)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return tok
	}
	tokenA := makeToken([]string{"a"})
	tokenB := makeToken([]string{"b"})

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		tok := tokenA
		if call == 2 {
			tok = tokenB
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"token":"` + tok + `","email":"e@test.com","name":"E","avatar":""}`))
	}))
	defer srv.Close()

	_, priv := testKeys(t)
	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "proj-1", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		ctx := &mock.Context{}
		ctx.SetCookie(router.Cookie{Name: client.SSOCookieName, Value: "cookie-val"})
		handler := consumer.Authn()(func(ctx router.Context) {})
		handler(ctx)
	}

	scope, ok := consumer.Scope("user-over")
	if !ok {
		t.Fatal("expected ok=true after two Sets")
	}
	if len(scope) != 1 || scope[0] != "b" {
		t.Errorf("scope after overwrite: got %+v, want [b]", scope)
	}
}

func TestAssignRoleRejectsEmptyArgs(t *testing.T) {
	_, priv := testKeys(t)
	consumer, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "p", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.AssignRole("", "editor"); err == nil || err.Error() != client.ErrMsgAssignRoleUserRequired {
		t.Errorf("empty userID: got %v, want %q", err, client.ErrMsgAssignRoleUserRequired)
	}
	if err := consumer.AssignRole("user-1", ""); err == nil || err.Error() != client.ErrMsgAssignRoleCodeRequired {
		t.Errorf("empty roleCode: got %v, want %q", err, client.ErrMsgAssignRoleCodeRequired)
	}
}

func TestResolveUserViaConsumer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr := r.Header.Get("Authorization")
		if authHdr == "" {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"sub":"resolved-user-id"}`))
	}))
	defer srv.Close()

	_, priv := testKeys(t)
	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "p1", PrivateKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := consumer.ResolveUser("test@example.com", "Test")
	if err != nil {
		t.Fatalf("ResolveUser: %v", err)
	}
	if sub != "resolved-user-id" {
		t.Errorf("got sub %q, want %q", sub, "resolved-user-id")
	}
}

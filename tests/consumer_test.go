package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tinywasm/json"
	tinyjwt "github.com/tinywasm/jwt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/router"
	"github.com/tinywasm/router/mock"
	"github.com/tinywasm/time"
	"github.com/veltylabs/iam/client"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

func TestNewRejectsEmptyConfig(t *testing.T) {
	if _, err := client.New(client.Config{BaseURL: "", ProjectID: "p", ClientSecret: "s"}); err == nil || err.Error() != client.ErrMsgMissingBaseURL {
		t.Errorf("missing BaseURL: got %v, want %q", err, client.ErrMsgMissingBaseURL)
	}
	if _, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "", ClientSecret: "s"}); err == nil || err.Error() != client.ErrMsgMissingProjectID {
		t.Errorf("missing ProjectID: got %v, want %q", err, client.ErrMsgMissingProjectID)
	}
	if _, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "p", ClientSecret: ""}); err == nil || err.Error() != client.ErrMsgMissingClientSecret {
		t.Errorf("missing ClientSecret: got %v, want %q", err, client.ErrMsgMissingClientSecret)
	}
}

func TestAuthnAnonymousWithoutCookie(t *testing.T) {
	consumer, err := client.New(client.Config{BaseURL: "http://localhost:1", ProjectID: "proj-1", ClientSecret: "secret"})
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "proj-1", ClientSecret: "secret"})
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
	consumer, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "p", ClientSecret: "s"})
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

	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "proj-1", ClientSecret: "secret"})
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

	consumer, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: "proj-1", ClientSecret: "secret"})
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
	consumer, err := client.New(client.Config{BaseURL: "http://x", ProjectID: "p", ClientSecret: "s"})
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

func TestAssignRoleIsIdempotent(t *testing.T) {
	backend := setupBackend(t)
	const projectID = "proj-assign-1"
	if err := config.CreateProject(backend.DB, projectID, "Proj Assign", "proj-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	u, err := backend.Auth.CreateUser("assign@test.com", "Assign", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	handler := routes.AssignRole(backend.DB, backend.RBAC)

	call := func(userID, roleCode string) int {
		ctx := &mock.Context{}
		var out []byte
		if err := json.Encode(&routes.AssignRoleRequest{
			ProjectID: projectID, ClientSecret: "proj-secret", UserID: userID, RoleCode: roleCode,
		}, &out); err != nil {
			t.Fatalf("encode: %v", err)
		}
		ctx.InBody = out
		handler(ctx)
		return ctx.Status
	}

	if status := call(u.Id, "editor"); status != 200 {
		t.Fatalf("first assign: got status %d, want 200", status)
	}
	if status := call(u.Id, "editor"); status != 200 {
		t.Fatalf("second assign (idempotent): got status %d, want 200", status)
	}
	roles, err := backend.RBAC.GetUserRoles(projectID, u.Id)
	if err != nil {
		t.Fatalf("GetUserRoles: %v", err)
	}
	found := false
	for _, r := range roles {
		if r.Code == "editor" {
			found = true
		}
	}
	if !found {
		t.Errorf("role editor not found after assign: got %+v", roles)
	}
}

func TestAssignRoleIsScopedToTheProject(t *testing.T) {
	backend := setupBackend(t)
	const projectID = "proj-scoped"
	const otherProjectID = "proj-other"
	if err := config.CreateProject(backend.DB, projectID, "Proj Scoped", "secret-scoped"); err != nil {
		t.Fatalf("CreateProject scoped: %v", err)
	}
	if err := config.CreateProject(backend.DB, otherProjectID, "Proj Other", "secret-other"); err != nil {
		t.Fatalf("CreateProject other: %v", err)
	}
	u, err := backend.Auth.CreateUser("scoped@test.com", "Scoped", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	consumer2, err := client.New(client.Config{BaseURL: srv.URL, ProjectID: projectID, ClientSecret: "secret-scoped"})
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer2.AssignRole(u.Id, "editor"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if !contains(gotBody, `"project_id":"`+projectID+`"`) {
		t.Errorf("body should contain project_id %q, got %q", projectID, gotBody)
	}
	if contains(gotBody, otherProjectID) {
		t.Errorf("body should not contain other project %q, got %q", otherProjectID, gotBody)
	}
	handler := routes.AssignRole(backend.DB, backend.RBAC)
	ctx := &mock.Context{}
	var out []byte
	if err := json.Encode(&routes.AssignRoleRequest{
		ProjectID: projectID, ClientSecret: "secret-scoped", UserID: u.Id, RoleCode: "editor",
	}, &out); err != nil {
		t.Fatal(err)
	}
	ctx.InBody = out
	handler(ctx)
	if ctx.Status != 200 {
		t.Fatalf("handler assign: got %d, want 200", ctx.Status)
	}
	rolesOther, err := backend.RBAC.GetUserRoles(otherProjectID, u.Id)
	if err != nil {
		t.Fatalf("GetUserRoles other: %v", err)
	}
	if len(rolesOther) != 0 {
		t.Errorf("other project should have no roles, got %+v", rolesOther)
	}
	_ = model.RoleCode("")
}

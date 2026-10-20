//go:build !wasm

package tests

import (
	"testing"

	"webtyp.com/json"
	tinyjwt "webtyp.com/jwt"
	"webtyp.com/router/mock"
	"github.com/veltylabs/iam/config"
)

// TestIssueAuthToken_UsesMostRestrictiveRoleTTL proves the differentiated
// TTL per role (ARCHITECTURE.md §6.3): a user with a role carrying its own
// SessionTTL gets THAT ttl embedded in the token, not DefaultAuthTokenTTL.
func TestIssueAuthToken_UsesMostRestrictiveRoleTTL(t *testing.T) {
	backend := setupBackend(t)

	const projectID = "proj-1"
	if err := backend.RBAC.CreateRole(projectID, "r_short", "short", "Short-lived", ""); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := backend.RBAC.SetRoleSessionTTL(projectID, "r_short", 300); err != nil {
		t.Fatalf("SetRoleSessionTTL: %v", err)
	}
	if err := backend.RBAC.AssignRole(projectID, "user-1", "r_short"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	token, err := config.IssueAuthToken(backend.RBAC, backend.JWTSecret, projectID, "user-1")
	if err != nil {
		t.Fatalf("IssueAuthToken: %v", err)
	}
	claims, outcome, err := tinyjwt.Verify(backend.JWTSecret, token)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != tinyjwt.Valid {
		t.Fatalf("outcome: got %v, want valid", outcome)
	}
	if got := claims.Exp - claims.Iat; got != 300 {
		t.Errorf("ttl: got %d, want 300 (the role's SessionTTL, not DefaultAuthTokenTTL=%d)", got, config.DefaultAuthTokenTTL)
	}
	if claims.Aud != projectID {
		t.Errorf("aud: got %q, want %q", claims.Aud, projectID)
	}
	if len(claims.Scope) != 1 || claims.Scope[0] != "short" {
		t.Errorf("scope: got %+v, want [short]", claims.Scope)
	}
}

func TestToken_ReturnsUserProfileAndClaims(t *testing.T) {
	backend := setupBackend(t)
	const projectID = "proj-1"
	if err := config.CreateProject(backend.DB, projectID, "Proj One", ""); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	u, err := backend.Auth.CreateUser("session@test.com", "Session User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	mockCtx := &mock.Context{}
	if err := backend.Auth.IssueSession(mockCtx, u.Id); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	ssoCookie, ok := mockCtx.Cookie(config.CookieSession)
	if !ok {
		t.Fatal("cookie not set")
	}

	handler := config.Token(backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret, backend.IDs)
	ctx := &mock.Context{}
	ctx.SetUserID(projectID)

	var body []byte
	_ = json.Encode(&config.TokenRequest{SubjectToken: ssoCookie.Value}, &body)
	ctx.InBody = body

	handler(ctx)

	if ctx.Status != 200 {
		t.Fatalf("status: got %d, want 200", ctx.Status)
	}
	var resp config.TokenResponse
	if err := json.Decode(ctx.ResponseBody(), &resp); err != nil {
		t.Fatal(err)
	}
	claims, outcome, err := tinyjwt.Verify(backend.JWTSecret, resp.Token)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != tinyjwt.Valid {
		t.Fatalf("outcome: got %v, want valid", outcome)
	}
	if claims.Sub != u.Id {
		t.Errorf("sub: got %q, want %q (the session's identity)", claims.Sub, u.Id)
	}
	if resp.Email != "session@test.com" || resp.Name != "Session User" {
		t.Errorf("profile: got email=%q name=%q", resp.Email, resp.Name)
	}
}

// TestResolveUser_CreatesThenReusesSameSub proves ResolveUser is
// idempotent by email.
func TestResolveUser_CreatesThenReusesSameSub(t *testing.T) {
	backend := setupBackend(t)
	const projectID = "proj-1"
	if err := config.CreateProject(backend.DB, projectID, "Proj One", ""); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	handler := config.ResolveUser(backend.DB, backend.Auth, backend.IDs)

	call := func() (int, config.ResolveUserResponse) {
		ctx := &mock.Context{}
		ctx.SetUserID(projectID)
		var body []byte
		if err := json.Encode(&config.ResolveUserRequest{
			Email: "client@test.com", Name: "Client",
		}, &body); err != nil {
			t.Fatal(err)
		}
		ctx.InBody = body
		handler(ctx)
		var resp config.ResolveUserResponse
		_ = json.Decode(ctx.ResponseBody(), &resp)
		return ctx.Status, resp
	}

	status1, resp1 := call()
	if status1 != 200 || resp1.Sub == "" {
		t.Fatalf("first call: status=%d sub=%q", status1, resp1.Sub)
	}
	status2, resp2 := call()
	if status2 != 200 || resp2.Sub != resp1.Sub {
		t.Fatalf("second call: status=%d sub=%q, want same sub as first (%q)", status2, resp2.Sub, resp1.Sub)
	}
}

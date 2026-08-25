package tests

import (
	"testing"

	"github.com/tinywasm/json"
	tinyjwt "github.com/tinywasm/jwt"
	"github.com/tinywasm/router/mock"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
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

// TestToken_WrongClientSecret403 y TestToken_UserIDFromSessionNeverFromBody
// exercise POST /api/token's handler directly via router/mock — the same
// mechanism tinywasm/auth's own tests use to drive a router.HandlerFunc
// without a live server.
func TestToken_WrongClientSecret403(t *testing.T) {
	backend := setupBackend(t)
	if err := config.CreateProject(backend.DB, "proj-1", "Proj One", "correct-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	handler := routes.Token(backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret)
	ctx := &mock.Context{}
	ctx.SetUserID("user-1")
	body, err := encodeTokenRequest(t, "proj-1", "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx.InBody = body

	handler(ctx)

	if ctx.Status != 403 {
		t.Errorf("status: got %d, want 403", ctx.Status)
	}
}

func TestToken_UserIDFromSessionNeverFromBody(t *testing.T) {
	backend := setupBackend(t)
	const projectID = "proj-1"
	if err := config.CreateProject(backend.DB, projectID, "Proj One", "correct-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	u, err := backend.Auth.CreateUser("session@test.com", "Session User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	handler := routes.Token(backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret)
	ctx := &mock.Context{}
	ctx.SetUserID(u.Id)
	// TokenRequest has no user id field at all — there is no way to smuggle
	// one through the body even if an attacker tries to; this proves the
	// emitted token's Sub is the SESSION's identity regardless.
	body, err := encodeTokenRequest(t, projectID, "correct-secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx.InBody = body

	handler(ctx)

	if ctx.Status != 200 {
		t.Fatalf("status: got %d, want 200", ctx.Status)
	}
	var resp routes.TokenResponse
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

func encodeTokenRequest(t *testing.T, projectID, clientSecret string) ([]byte, error) {
	t.Helper()
	req := &routes.TokenRequest{ProjectID: projectID, ClientSecret: clientSecret}
	var out []byte
	if err := json.Encode(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

package tests

import (
	"testing"

	"github.com/tinywasm/router/mock"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

// TestToken_CORSReflectsAllowedOrigin y TestToken_CORSRejectsDisallowedOrigin
// prove ARCHITECTURE.md §7's CORS rule: an Origin under *.velty.cl gets
// reflected back (never "*" — CORS with credentials forbids the wildcard),
// and an Origin outside that domain gets no header at all.
func TestToken_CORSReflectsAllowedOrigin(t *testing.T) {
	backend := setupBackend(t)
	if err := config.CreateProject(backend.DB, "proj-1", "Proj One", "correct-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	handler := routes.TokenHandler(backend.DB, backend.RBAC, backend.JWTSecret)
	ctx := &mock.Context{}
	ctx.SetUserID("user-1")
	ctx.SetHeader("Origin", "https://misitio.velty.cl")
	body, err := encodeTokenRequest(t, "proj-1", "correct-secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx.InBody = body

	handler(ctx)

	if got := ctx.GetHeader("Access-Control-Allow-Origin"); got != "https://misitio.velty.cl" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want %q", got, "https://misitio.velty.cl")
	}
	if got := ctx.GetHeader("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials: got %q, want %q", got, "true")
	}
}

func TestToken_CORSRejectsDisallowedOrigin(t *testing.T) {
	backend := setupBackend(t)
	if err := config.CreateProject(backend.DB, "proj-1", "Proj One", "correct-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	handler := routes.TokenHandler(backend.DB, backend.RBAC, backend.JWTSecret)
	ctx := &mock.Context{}
	ctx.SetUserID("user-1")
	ctx.SetHeader("Origin", "https://evilvelty.cl")
	body, err := encodeTokenRequest(t, "proj-1", "correct-secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx.InBody = body

	handler(ctx)

	if got := ctx.GetHeader("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want empty (disallowed origin)", got)
	}
}

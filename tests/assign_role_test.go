package tests

import (
	"testing"

	"github.com/tinywasm/json"
	"github.com/tinywasm/router/mock"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

// TestAssignRoleUnknownCode404: /api/roles/assign ya no crea roles. Un
// proyecto consumidor no define roles, los usa — definirlos es del panel —
// así que un roleCode inexistente devuelve 404.
func TestAssignRoleUnknownCode404(t *testing.T) {
	backend := setupBackend(t)
	const projectID = "proj-assign-404"
	if err := config.CreateProject(backend.DB, projectID, "Proj Assign 404", "secret-404"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	u, err := backend.Auth.CreateUser("unknown404@test.com", "Unknown404", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	handler := routes.AssignRole(backend.DB, backend.RBAC)
	call := func(roleCode string) int {
		ctx := &mock.Context{}
		var out []byte
		if err := json.Encode(&routes.AssignRoleRequest{
			ProjectID: projectID, ClientSecret: "secret-404", UserID: u.Id, RoleCode: roleCode,
		}, &out); err != nil {
			t.Fatalf("encode: %v", err)
		}
		ctx.InBody = out
		handler(ctx)
		return ctx.Status
	}

	if status := call("no-such-role"); status != 404 {
		t.Fatalf("assign unknown role code: got status %d, want 404", status)
	}

	roles, err := backend.RBAC.GetUserRoles(projectID, u.Id)
	if err != nil {
		t.Fatalf("GetUserRoles: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("failed assign created roles: %+v", roles)
	}
}

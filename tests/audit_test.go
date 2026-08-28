package tests

import (
	"testing"

	"github.com/tinywasm/json"
	"github.com/tinywasm/router/mock"

	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/admin"
)

func TestAudit_RecordsEveryMutation(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	adminU, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	// 1. Create Project
	hCreateProj := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.CreateProjectHandler(b.DB, b.IDs))
	ctx1 := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjects}
	ctx1.SetUserID(adminU.Id)
	ctx1.InBody = encodeBody(t, &config.AdminCreateProjectRequest{Name: "Audit Proj"})
	hCreateProj(ctx1)

	var projResp config.AdminSecretResponse
	_ = json.Decode(ctx1.ResponseBody(), &projResp)
	pID := projResp.Id

	// 2. Rotate Secret
	hRotate := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.RotateSecretHandler(b.DB, b.IDs))
	ctx2 := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjectRotate}
	ctx2.SetUserID(adminU.Id)
	ctx2.InBody = encodeBody(t, &config.AdminProjectIDRequest{ProjectId: pID})
	hRotate(ctx2)

	// 3. Create Role
	hCreateRole := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.CreateRoleHandler(b.DB, b.RBAC, b.IDs))
	ctx3 := &mock.Context{InMethod: "POST", InPath: config.PathAdminRoles}
	ctx3.SetUserID(adminU.Id)
	ctx3.InBody = encodeBody(t, &config.AdminCreateRoleRequest{ProjectId: pID, Code: "auditor", Name: "Auditor", Description: ""})
	hCreateRole(ctx3)

	// Fetch audit entries
	hAudit := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.ListAuditHandler(b.DB))
	ctxAudit := &mock.Context{InMethod: "GET", InPath: config.PathAdminAudit}
	ctxAudit.SetUserID(adminU.Id)
	hAudit(ctxAudit)

	var auditResp config.AdminAuditResponse
	_ = json.Decode(ctxAudit.ResponseBody(), &auditResp)

	if len(auditResp.Entries) < 3 {
		t.Fatalf("expected at least 3 audit entries, got %d", len(auditResp.Entries))
	}

	for _, entry := range auditResp.Entries {
		if entry.ActorEmail != "admin@example.com" {
			t.Errorf("expected actor email admin@example.com, got %s", entry.ActorEmail)
		}
	}
}

func TestAudit_ReadOnlyEndpoint(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	adminU, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	_ = config.RecordAudit(b.DB, b.IDs, "admin@example.com", config.AuditProjectCreate, "proj1", "detail")

	hAudit := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.ListAuditHandler(b.DB))
	ctxAudit := &mock.Context{InMethod: "GET", InPath: config.PathAdminAudit}
	ctxAudit.SetUserID(adminU.Id)
	hAudit(ctxAudit)

	if ctxAudit.Status != 200 {
		t.Fatalf("ListAudit status %d, want 200", ctxAudit.Status)
	}

	var auditResp config.AdminAuditResponse
	_ = json.Decode(ctxAudit.ResponseBody(), &auditResp)
	if len(auditResp.Entries) != 1 || auditResp.Entries[0].Action != config.AuditProjectCreate {
		t.Fatalf("unexpected audit list: %+v", auditResp.Entries)
	}
}

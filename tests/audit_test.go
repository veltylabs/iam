package tests

import (
	"strings"
	"testing"

	"github.com/tinywasm/json"
	"github.com/tinywasm/router/mock"

	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/admin"
	"github.com/veltylabs/iam/routes"
)

func TestAudit_RecordsEveryMutation(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	adminU, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	// 1. Create Project
	hCreateProj := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.CreateProjectHandler(b.DB, b.IDs))
	ctx1 := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjects}
	ctx1.SetUserID(adminU.Id)
	ctx1.InBody = encodeBody(t, &config.AdminCreateProjectRequest{Name: "Audit Proj"})
	hCreateProj(ctx1)

	var projResp config.AdminSecretResponse
	_ = json.Decode(ctx1.ResponseBody(), &projResp)
	pID := projResp.Id

	// 2. Rotate Secret
	hRotate := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.RotateSecretHandler(b.DB, b.IDs))
	ctx2 := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjectRotate}
	ctx2.SetUserID(adminU.Id)
	ctx2.InBody = encodeBody(t, &config.AdminProjectIDRequest{ProjectId: pID})
	hRotate(ctx2)

	// 3. Create Role
	hCreateRole := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.CreateRoleHandler(b.DB, b.RBAC, b.IDs))
	ctx3 := &mock.Context{InMethod: "POST", InPath: config.PathAdminRoles}
	ctx3.SetUserID(adminU.Id)
	ctx3.InBody = encodeBody(t, &config.AdminCreateRoleRequest{ProjectId: pID, Code: "auditor", Name: "Auditor", Description: ""})
	hCreateRole(ctx3)

	// Fetch audit entries
	hAudit := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListAuditHandler(b.DB))
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

	hAudit := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListAuditHandler(b.DB))
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

// TestAudit_RecordsPanelDenial: no-admin con sesión válida tocando el panel
// → 403 + fila panel.access_denied con su email.
func TestAudit_RecordsPanelDenial(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, err := b.Auth.CreateUser("intruder@example.com", "Intruder", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	h := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListProjectsHandler(b.DB))
	ctx := &mock.Context{InMethod: "GET", InPath: config.PathAdminProjects}
	ctx.SetUserID(u.Id)
	h(ctx)
	if ctx.Status != 403 {
		t.Fatalf("status %d, want 403", ctx.Status)
	}

	entry := lastAudit(t, b.DB)
	if entry.Action != config.AuditPanelDenied {
		t.Errorf("action: got %q, want %q", entry.Action, config.AuditPanelDenied)
	}
	if entry.Target != "intruder@example.com" {
		t.Errorf("target: got %q, want the denied email", entry.Target)
	}
}

// TestAudit_RecordsOriginDenial: mutación desde un origen ajeno → 403 + fila
// panel.origin_denied.
func TestAudit_RecordsOriginDenial(t *testing.T) {
	b, r := setupPanel(t, "admin@example.com")

	ctx := crossSiteCtx(t, b, "admin@example.com")
	ctx.InMethod = "POST"
	ctx.InPath = config.PathAdminProjects
	ctx.InBody = encodeBody(t, &config.AdminCreateProjectRequest{Name: "Denied App"})
	r.Invoke("POST", config.PathAdminProjects, ctx)
	if ctx.Status != 403 {
		t.Fatalf("status %d, want 403", ctx.Status)
	}

	entry := lastAudit(t, b.DB)
	if entry.Action != config.AuditOriginDenied {
		t.Errorf("action: got %q, want %q", entry.Action, config.AuditOriginDenied)
	}
	if entry.Target != "admin@example.com" {
		t.Errorf("target: got %q, want the resolved admin email", entry.Target)
	}
}

// TestAudit_RecordsInvalidClientSecret: /api/token con client_secret
// inválido → 403 + fila token.secret_invalid con el project_id.
func TestAudit_RecordsInvalidClientSecret(t *testing.T) {
	backend := setupBackend(t)
	if err := config.CreateProject(backend.DB, "proj-audit-deny", "Audit Deny", "correct-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	handler := routes.Token(backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret, backend.IDs)
	ctx := &mock.Context{}
	ctx.SetUserID("user-1")
	body, err := encodeTokenRequest(t, "proj-audit-deny", "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx.InBody = body
	handler(ctx)
	if ctx.Status != 403 {
		t.Fatalf("status: got %d, want 403", ctx.Status)
	}

	entry := lastAudit(t, backend.DB)
	if entry.Action != config.AuditTokenDenied {
		t.Errorf("action: got %q, want %q", entry.Action, config.AuditTokenDenied)
	}
	if entry.Target != "proj-audit-deny" {
		t.Errorf("target: got %q, want the project id", entry.Target)
	}
	if entry.ActorEmail != "" {
		t.Errorf("actor: got %q, want empty (no hay admin en /api/token)", entry.ActorEmail)
	}
}

// TestAudit_NeverStoresClientSecret: tras un /api/token fallido, NINGUNA fila
// de auditoría contiene el secreto enviado.
func TestAudit_NeverStoresClientSecret(t *testing.T) {
	backend := setupBackend(t)
	if err := config.CreateProject(backend.DB, "proj-audit-leak", "Audit Leak", "correct-secret"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	const hostileSecret = "leak-me-client-secret-12345"
	handler := routes.Token(backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret, backend.IDs)
	ctx := &mock.Context{}
	ctx.SetUserID("user-1")
	body, err := encodeTokenRequest(t, "proj-audit-leak", hostileSecret)
	if err != nil {
		t.Fatal(err)
	}
	ctx.InBody = body
	handler(ctx)

	entries, err := config.ListAudit(backend.DB, 0)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}
	for _, e := range entries {
		for _, field := range []string{e.ActorEmail, e.Action, e.Target, e.Detail} {
			if strings.Contains(field, hostileSecret) {
				t.Errorf("audit entry stores the client_secret: %+v", e)
			}
		}
	}
}

// TestAudit_TruncatesHostileDetail: un Origin de 10 000 bytes se guarda
// truncado a auditDetailMax — es entrada del atacante y no puede inflar una
// fila sin techo.
func TestAudit_TruncatesHostileDetail(t *testing.T) {
	b, r := setupPanel(t, "admin@example.com")

	ctx := adminCtx(t, b, "admin@example.com")
	ctx.InMethod = "POST"
	ctx.InPath = config.PathAdminProjects
	ctx.SetHeader("Sec-Fetch-Site", "cross-site")
	ctx.SetHeader("Origin", strings.Repeat("x", 10000))
	ctx.InBody = encodeBody(t, &config.AdminCreateProjectRequest{Name: "Huge Origin"})
	r.Invoke("POST", config.PathAdminProjects, ctx)
	if ctx.Status != 403 {
		t.Fatalf("status %d, want 403", ctx.Status)
	}

	entry := lastAudit(t, b.DB)
	if entry.Action != config.AuditOriginDenied {
		t.Fatalf("action: got %q, want %q", entry.Action, config.AuditOriginDenied)
	}
	if len(entry.Detail) > 200 {
		t.Errorf("detail length %d, want at most 200", len(entry.Detail))
	}
}

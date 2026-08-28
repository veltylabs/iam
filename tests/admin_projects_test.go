package tests

import (
	"testing"

	"github.com/tinywasm/json"
	"github.com/tinywasm/model"
	"github.com/tinywasm/router/mock"
	"github.com/tinywasm/sqlt"

	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/admin"
)

func encodeBody(t *testing.T, v model.Encodable) []byte {
	t.Helper()
	var out []byte
	if err := json.Encode(v, &out); err != nil {
		t.Fatalf("encodeBody: %v", err)
	}
	return out
}

func TestCreateProject_ReturnsSecretOnceAndVerifies(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	h := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.CreateProjectHandler(b.DB, b.IDs))
	ctx := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjects}
	ctx.SetUserID(u.Id)
	ctx.InBody = encodeBody(t, &config.AdminCreateProjectRequest{Name: "Test App"})

	h(ctx)
	if ctx.Status != 201 {
		t.Fatalf("CreateProject status %d, want 201", ctx.Status)
	}

	var secretResp config.AdminSecretResponse
	if err := json.Decode(ctx.ResponseBody(), &secretResp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if secretResp.Id == "" || secretResp.ClientSecret == "" {
		t.Fatalf("invalid secret response: %+v", secretResp)
	}

	ok, err := config.VerifyProjectSecret(b.DB, secretResp.Id, secretResp.ClientSecret)
	if err != nil || !ok {
		t.Fatalf("VerifyProjectSecret failed: ok=%v err=%v", ok, err)
	}

	okBad, _ := config.VerifyProjectSecret(b.DB, secretResp.Id, "wrong-secret")
	if okBad {
		t.Fatalf("VerifyProjectSecret accepted wrong secret")
	}

	hList := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.ListProjectsHandler(b.DB))
	ctxList := &mock.Context{InMethod: "GET", InPath: config.PathAdminProjects}
	ctxList.SetUserID(u.Id)
	hList(ctxList)

	var listResp config.AdminProjectsResponse
	_ = json.Decode(ctxList.ResponseBody(), &listResp)
	if len(listResp.Projects) != 1 || listResp.Projects[0].Name != "Test App" {
		t.Fatalf("unexpected projects list: %+v", listResp.Projects)
	}
}

func TestRotateSecret_OldStopsVerifying(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	secret1, _ := config.GenerateClientSecret()
	_ = config.CreateProject(b.DB, "proj1", "App 1", secret1)

	hRotate := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.RotateSecretHandler(b.DB, b.IDs))
	ctx := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjectRotate}
	ctx.SetUserID(u.Id)
	ctx.InBody = encodeBody(t, &config.AdminProjectIDRequest{ProjectId: "proj1"})

	hRotate(ctx)
	if ctx.Status != 200 {
		t.Fatalf("RotateSecret status %d, want 200", ctx.Status)
	}

	var rotResp config.AdminSecretResponse
	_ = json.Decode(ctx.ResponseBody(), &rotResp)
	secret2 := rotResp.ClientSecret

	okOld, _ := config.VerifyProjectSecret(b.DB, "proj1", secret1)
	if okOld {
		t.Fatalf("old secret still verified after rotation")
	}

	okNew, err := config.VerifyProjectSecret(b.DB, "proj1", secret2)
	if err != nil || !okNew {
		t.Fatalf("new secret failed verification: ok=%v err=%v", okNew, err)
	}
}

func TestRotateSecret_UnknownProject404(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	hRotate := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.RotateSecretHandler(b.DB, b.IDs))
	ctx := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjectRotate}
	ctx.SetUserID(u.Id)
	ctx.InBody = encodeBody(t, &config.AdminProjectIDRequest{ProjectId: "non-existent"})

	hRotate(ctx)
	if ctx.Status != 404 {
		t.Fatalf("RotateSecret status %d, want 404", ctx.Status)
	}
}

func TestDeactivateProject_BlocksTokenIssue(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, _ := b.Auth.CreateUser("admin@example.com", "Admin", "")

	secret, _ := config.GenerateClientSecret()
	_ = config.CreateProject(b.DB, "proj1", "App 1", secret)

	hActive := admin.RequirePanelAdmin(b.Auth, []string{"admin@example.com"}, admin.SetActiveHandler(b.DB, b.IDs))
	ctx := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjectActive}
	ctx.SetUserID(u.Id)
	ctx.InBody = encodeBody(t, &config.AdminSetActiveRequest{ProjectId: "proj1", Active: false})

	hActive(ctx)
	if ctx.Status != 200 {
		t.Fatalf("SetActive status %d, want 200", ctx.Status)
	}

	okDeactive, _ := config.VerifyProjectSecret(b.DB, "proj1", secret)
	if okDeactive {
		t.Fatalf("deactivated project accepted valid secret")
	}

	ctxReactivate := &mock.Context{InMethod: "POST", InPath: config.PathAdminProjectActive}
	ctxReactivate.SetUserID(u.Id)
	ctxReactivate.InBody = encodeBody(t, &config.AdminSetActiveRequest{ProjectId: "proj1", Active: true})
	hActive(ctxReactivate)

	okActive, _ := config.VerifyProjectSecret(b.DB, "proj1", secret)
	if !okActive {
		t.Fatalf("reactivated project rejected valid secret")
	}
}

func TestExistingRowWithoutActiveColumn_TreatedAsActive(t *testing.T) {
	b := setupBackend(t)

	secret, _ := config.GenerateClientSecret()
	err := config.CreateProject(b.DB, "legacy", "Legacy App", secret)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	conn := b.DB.RawConn()
	_ = conn.Exec("UPDATE project SET active = 0 WHERE id = 'legacy'")

	compiler := sqlt.NewCompiler()
	if err := config.MigrateSchema(conn, compiler); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	ok, err := config.VerifyProjectSecret(b.DB, "legacy", secret)
	if err != nil || !ok {
		t.Fatalf("legacy row not treated as active after MigrateSchema")
	}
}

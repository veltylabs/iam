package tests

import (
	"reflect"
	"testing"

	"github.com/tinywasm/json"
	"github.com/tinywasm/router/mock"

	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/admin"
)

func TestPanelGate_NoSession401(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	h := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListProjectsHandler(b.DB))

	ctx := &mock.Context{InMethod: "GET", InPath: config.PathAdminProjects}
	h(ctx)
	if ctx.Status != 401 {
		t.Fatalf("expected status 401, got %d", ctx.Status)
	}
}

func TestPanelGate_NonAdmin403(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, err := b.Auth.CreateUser("user@example.com", "User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	h := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListProjectsHandler(b.DB))
	ctx := &mock.Context{InMethod: "GET", InPath: config.PathAdminProjects}
	ctx.SetUserID(u.Id)
	h(ctx)
	if ctx.Status != 403 {
		t.Fatalf("expected status 403, got %d", ctx.Status)
	}
}

func TestPanelGate_Admin200(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	u, err := b.Auth.CreateUser("admin@example.com", "Admin", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	h := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListProjectsHandler(b.DB))
	ctx := &mock.Context{InMethod: "GET", InPath: config.PathAdminProjects}
	ctx.SetUserID(u.Id)
	h(ctx)
	if ctx.Status != 200 {
		t.Fatalf("expected status 200, got %d", ctx.Status)
	}

	var resp config.AdminProjectsResponse
	if err := json.Decode(ctx.ResponseBody(), &resp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
}

func TestIsPanelAdmin(t *testing.T) {
	list := []string{"admin@example.com", "super@example.com"}
	cases := []struct {
		email string
		want  bool
	}{
		{"admin@example.com", true},
		{"super@example.com", true},
		{"other@example.com", false},
		{"Admin@example.com", false}, // case-sensitive
		{"", false},
	}
	for _, c := range cases {
		got := config.IsPanelAdmin(c.email, list)
		if got != c.want {
			t.Errorf("IsPanelAdmin(%q) = %v, want %v", c.email, got, c.want)
		}
	}
	if config.IsPanelAdmin("admin@example.com", nil) != false {
		t.Errorf("expected false for nil list")
	}
}

func TestPanelAdminList_ParsesEnv(t *testing.T) {
	t.Setenv(config.EnvAdminEmails, " a@x.com , b@x.com ,, c@x.com ")
	list := config.PanelAdminList()
	want := []string{"a@x.com", "b@x.com", "c@x.com"}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("PanelAdminList() = %v, want %v", list, want)
	}

	t.Setenv(config.EnvAdminEmails, "")
	if config.PanelAdminList() != nil {
		t.Fatalf("expected nil for empty EnvAdminEmails")
	}
}

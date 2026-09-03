package tests

import (
	"testing"

	"github.com/tinywasm/json"
	"github.com/tinywasm/model"
	"github.com/tinywasm/router/mock"

	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/admin"
	"github.com/veltylabs/iam/routes"
)

// TestHostileConsumerCannotInjectIntoPanel reproduce la cadena de ataque
// completa que motivó la auditoría, con el stack real:
//
// 1. Un proyecto legítimo obtiene su client_secret.
// 2. Ese consumidor, con secreto válido, manda datos hostiles
//    (name con un XSS) por POST /api/users/resolve.
// 3. El panel le asigna un rol al usuario.
// 4. El administrador pide GET /api/admin/roles/users.
// 5. El name vuelve TAL CUAL en el JSON: iam no sanea datos en el borde
//    (sanear ahí corrompe el dato), y el panel los muestra inertes porque
//    tinywasm/dom escapa.
// 6. El paso 6 (el HTML renderizado no contiene <img) se afirma en el propio
//    tinywasm/dom como TestAdminTable_RendersHostileUserDataInert — ver ese
//    test. No se inventa un renderizador de HTML en tests/ para afirmarlo
//    localmente: eso sería recrear tinywasm/dom (Restricción #2).
func TestHostileConsumerCannotInjectIntoPanel(t *testing.T) {
	b, _ := setupPanel(t, "admin@example.com")
	adminU, err := b.Auth.CreateUser("admin@example.com", "Admin", "")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}

	// 1. Proyecto legítimo con su client_secret.
	secret, err := config.GenerateClientSecret()
	if err != nil {
		t.Fatalf("GenerateClientSecret: %v", err)
	}
	if err := config.CreateProject(b.DB, "proj-hostile", "Hostile Consumer", secret); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// 2. Consumidor legítimo, secreto válido, datos hostiles.
	const hostileName = `<img src=x onerror=alert(1)>`
	resolveHandler := routes.ResolveUser(b.DB, b.Auth)
	resolveCtx := &mock.Context{InMethod: "POST", InPath: config.PathUsersResolve}
	var resolveBody []byte
	if err := json.Encode(&routes.ResolveUserRequest{
		ProjectID: "proj-hostile", ClientSecret: secret, Email: "hostile@example.com", Name: hostileName,
	}, &resolveBody); err != nil {
		t.Fatalf("encode: %v", err)
	}
	resolveCtx.InBody = resolveBody
	resolveHandler(resolveCtx)
	if resolveCtx.Status != 200 {
		t.Fatalf("ResolveUser status %d, want 200", resolveCtx.Status)
	}
	var resolveResp routes.ResolveUserResponse
	if err := json.Decode(resolveCtx.ResponseBody(), &resolveResp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if resolveResp.Sub == "" {
		t.Fatal("ResolveUser returned empty sub")
	}

	// 3. El panel le asigna un rol.
	if err := b.RBAC.CreateRole("proj-hostile", "r1", model.RoleCode("member"), "Member", ""); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	hAssign := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.AssignUserHandler(b.DB, b.Auth, b.RBAC, b.IDs))
	assignCtx := &mock.Context{InMethod: "POST", InPath: config.PathAdminUserAssign}
	assignCtx.SetUserID(adminU.Id)
	assignCtx.InBody = encodeBody(t, &config.AdminUserRoleRequest{ProjectId: "proj-hostile", Code: "member", Email: "hostile@example.com"})
	hAssign(assignCtx)
	if assignCtx.Status != 200 {
		t.Fatalf("AssignUser status %d, want 200", assignCtx.Status)
	}

	// 4 y 5. El administrador lista los usuarios del rol: el name vuelve tal
	// cual (iam no sanea), y es el panel — vía tinywasm/dom — quien lo
	// muestra inerte.
	hUsers := admin.RequirePanelAdmin(b.DB, b.IDs, b.Auth, []string{"admin@example.com"}, admin.ListRoleUsersHandler(b.DB, b.Auth, b.RBAC))
	listCtx := &mock.Context{InMethod: "GET", InPath: config.PathAdminRoleUsers + "?project_id=proj-hostile&code=member"}
	listCtx.SetUserID(adminU.Id)
	hUsers(listCtx)
	if listCtx.Status != 200 {
		t.Fatalf("ListRoleUsers status %d, want 200", listCtx.Status)
	}
	var usersResp config.AdminRoleUsersResponse
	if err := json.Decode(listCtx.ResponseBody(), &usersResp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(usersResp.Users) != 1 {
		t.Fatalf("users: got %+v, want exactly one", usersResp.Users)
	}
	if usersResp.Users[0].Name != hostileName {
		t.Errorf("name: got %q, want it verbatim %q (iam must not sanitize at the edge)", usersResp.Users[0].Name, hostileName)
	}
}

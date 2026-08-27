← [Etapa 1](PLAN_STAGE_1_CONFIG.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 3](PLAN_STAGE_3_ROUTES_ENTRY.md)

# Etapa 2 — `modules/admin/`: los handlers del panel

Paquete `modules/admin`, **sin build tag** (compila al Worker → regla TinyGo),
archivos **planos** con nombres canónicos. Importa `config/` y
`tinywasm/{router,orm,rbac,auth/authority,model,fmt}`; **nunca `routes/`**;
**nunca `dom`/`html`/`form`/`layout`/`svg`**.

```
modules/admin/
├── backend.go   # ListRoles, ListRoleUsers (lectura componiendo codecs de rbac)
└── handler.go   # RequirePanelAdmin + los 13 constructores de handler
                 # (si pasa 500 líneas: handler_projects.go / handler_roles.go /
                 #  handler_users.go / handler_audit.go — planos, sin subcarpetas)
```

Los DTOs de cable ya están en `config/` (Etapa 1.4) → **no** hay `model.go`.

## 2.1 — `backend.go`

`tinywasm/rbac` no expone "listar roles de un proyecto" ni "usuarios de un
rol"; se compone con sus **codecs ORM exportados** sobre el `*orm.DB`
compartido (permitido; **NO** agregues métodos a `rbac`). Antes de escribir,
confirmá en `tinywasm/rbac` (v0.0.7, `models_orm.go`) que `ReadAllRole`,
`Role_`, `Role`, `ReadAllUserRole`, `UserRole_` están exportados. Si alguno no,
**PARÁ y REPORTÁ**.

```go
package admin

// ListRoles: roles de projectID como config.RoleView, con UserCount.
//   qb := db.Query(&rbac.Role{}).Where(rbac.Role_.ProjectId).Eq(projectID)
//   roles, _ := rbac.ReadAllRole(qb)
//   UserCount = nº de filas de rbac.ReadAllUserRole(
//     db.Query(&rbac.UserRole{}).Where(rbac.UserRole_.ProjectId).Eq(projectID).Where(rbac.UserRole_.RoleId).Eq(role.Id))
func ListRoles(db *orm.DB, projectID string) ([]config.RoleView, error)

// ListRoleUsers: usuarios con roleCode en projectID.
//   role, err := rbacSvc.GetRoleByCode(projectID, model.RoleCode(roleCode))  // orm.ErrNotFound → nil, err
//   urs, _ := rbac.ReadAllUserRole(db.Query(&rbac.UserRole{}).Where(...ProjectId).Where(...RoleId=role.Id))
//   por cada ur: u, _ := authMod.GetUser(ur.UserId) → config.RoleUser{u.Email, u.Name, u.Id}
func ListRoleUsers(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, projectID, roleCode string) ([]config.RoleUser, error)
```

## 2.2 — `handler.go`: el gate

```go
// RequirePanelAdmin envuelve un handler: 401 sin sesión, 403 si el correo de
// la sesión no está en adminEmails. En el camino feliz pasa el correo del
// admin al handler (para auditoría).
func RequirePanelAdmin(authMod *authority.Module, adminEmails []string, h func(ctx router.Context, adminEmail string)) router.HandlerFunc {
	return func(ctx router.Context) {
		uid := ctx.UserID()
		if uid == "" { ctx.WriteStatus(401); return }
		u, err := authMod.GetUser(uid)
		if err != nil { ctx.WriteStatus(500); return }
		if !config.IsPanelAdmin(u.Email, adminEmails) { ctx.WriteStatus(403); return }
		h(ctx, u.Email)
	}
}
```

## 2.3 — `handler.go`: los handlers

Un `func(...) func(ctx router.Context, adminEmail string)` por ruta, estilo
`routes/routes.go` actual (`ctx.Decode`/`ctx.Encode`/`ctx.WriteStatus`/`ctx.UserID()`).
**Todos** con la firma `func(ctx router.Context, adminEmail string)` (los GET
de solo lectura ignoran `adminEmail`). Para GET con query usá el lector de
query string de `router.Context` (v0.1.28); si no existe, pasá esas 2 rutas
(`roles`, `roles/users`) a POST con body y ajustá `config/api.go` — **PARÁ y
REPORTÁ** si el router no ofrece ninguna de las dos.

Cuerpo: (1) decodificar / leer query; validar no-vacíos → `400`. (2) ejecutar.
(3) en éxito de una mutación: `config.RecordAudit(db, ids, adminEmail, <code>,
<target>, <detail>)`, logueando `fmt.Println("audit:", err)` si falla, sin
tocar el status. (4) `ctx.Encode(&resp)` con `200` (o `201` al crear).

| Constructor | Ruta | Lógica | Códigos |
|---|---|---|---|
| `Me(authMod)` | `PathAdminMe` | `config.AdminMeResponse{Email: adminEmail, Name: authMod.GetUser(ctx.UserID()).Name}` | 200 |
| `ListProjects(db)` | `GET PathAdminProjects` | `config.ListProjects` → `AdminProjectView` (`Active: row.Active != 0`) | 200 |
| `CreateProject(db, ids)` | `POST PathAdminProjects` | valida `name`; `id := ids.New…()`; `secret,_ := config.GenerateClientSecret()`; `config.CreateProject(db,id,name,secret)`; audita `AuditProjectCreate` target=id; resp `AdminSecretResponse{id, secret}` | 201/400/500 |
| `RotateSecret(db, ids)` | `POST PathAdminProjectRotate` | body `AdminProjectIDRequest`; `secret,_ := config.GenerateClientSecret()`; `config.RegenerateProjectSecret` (`ErrProjectNotFound`→404); audita `AuditProjectRotate`; resp `AdminSecretResponse` | 200/400/404/500 |
| `SetActive(db, ids)` | `POST PathAdminProjectActive` | body `AdminSetActiveRequest`; `config.SetProjectActive`; audita activate/deactivate | 200/400/404/500 |
| `ListRoles(db)` | `GET PathAdminRoles` | `admin.ListRoles(db, project_id)` | 200/400 |
| `CreateRole(db, rbacSvc, ids)` | `POST PathAdminRoles` | body `AdminCreateRoleRequest`; `roleID := ids.New…()`; `rbacSvc.CreateRole(project_id, roleID, model.RoleCode(code), name, description)`; audita `AuditRoleCreate` target=`project_id+"/"+code` | 201/400/500 |
| `SetRoleTTL(db, rbacSvc, ids)` | `POST PathAdminRoleTTL` | body `AdminRoleTTLRequest`; `role,_ := rbacSvc.GetRoleByCode(project_id, code)` (`orm.ErrNotFound`→404); `rbacSvc.SetRoleSessionTTL(project_id, role.Id, session_ttl)`; audita `AuditRoleSetTTL` detail=ttl | 200/400/404/500 |
| `DeleteRole(db, rbacSvc, ids)` | `POST PathAdminRoleDelete` | body `AdminRoleRefRequest`; `role` vía `GetRoleByCode`; `rbacSvc.DeleteRole(project_id, role.Id)`; audita `AuditRoleDelete` | 200/400/404/500 |
| `ListRoleUsers(db, authMod, rbacSvc)` | `GET PathAdminRoleUsers` | `admin.ListRoleUsers(...)` | 200/400/404 |
| `AssignUser(db, authMod, rbacSvc, ids)` | `POST PathAdminUserAssign` | body `AdminUserRoleRequest`; valida email; `u,err := authMod.UserByEmail(email)`; si err → `u,err = authMod.CreateUser(email, email, "")`; `role,err := rbacSvc.GetRoleByCode(project_id, code)` (404); `rbacSvc.AssignRole(project_id, u.Id, role.Id)` (idempotente); audita `AuditUserAssign` detail=email; resp `AdminAssignResponse{Sub: u.Id}` | 200/400/404/500 |
| `RevokeUser(db, authMod, rbacSvc, ids)` | `POST PathAdminUserRevoke` | body `AdminUserRoleRequest`; `u` vía `UserByEmail` (404 si no existe); `role` vía `GetRoleByCode` (404); `rbacSvc.RevokeRole(project_id, u.Id, role.Id)`; audita `AuditUserRevoke` | 200/400/404/500 |
| `ListAudit(db)` | `GET PathAdminAudit` | `config.ListAudit(db, 200)` → `AdminAuditEntry` | 200 |

## Criterios (Etapa 2)

- `gotest ./modules/admin/...` compila; `go vet ./...` limpio.
- `grep -rn "veltylabs/iam/routes" modules/admin/` → vacío.
- `grep -rn "tinywasm/\(dom\|html\|form\|layout\|svg\)" modules/admin/` → vacío.
- `grep -rn "map\[\|\"strings\"\|encoding/json\|os\." modules/admin/` → vacío.
- `find modules -mindepth 2 -type d` → vacío.
- Existen: `admin.RequirePanelAdmin`, `admin.ListRoles`, `admin.ListRoleUsers`,
  y los 13 constructores.

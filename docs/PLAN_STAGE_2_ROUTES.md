← [Etapa 1](PLAN_STAGE_1_BACKEND.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 3](PLAN_STAGE_3_PANEL_UI.md)

# Etapa 2 — Rutas `/admin/api/*`

Vive en `routes/`. Compila para el Worker: regla TinyGo. Mirá `routes/routes.go`
actual para el estilo exacto (structs con `IsNil`/`EncodeFields`/`DecodeFields`,
`ctx.Decode`/`ctx.Encode`, `ctx.WriteStatus`, `ctx.UserID()`, handlers
`func(...) router.HandlerFunc`).

## 2.1 — Constantes de ruta: `api/api.go`

Añadí al bloque `const`:

```go
const (
	PathAdminMe             = "/admin/api/me"
	PathAdminProjects       = "/admin/api/projects"          // GET lista, POST crea
	PathAdminProjectRotate  = "/admin/api/projects/rotate"   // POST { project_id }
	PathAdminProjectActive  = "/admin/api/projects/active"   // POST { project_id, active }
	PathAdminRoles          = "/admin/api/roles"             // GET ?project_id=, POST crea
	PathAdminRoleTTL        = "/admin/api/roles/ttl"         // POST { project_id, code, session_ttl }
	PathAdminRoleDelete     = "/admin/api/roles/delete"      // POST { project_id, code }
	PathAdminRoleUsers      = "/admin/api/roles/users"       // GET ?project_id=&code=
	PathAdminUserAssign     = "/admin/api/users/assign"      // POST { project_id, code, email }
	PathAdminUserRevoke     = "/admin/api/users/revoke"      // POST { project_id, code, email }
	PathAdminAudit          = "/admin/api/audit"             // GET
)
```

(Rutas con verbos distintos sobre el mismo recurso van como sub-paths
explícitos porque el router del ecosistema enruta por path, no combina
método+path para elegir handler. Seguí el patrón de sub-paths que ya usa
`misitio` — `/api/admin/accept/<id>` etc.)

## 2.2 — Gate: `routes/adminauth.go` (nuevo)

```go
package routes

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/router"
	"github.com/veltylabs/iam/config"
)

// requirePanelAdmin envuelve un handler: 401 si no hay sesión, 403 si el
// correo de la sesión no está en adminEmails. En el camino feliz, deja el
// correo del admin disponible vía adminEmailFromCtx para que el handler lo
// registre en auditoría.
func requirePanelAdmin(authMod *authority.Module, adminEmails []string, h func(ctx router.Context, adminEmail string)) router.HandlerFunc {
	return func(ctx router.Context) {
		uid := ctx.UserID()
		if uid == "" {
			ctx.WriteStatus(401)
			return
		}
		u, err := authMod.GetUser(uid)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !config.IsPanelAdmin(u.Email, adminEmails) {
			ctx.WriteStatus(403)
			return
		}
		h(ctx, u.Email)
	}
}
```

Todas las rutas `/admin/api/*` (incluida `PathAdminMe`) se montan `.Public()`
a nivel del router (no `.Authenticated()`), y el gate real es
`requirePanelAdmin` **dentro** del handler — mismo patrón que
`PathUsersResolve` en `routes.go` (comentario "Public() at the router's
access-gate level ... NOT reachable without a valid ..."). El `Authn` del
servidor/edge ya resuelve la cookie SSO y setea `ctx.UserID()`.

## 2.3 — `routes.Register` gana `adminEmails []string`

Firma nueva:

```go
func Register(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte, adminEmails []string)
```

- `edge/main.go`: pasar `config.PanelAdminList()`.
- `web/server.go`: calcular la lista así (ver Etapa 4 para el archivo
  completo):
  ```go
  adminEmails := config.PanelAdminList()
  if len(adminEmails) == 0 {
  	// En local, la identidad mock de desarrollo entra sin configurar nada.
  	adminEmails = []string{config.LocalScenarios[0].Email}
  }
  ```
- Todos los `tests/` que llaman `routes.Register` pasan un `[]string{...}`
  explícito (Etapa 5).

## 2.4 — Handlers

Los **structs de request/response** (`AdminMeResponse`, `AdminProjectsResponse`,
… todos los `Admin*` de esta sección) van en **`api/admin.go`**, NO en
`routes/`. Motivo: el módulo WASM del panel (Etapa 3) los decodifica y no debe
importar `routes` (arrastra el Worker). `api/` es paquete hoja, sin lógica —
solo tipos y sus `EncodeFields`/`DecodeFields`. `routes/` y `modules/` los
importan desde ahí. `config.RoleView` / `config.RoleUser` (Etapa 1) se
referencian por su paquete; si eso crea un ciclo `api → config`, mové también
`RoleView`/`RoleUser` a `api/` y que `config` los importe.

Creá `routes/admin.go` con los **handlers** (si supera 500 líneas, partí en
`admin_projects.go` / `admin_roles.go` / `admin_users.go` / `admin_audit.go`).
Un handler por ruta. Patrón común:

1. Decodificar el body con `ctx.Decode(&req)` (POST) o leer query con
   `ctx.Query("project_id")` (GET) — usá el método de query que exponga
   `router.Context` en `v0.1.28`; si no hay lectura de query string,
   convertí esas rutas GET a POST con body. **Pará y reportá** si el router
   no ofrece ninguna de las dos.
2. Validar campos obligatorios no vacíos → `400`.
3. Ejecutar la operación de `config`/`rbacSvc`.
4. En éxito de una mutación: `config.RecordAudit(db, ids, adminEmail, <code>,
   <target>, <detail>)`, logueando `fmt.Println("audit:", err)` si falla,
   sin cambiar el status.
5. `ctx.Encode(&resp)` con `200` (o `201` para crear).

`ids` (`model.IDGenerator`) hace falta en los handlers de mutación para
`RecordAudit`. Pasalo a `Register` también, o guardalo junto a `db`. La forma
más limpia: `Register` ya recibe `db`; agregá `ids model.IDGenerator` al
final de la firma y propagalo. Actualizá `edge/main.go`, `web/server.go` y
tests.

### Structs (todas con `IsNil`/`EncodeFields`/`DecodeFields`, estilo `routes.go`)

```go
// GET /admin/api/me
type AdminMeResponse struct {
	Email   string
	Name    string
	IsAdmin bool   // siempre true si llegó acá (el gate ya filtró); el panel
	               // lo usa para distinguir 200-no-admin imposible de 200-ok
}

// GET /admin/api/projects  → { projects: [...] }
type AdminProjectView struct {
	Id        string
	Name      string
	Active    bool
	CreatedAt int64
}
type AdminProjectsResponse struct{ Projects []AdminProjectView }

// POST /admin/api/projects  { name }  → 201 { id, client_secret }
type AdminCreateProjectRequest struct{ Name string }
type AdminCreateProjectResponse struct {
	Id           string
	ClientSecret string // EN CLARO — única vez que se devuelve
}

// POST /admin/api/projects/rotate  { project_id }  → 200 { client_secret }
type AdminRotateRequest struct{ ProjectId string }
type AdminRotateResponse struct{ ClientSecret string }

// POST /admin/api/projects/active  { project_id, active }  → 200 {}
type AdminSetActiveRequest struct {
	ProjectId string
	Active    bool
}

// GET /admin/api/roles?project_id=  → { roles: [...] }  (api.RoleView, mismo paquete)
type AdminRolesResponse struct{ Roles []RoleView }

// POST /admin/api/roles  { project_id, code, name, description }  → 201 {}
type AdminCreateRoleRequest struct {
	ProjectId   string
	Code        string
	Name        string
	Description  string
}

// POST /admin/api/roles/ttl  { project_id, code, session_ttl }  → 200 {}
type AdminRoleTTLRequest struct {
	ProjectId  string
	Code       string
	SessionTtl int64
}

// POST /admin/api/roles/delete  { project_id, code }  → 200 {}
type AdminRoleDeleteRequest struct {
	ProjectId string
	Code      string
}

// GET /admin/api/roles/users?project_id=&code=  → { users: [...] } (api.RoleUser, mismo paquete)
type AdminRoleUsersResponse struct{ Users []RoleUser }

// POST /admin/api/users/assign  { project_id, code, email }  → 200 { sub }
// POST /admin/api/users/revoke  { project_id, code, email }  → 200 {}
type AdminUserRoleRequest struct {
	ProjectId string
	Code      string
	Email     string
}
type AdminAssignResponse struct{ Sub string }

// GET /admin/api/audit  → { entries: [...] }
type AdminAuditEntry struct {
	ActorEmail string
	Action     string
	Target     string
	Detail     string
	CreatedAt  int64
}
type AdminAuditResponse struct{ Entries []AdminAuditEntry }
```

Para `Projects []AdminProjectView` y demás slices, seguí el patrón de
`MeResponse` de `misitio` (`w.Array("projects", len(...))` + `arr.Object(...)`
+ `arr.Close()`), o el que `routes.go` ya use para colecciones. Si `routes.go`
no serializa slices hoy, mirá cómo lo hace `veltylabs/misitio/routes/routes.go`
`MeResponse` (misma versión de `tinywasm/model`):
`https://github.com/veltylabs/misitio/blob/main/routes/routes.go`.

### Semántica por handler

| Ruta | Lógica | Códigos |
|---|---|---|
| `GET PathAdminMe` | devuelve email/nombre del admin | 200 / 401 / 403 |
| `GET PathAdminProjects` | `config.ListProjects` → map a `AdminProjectView` (`Active: row.Active != 0`) | 200 |
| `POST PathAdminProjects` | valida `name`; `id := ids.NewID()`; `secret, _ := config.GenerateClientSecret()`; `config.CreateProject(db, id, name, secret)`; audita `AuditProjectCreate` target=id; responde `{id, secret}` | 201 / 400 / 500 |
| `POST PathAdminProjectRotate` | `secret,_ := config.GenerateClientSecret()`; `config.RegenerateProjectSecret(db, project_id, secret)` (`ErrProjectNotFound` → 404); audita `AuditProjectRotate`; responde `{secret}` | 200 / 400 / 404 / 500 |
| `POST PathAdminProjectActive` | `config.SetProjectActive`; audita activate/deactivate según `active` | 200 / 400 / 404 / 500 |
| `GET PathAdminRoles` | `config.ListRoles(db, project_id)` | 200 / 400 |
| `POST PathAdminRoles` | `roleID := ids.NewID()`; `rbacSvc.CreateRole(project_id, roleID, model.RoleCode(code), name, description)`; audita `AuditRoleCreate` target=`project_id+"/"+code` | 201 / 400 / 500 |
| `POST PathAdminRoleTTL` | resolvé `role.Id` con `rbacSvc.GetRoleByCode(project_id, code)` (404 si `orm.ErrNotFound`); `rbacSvc.SetRoleSessionTTL(project_id, role.Id, session_ttl)`; audita `AuditRoleSetTTL` detail=`session_ttl` | 200 / 400 / 404 / 500 |
| `POST PathAdminRoleDelete` | `role.Id` vía `GetRoleByCode`; `rbacSvc.DeleteRole(project_id, role.Id)`; audita `AuditRoleDelete` | 200 / 400 / 404 / 500 |
| `GET PathAdminRoleUsers` | `config.ListRoleUsers(db, authMod, rbacSvc, project_id, code)` | 200 / 400 / 404 |
| `POST PathAdminUserAssign` | valida email; `u, err := authMod.UserByEmail(email)`; si error → `u, err = authMod.CreateUser(email, email, "")`; `role, err := rbacSvc.GetRoleByCode(project_id, code)` (404); `rbacSvc.AssignRole(project_id, u.Id, role.Id)` (idempotente); audita `AuditUserAssign` target=`project_id+"/"+code` detail=email; responde `{sub: u.Id}` | 200 / 400 / 404 / 500 |
| `POST PathAdminUserRevoke` | `u` vía `UserByEmail` (404 si no existe); `role` vía `GetRoleByCode` (404); `rbacSvc.RevokeRole(project_id, u.Id, role.Id)`; audita `AuditUserRevoke` | 200 / 400 / 404 / 500 |
| `GET PathAdminAudit` | `config.ListAudit(db, 200)` → map a `AdminAuditEntry` | 200 |

## 2.5 — Montaje en `routes.Register`

Después de las rutas actuales, añadí:

```go
r.Get(PathAdminMe, requirePanelAdmin(authMod, adminEmails, adminMe())).Public()
r.Get(PathAdminProjects, requirePanelAdmin(authMod, adminEmails, listProjects(db))).Public()
r.Post(PathAdminProjects, requirePanelAdmin(authMod, adminEmails, createProject(db, ids))).Public()
// … una línea por ruta de 2.1, GET con r.Get, POST con r.Post …
```

Mantené el comentario de cabecera de `Register` sobre "ninguna ruta lleva
CORS: el llamador es siempre el SERVIDOR de un proyecto consumidor" — extendelo
en una frase: el panel también es server-side (la cookie SSO viaja sola), así
que `/admin/api/*` tampoco lleva CORS.

## Criterios de aceptación (Etapa 2)

- `gotest ./routes/...` compila; `go vet ./...` limpio.
- `edge/main.go` y `web/server.go` compilan con la firma nueva de `Register`.
- `grep -rn "requirePanelAdmin" routes/` muestra que **todas** las rutas
  `/admin/api/*` pasan por él (13 montajes de handler: `me`, `projects` GET +
  POST, `projects/rotate`, `projects/active`, `roles` GET + POST, `roles/ttl`,
  `roles/delete`, `roles/users`, `users/assign`, `users/revoke`, `audit`).
- Ninguna ruta `/admin/api/*` está montada `.Authenticated()` — todas
  `.Public()` con gate interno.
- No hay `map[`, `os.`, `encoding/json`, `errors`, `strings` en `routes/`.

← [Etapa 2](PLAN_STAGE_2_ROUTES.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 4](PLAN_STAGE_4_SERVE.md)

# Etapa 3 — Panel WASM

Compila a `web/public/client.wasm` desde `web/client.go` con
`//go:build wasm`. Este binario es un **asset estático** — SIN límite de
plataforma (`AGENTS.md` Restricción #4), puede importar todo el kit de UI.
**No debe compartir código nuevo con `edge/main.go`**: los módulos viven en
`modules/*` con `//go:build wasm` y solo los importa `web/client.go`.

Seguí el skill `components` al pie de la letra: embeber `dom.Element` por
valor; SSR split en archivos con extensión (`css.go`, `svg.go`) con
`//go:build !wasm`; sin `front.go`; interactividad en el archivo principal vía
`OnMount()`; nunca clases CSS sueltas; tokens `var(--color-*)`,
`var(--mag-*)` sin fallback; iconos por sprite (`IconSvg()` en `ssr.go`,
`<svg><use href="#id">` en `Render()`).

## 3.1 — Chasis: `tinywasm/layout/platformd`

Añadí a `go.mod` (todas son deps del panel, no del Worker):
`github.com/tinywasm/layout`, `github.com/tinywasm/components`,
`github.com/tinywasm/form`, `github.com/tinywasm/dom`,
`github.com/tinywasm/html`, `github.com/tinywasm/svg`. `github.com/tinywasm/fetch`
y `github.com/tinywasm/json` ya están.

El chasis se usa así (calcá
`github.com/tinywasm/layout/platformd/web/client.go` y
`https://github.com/veltylabs/misitio/blob/main/web/client.go`):

```go
p := &platformd.Platform{
	AppName:   "iam · administración",
	DefaultID: ModuleProjects,
	User:      adminIdentity{email: me.Email, name: me.Name},
	CanView:   func(id string) bool { return true }, // el gate real es server-side
}
p.Modules = []platformd.UIModule{
	platformd.NewUIModule(ModuleProjects, "Proyectos", IconProjects, projectsView()),
	platformd.NewUIModule(ModuleRoles, "Roles", IconRoles, rolesView()),
	platformd.NewUIModule(ModuleUsers, "Usuarios", IconUsers, usersView()),
	platformd.NewUIModule(ModuleAudit, "Auditoría", IconAudit, auditView()),
}
dom.Append("body", p)
select {}
```

`adminIdentity` es un struct local en `web/client.go` que satisface
`platformd.Identity` (`UserName`, `UserAvatar` → `""`, `UserRoles` → `nil`).

## 3.2 — `web/client.go` (nuevo, `//go:build wasm`, < 80 líneas, solo `main`)

Flujo (calcá `misitio/web/client.go`):

1. `fetch.Get(api.PathAdminMe).Send(cb)`.
2. `resp.Status == 401` → no hacer nada (el HTML servido ya muestra el link de
   login; ver Etapa 4). `== 403` → mostrar en `#app` un texto
   "Tu cuenta no tiene acceso al panel de administración." y salir.
   `!= 200` → texto de error genérico.
3. `200` → `json.Decode` a `AdminMeResponse`, montar el `platformd.Platform`
   con los 4 módulos, `dom.Append("body", p)`.
4. `select {}` al final (mantiene viva la goroutine WASM — el comentario que
   ya usa `misitio/web/client.go`).

`web/client.go` importa `github.com/veltylabs/iam/modules/projects` (y roles,
users, audit) y `github.com/veltylabs/iam/api` (constantes de ruta + los
structs `Admin*Request/Response`, que la Etapa 2 ya puso en `api/admin.go`
justamente para esto). `web/client.go` **no** importa `routes` ni `config`.

## 3.3 — Constantes compartidas del panel: `modules/panelconst/panelconst.go`

```go
package panelconst

import "github.com/tinywasm/svg"

const (
	ModuleProjects = "projects"
	ModuleRoles    = "roles"
	ModuleUsers    = "users"
	ModuleAudit    = "audit"
)

const (
	IconProjects = svg.Icon("iam-projects")
	IconRoles    = svg.Icon("iam-roles")
	IconUsers    = svg.Icon("iam-users")
	IconAudit    = svg.Icon("iam-audit")
)

// IDs de nodos DOM que los módulos rellenan tras el fetch.
const (
	IDProjectsList = "iam-projects-list"
	IDProjectsNew  = "iam-projects-new"
	IDRolesProject = "iam-roles-project-select"
	IDRolesList    = "iam-roles-list"
	IDRolesNew     = "iam-roles-new"
	IDUsersForm    = "iam-users-form"
	IDUsersList    = "iam-users-list"
	IDAuditList    = "iam-audit-list"
)
```

## 3.4 — Módulos (uno por carpeta, `//go:build wasm`)

Cada carpeta: `<name>.go` (vista + `OnMount` + fetch/wire), `css.go`
(`//go:build !wasm`, `//go:embed <name>.css` + `RenderCSS`), `svg.go`
(`//go:build !wasm`, `IconSvg()` con el icono del módulo — `fill="currentColor"`,
sin `<svg>` envolvente), `<name>.css`.

### `modules/projects/`

- Vista: título + `<div id=IDProjectsList>` + `<div id=IDProjectsNew>` con un
  `form.New` de `{Name string}` (`SubmitLabel("Crear proyecto")`).
- `OnMount`/wire: `fetch.Get(api.PathAdminProjects)` → por cada proyecto, una
  fila con nombre, estado (activo/inactivo), fecha, y botones **Regenerar
  secreto** (POST `PathAdminProjectRotate`) y **Activar/Desactivar** (POST
  `PathAdminProjectActive`).
- Al crear o regenerar: la respuesta trae `client_secret` **en claro**.
  Mostralo en un bloque copiable **destacado** con la advertencia "Se muestra
  una sola vez. Copialo ahora; después solo se puede regenerar." No lo
  vuelvas a pedir ni lo caches.
- Tras cualquier mutación exitosa: re-fetch de la lista (remount del módulo o
  recarga del `<div>`).

### `modules/roles/`

- Selector de proyecto arriba (`<select id=IDRolesProject>`), poblado con
  `fetch.Get(api.PathAdminProjects)`.
- Al elegir proyecto: `fetch.Get(api.PathAdminRoles + "?project_id=" + pid)` →
  lista de `RoleView` con: código, nombre, `SessionTTL` (editable inline →
  POST `PathAdminRoleTTL`), `UserCount`, botón **Borrar** (POST
  `PathAdminRoleDelete`, con confirmación).
- `<div id=IDRolesNew>`: `form.New` de
  `{Code, Name, Description string}` → POST `PathAdminRoles`.

### `modules/users/`

- `form.New` de `{ProjectId, Code, Email string}` (o dos selects + un input
  email) → POST `PathAdminUserAssign`. Mostrá el `sub` devuelto.
- Debajo: dado proyecto+rol, `fetch.Get(api.PathAdminRoleUsers + "?project_id=&code=")`
  → lista de `RoleUser` (email, nombre) con botón **Revocar** (POST
  `PathAdminUserRevoke`).

### `modules/audit/`

- `fetch.Get(api.PathAdminAudit)` → tabla read-only: fecha (formateá
  `CreatedAt` epoch-segundos con `tinywasm/time`), actor, acción, objetivo,
  detalle. Sin acciones.

## 3.5 — Estilo

- CSS por módulo, prefijo = nombre del módulo (`iam-projects-*`), solo tokens
  del tema. El bloque destacado del secreto: fondo `var(--color-selection)`,
  texto monoespaciado, `padding: var(--mag-pri)`.
- Skin global de formularios: en `web/client.go`, import en blanco
  `_ "github.com/tinywasm/components/fieldset"` (igual que el ejemplo de
  `platformd/web/client.go`) para que todos los `form.New` se pinten
  consistentes.
- **Nada de `RootCSS()`** en los módulos (regla del skill `components`): los
  tokens `:root` los aporta el tema global.

## Criterios de aceptación (Etapa 3)

- `go build -tags wasm ./web/... ./modules/...` compila (o el flujo del daemon
  MCP compila `client.wasm` sin errores).
- `grep -rn "front.go" modules/` → vacío.
- Cada carpeta de `modules/*` tiene su `css.go` y `svg.go` con
  `//go:build !wasm`, y ningún `.css`/SVG string aparece en un archivo sin ese
  tag.
- `grep -rn "Attr(\"class\"\|Class(\"" modules/` → solo clases con prefijo de
  módulo definidas en el `.css` correspondiente; ninguna clase "suelta" tipo
  `btn btn-primary`.
- `web/client.go` no importa `github.com/veltylabs/iam/routes` ni
  `github.com/veltylabs/iam/config`.
- Verificá a mano (daemon MCP / `go build`) que `edge.wasm` **no** creció:
  `modules/*` y `tinywasm/layout` no aparecen en su grafo de imports.

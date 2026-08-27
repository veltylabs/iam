← [Etapa 3](PLAN_STAGE_3_ROUTES_ENTRY.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 5](PLAN_STAGE_5_TESTS.md)

# Etapa 4 — `modules/panel/` (WASM) + `web/client.go` + `web/public/`

`modules/panel/` es **un solo paquete**, `//go:build wasm` (salvo `svg.go`,
`!wasm`), archivos **planos**, y **`routes/` NUNCA lo importa** — esa es la
frontera del Worker. Compila a `web/public/client.wasm` desde `web/client.go`.
Sin límite de plataforma (`AGENTS.md` Restricción #4): puede importar
`tinywasm/layout`, `dom`, `html`, `form`, `svg`.

Seguí el skill `components`: embeber `dom.Element` por valor; SSR split (SVG en
`!wasm`); sin `front.go`; interactividad en `OnMount()`; nunca clases CSS
sueltas; tokens `var(--color-*)`/`var(--mag-*)` sin fallback; iconos por sprite.

```
modules/panel/
├── panel.go     # Boot(me config.AdminMeResponse): arma el platformd.Platform con los 4 UIModule
├── view.go      # LAS VISTAS: projectsView(), rolesView(), usersView(), auditView()
│                #   — una func por área, TODAS acá. Solo construyen el marcado
│                #   (contenedores + IDs de constants.go). El cableado va aparte.
├── projects.go  # cableado interactivo de Proyectos (fetch + form + OnMount)
├── roles.go     # idem Roles
├── users.go     # idem Usuarios
├── audit.go     # idem Auditoría (solo lectura)
├── support.go   # ShowStatus, Hide, Show…
├── constants.go # IDs de nodos DOM + identificadores/labels de módulo
└── svg.go       # //go:build !wasm — sprite de iconos
```

> **Por qué las vistas van juntas en `view.go`:** es la convención de
> `veltylabs/mjosefa-cms/modules/<m>/view.go` y de lo que
> `misitio/docs/PLAN_ARQUITECTURA_MODULOS.md` mueve a `modules/panel/view.go`
> (todas las `buildXxxView`). El *cableado* específico de un área sí va en su
> archivo (`projects.go`, …), igual que `misitio` tiene `modules/panel/admin.go`
> e `images.go`.

`modules/panel/` importa `github.com/veltylabs/iam/config` (constantes de ruta
+ DTOs de cable) y `tinywasm/*`. **No importa `github.com/veltylabs/iam/routes`
ni `.../modules/admin`.**

## 4.1 — `constants.go`

```go
package panel

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
const (
	IDProjectsList   = "iam-projects-list"
	IDProjectsNew    = "iam-projects-new"
	IDProjectsSecret = "iam-projects-secret" // bloque destacado del secreto
	IDRolesProject   = "iam-roles-project"
	IDRolesList      = "iam-roles-list"
	IDRolesNew       = "iam-roles-new"
	IDUsersForm      = "iam-users-form"
	IDUsersList      = "iam-users-list"
	IDAuditList      = "iam-audit-list"
)
```

## 4.2 — `view.go` — construcción de las 4 vistas

Cada `xxxView() dom.Component` arma solo el árbol de nodos con los `ID*` de
`constants.go` (títulos, contenedores vacíos, y los `form.New` de alta). El
`OnMount` del componente raíz llama a la función de cableado de su archivo
(`wireProjects()`, etc.). Calcá el estilo de
`https://github.com/veltylabs/misitio/blob/main/config/panel/modules.go`
(`buildXxxView` → `html.Div().Class("panel-module-view").Child(...)`).

```go
func projectsView() dom.Component {
	return html.Div().Class("iam-panel-view").Child(
		html.H2().Text("Proyectos"),
		html.Div().ID(IDProjectsSecret), // vacío hasta que un alta/rotación lo llene
		html.Div().ID(IDProjectsNew),    // form de alta {Name}
		html.Div().ID(IDProjectsList),
	) // + OnMount → wireProjects()
}
// rolesView(), usersView(), auditView() análogas.
```

## 4.3 — `projects.go` / `roles.go` / `users.go` / `audit.go` — cableado

`tinywasm/fetch` (`fetch.Get(config.PathAdmin*)`, `fetch.Post(...).ContentTypeJSON().Body(b)`),
`tinywasm/json` (`json.Encode`/`json.Decode` sobre los DTOs de `config/`),
`tinywasm/form` (`form.New(IDProjectsNew, &config.AdminCreateProjectRequest{}, idGen)`).

- **projects.go** — `wireProjects()`: `GET PathAdminProjects` → por proyecto,
  fila (nombre, estado activo/inactivo, fecha) + botones *Regenerar secreto*
  (`POST PathAdminProjectRotate`) y *Activar/Desactivar* (`POST PathAdminProjectActive`).
  Form de alta `{Name}` → `POST PathAdminProjects`. Al crear/rotar, la
  respuesta (`config.AdminSecretResponse`) trae `ClientSecret` en claro:
  renderizalo en `#iam-projects-secret`, destacado, monoespaciado, con "Se
  muestra una sola vez. Copialo ahora." No lo caches. Tras cualquier mutación
  OK: re-fetch de la lista.
- **roles.go** — `wireRoles()`: `<select id=iam-roles-project>` poblado con
  `GET PathAdminProjects`. Al elegir: `GET PathAdminRoles?project_id=` → lista
  de `config.RoleView` (código, nombre, `SessionTtl` editable inline → `POST
  PathAdminRoleTTL`, `UserCount`, botón *Borrar* → `POST PathAdminRoleDelete`
  con confirmación). Form `{Code, Name, Description}` → `POST PathAdminRoles`.
- **users.go** — `wireUsers()`: form `{ProjectId, Code, Email}` → `POST
  PathAdminUserAssign` (mostrar el `sub`). Debajo: `GET
  PathAdminRoleUsers?project_id=&code=` → lista con botón *Revocar* → `POST
  PathAdminUserRevoke`.
- **audit.go** — `wireAudit()`: `GET PathAdminAudit` → tabla solo lectura:
  fecha (`CreatedAt` epoch-seg con `tinywasm/time`), actor, acción, objetivo,
  detalle.

Skin global de formularios: en `web/client.go`, import en blanco
`_ "github.com/tinywasm/components/fieldset"`. **Nada de `RootCSS()`** en
`modules/panel/`.

## 4.4 — `panel.go`

Calcá `github.com/tinywasm/layout/platformd/web/client.go`.

```go
func Boot(me config.AdminMeResponse) {
	p := &platformd.Platform{
		AppName:   "iam · administración",
		DefaultID: ModuleProjects,
		User:      adminIdentity{name: me.Name},
		CanView:   func(id string) bool { return true }, // el gate real es server-side
	}
	p.Modules = []platformd.UIModule{
		platformd.NewUIModule(ModuleProjects, "Proyectos", IconProjects, projectsView()),
		platformd.NewUIModule(ModuleRoles, "Roles", IconRoles, rolesView()),
		platformd.NewUIModule(ModuleUsers, "Usuarios", IconUsers, usersView()),
		platformd.NewUIModule(ModuleAudit, "Auditoría", IconAudit, auditView()),
	}
	dom.Append("body", p)
}
```

`adminIdentity` (struct local) satisface `platformd.Identity` (`UserName`→`name`,
`UserAvatar`→`""`, `UserRoles`→`nil`).

## 4.5 — `svg.go` (`//go:build !wasm`)

Registra el sprite con los 4 iconos (`iam-projects`, `iam-roles`, `iam-users`,
`iam-audit`), `fill="currentColor"`, sin `<svg>` envolvente. Calcá
`config/svg.go` de este repo (ya construye un sprite).

## 4.6 — `web/client.go` (nuevo, `//go:build wasm`, solo `main`, < 80 líneas)

Calcá `misitio/web/client.go`.

```go
//go:build wasm
package main

import (
	_ "github.com/tinywasm/components/fieldset"
	"github.com/tinywasm/fetch"
	"github.com/tinywasm/json"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/panel"
)

func main() {
	fetch.Get(config.PathAdminMe).Send(func(resp *fetch.Response, err error) {
		if err != nil { panel.ShowStatus("No se pudo contactar al servidor."); return }
		switch resp.Status {
		case 401:
			return // el HTML servido ya muestra el enlace de login
		case 403:
			panel.ShowStatus("Tu cuenta no tiene acceso al panel de administración.")
		case 200:
			var me config.AdminMeResponse
			if err := json.Decode(resp.Body, &me); err != nil {
				panel.ShowStatus("Respuesta ilegible del servidor."); return
			}
			panel.Boot(me)
		default:
			panel.ShowStatus("El servidor respondió un estado inesperado.")
		}
	})
	select {}
}
```

`web/client.go` **no** importa `routes` ni `modules/admin`.

## 4.7 — `web/public/` (nuevo)

Calcá `https://github.com/veltylabs/misitio/tree/main/web/public`.

- `index.html`: `<div id="app"></div>`, un bloque `id="login"` **visible** con
  `<a href="/oauth/google?redirect_uri=<origin>">Entrar con Google</a>` (el
  WASM lo oculta al recibir `200` en `/admin/api/me`; con `401` queda visible
  — el login funciona sin WASM), y el `<script>` de `wasm_exec.js` +
  `client.wasm` (copialo de `misitio/web/public/index.html`, cambiando textos).
- `favicon.svg`: una llave simple, `fill="currentColor"`.

`edge.Serve` sirve `web/public/` cuando existe (mismo mecanismo que `misitio`).
**Verificá** contra `misitio/edge/main.go` y su `deploy.yml`; si `goflare`
necesita config extra para los assets o para compilar `client.wasm` en CI,
replicá lo de `misitio`. Si `edge` no sirve estáticos solo, **PARÁ y REPORTÁ**.

`.gitignore`: `web/public/client.wasm` y `.build/` ignorados; `index.html`,
`favicon.svg`, `wasm_exec.js` se versionan (mirá `.gitignore` de `misitio`).

## Criterios (Etapa 4)

- Build WASM de `web/client.go` compila (daemon MCP o `GOOS=js GOARCH=wasm go build ./web/`).
- `grep -rn "veltylabs/iam/routes\|veltylabs/iam/modules/admin" modules/panel/ web/client.go` → vacío.
- **`grep -rln "func .*View() dom.Component\|func .*view() dom.Component" modules/panel/` → solo `view.go`.**
- `grep -rn "front.go" modules/panel/` → vacío; `svg.go` es `//go:build !wasm`.
- `grep -rn "Class(\"\|Attr(\"class\"" modules/panel/` → solo clases `iam-*` definidas en el `.css` del paquete.
- `find modules -mindepth 2 -type d` → vacío.
- A mano: `GOOS=js GOARCH=wasm go list -deps ./edge/` **no** contiene
  `veltylabs/iam/modules/panel` ni `tinywasm/layout`/`dom`/`html`/`form`.
- Levantando `web/server.go`: `GET /` sirve `index.html`; sin sesión muestra el
  enlace de login; `GET /admin/api/me` sin sesión → 401; `GET /api/health` → `{"ok":true}`.

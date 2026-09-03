# AGENTS.md — restricciones de `veltylabs/iam`

Guía obligatoria para cualquier agente (o persona) que trabaje en este repo.
Léela antes de escribir una línea de código o de documentación.

## Qué es

El servicio central de **identidad y RBAC** para todos los proyectos Velty.
Vive en un **Cloudflare Worker con panel de administración** (assets
estáticos + WASM); los datos viven en una **D1 propia**, multi-proyecto por
`project_id`. Ningún otro repo del ecosistema vuelve a montar
`authority.Module`/`rbac.Service` contra su propia base — llaman a este
servicio.

Visión e índice: [`README.md`](README.md).
Diseño: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Restricción #1 — este repo NO tiene lógica de identidad propia

`iam` es una **raíz de composición** sobre `tinywasm/user` +
`tinywasm/auth` + `tinywasm/rbac` — no reimplementa autenticación ni
autorización, las ensambla. Su contenido legítimo:

- `config/` — construcción del motor (auth+rbac), DTOs de cable, helpers de dominio. **Hoja: no importa nada de `veltylabs/iam/`.**
- `routes/` — `func Register(r router.Router, deps …)` en `routes/routes.go` (un solo archivo, una sola tabla).
- `modules/admin/*.go` — handlers server-side (`backend.go`, `handler.go`), sin subcarpetas.
- `modules/panel/*.go` (`//go:build wasm`) — frontend del panel de administración (`view.go` contiene todas las funciones `buildXxxView`/`xxxView`; interactividad en archivos por área). `routes/` **nunca** lo importa.
- `web/client.go` (`//go:build wasm`) — punto de entrada del panel (solo `main`).
- `web/server.go` (`//go:build !wasm`) — servidor de desarrollo local.
- `edge/main.go` (`//go:build wasm`) — punto de entrada del Worker.
- `docs/`, `tests/`.

**Nada más.** Si aparece una regla de negocio que no sea "identidad,
proyecto o rol", no vive aquí — la autorización fina de cada app
consumidora (ej. `SiteMember` de `misitio`) se queda en esa app.

## Restricción #2 — el harness: nunca recrear localmente un símbolo que falta

Doctrina completa: [`tinywasm/app/docs/CONSTRUCTION_HARNESS.md`](https://github.com/tinywasm/app/blob/main/docs/CONSTRUCTION_HARNESS.md)
— aplica a todo el ecosistema.

- **Si una librería no expone lo que este repo necesita, se detiene y se
  arregla upstream** (nueva versión publicada de esa librería), nunca con
  un workaround local. Ejemplo real: el botón de login necesita ser un
  `<a href>` sin JS (funciona antes de que cargue el WASM y con JS
  desactivado) — `tinywasm/components/actionbutton` hoy solo sabe
  renderizar `<button OnClick>`. La corrección es extender ese componente
  para soportar un modo `Href` (aditivo, no rompe a sus otros
  consumidores), no inventar un widget de botón dentro de `iam`.
- **Nunca clases CSS sueltas** (`Attr("class", "btn btn-primary")`). Todo
  estilo pasa por `tinywasm/widget/style` (`style.Interactive(style.Primary)`,
  tokens `style.SpaceN`/`style.RadiusX`/etc.) en un `css.go` `!wasm`, como
  hace cualquier componente de `tinywasm/components` o `tinywasm/layout`.
  Una clase que no tiene un `RenderCSS()` en algún lado no es un estilo, es
  una promesa rota.
- Si llegar a una decisión de este tipo requiere preguntar "¿puedo
  declarar esto localmente?", la respuesta por doctrina ya es no — se
  reporta el hueco y se arregla donde vive.

## Restricción #3 — TinyGo dentro del Worker

Calco de la regla del ecosistema (ver
[`misitio/AGENTS.md`](https://github.com/veltylabs/misitio/blob/main/AGENTS.md)
Restricción #4 — misma tabla, mismo alcance):

| Regla | Detalle |
|---|---|
| **Sin mapas** | Prohibido `map[K]V`. Slices + búsqueda lineal, o structs de campos fijos. |
| **Sin stdlib pesada** | Nada de `fmt`, `errors`, `strconv`, `strings`, `log`, **`os`**. Usa `tinywasm/fmt`. Variables de entorno/config: `tinywasm/env` vía un `Reader`/`Writer` inyectado (agnóstico), nunca `os.Getenv` directo — ver `docs/ARCHITECTURE.md`. |
| **`context` de tinywasm** | `tinywasm/context`, no el de la stdlib. |
| **`error` sí, `errors` no** | Devolver `error` está bien; construirlo con `errors.New` no. |
| **JSON sin reflexión** | `tinywasm/json`, nunca `encoding/json`. |
| **Sin `reflect`** | En ninguna forma, ni transitiva. |

**Alcance:** aplica a lo que compila TinyGo **para el Worker** (`edge/main.go`
y lo que importa transitivamente — hoy eso incluye todo `config/`, porque
`web/server.go` y `edge/main.go` comparten esa capa). `tests/` compila con
Go estándar y no está sujeto a esto — no le "arregles" los imports.

## Restricción #4 — tamaño del binario, sin test automatizado

Dos WASM, como en `misitio`: `edge.wasm` (`edge/main.go`, límite duro de
Cloudflare, **< 1 MB**) y `client.wasm` (`web/client.go`, panel, sin límite
de plataforma). **Decisión explícita del mantenedor: el tamaño se revisa a
mano, no con un test automatizado** — un test de presupuesto ralentiza el
flujo de trabajo más de lo que aporta en esta etapa. Esto es lo contrario
de lo que hace `misitio` (que sí lo automatiza) — es una diferencia de
convención consciente entre los dos repos, no un descuido de `iam`.

## Restricción #5 — una sola base, multi-proyecto por `project_id`

Una sola D1 para todo `iam`. El aislamiento entre proyectos (`misitio`,
`mjosefa-cms`, futuros) es `project_id`, siguiendo el patrón canónico ya
establecido en
[`veltylabs/modules/AGENTS.md`](https://github.com/veltylabs/site_manager/blob/main/AGENTS.md)
§"Multi-tenancy": cada tabla lleva `project_id NotNull`, cada condición de
`UPDATE`/`DELETE` lo incluye. Esto se implementa modificando
`tinywasm/rbac` (breaking, `Role`/`Permission`/`UserRole`/`RolePermission`
ganan `project_id` nativo) — no una partición paralela dentro de `iam` que
esquive tocar la librería compartida. Pendiente: Etapa 2, sin ejecutar
todavía.

## Dev loop

Se desarrolla con el **daemon MCP de tinywasm**, igual que `misitio`
(recompila al guardar; verificar con `app_get_logs`,
`browser_get_console`, `browser_get_errors`) — decisión explícita del
mantenedor, aplicada desde ya aunque el panel hoy sea mínimo, para no
tener que introducir el flujo después.

Variables de entorno propias (además de `GOOGLE_*` y `JWT_SECRET`):
`IAM_ADMIN_EMAILS` (quién entra al panel) e `IAM_PANEL_ORIGIN` (origen
exacto del panel — sin ella el Worker no arranca; en local vale
`http://localhost:8080`). Detalle en `docs/ARCHITECTURE.md` §8.7 y
`docs/DEPLOY.md`.

## No hagas

- **No recrees localmente un símbolo que falta en una librería** — ver
  Restricción #2. Repórtalo/arréglalo upstream.
- **No uses carpetas `internal/`.**
- **No versiones los documentos** (nada de `v1`/`v2` dentro de los archivos).
- **No enlaces `PLAN.md`/`LAST_PLAN_EXECUTED.md` desde un documento permanente.**
- **No inventes decisiones.** Toda duda se pregunta antes de escribir código.

## Convención de idioma

- **Código en inglés**: structs, campos, funciones, paquetes, identificadores.
- **Todo lo demás en español**: documentación, comentarios de prosa.

## Estructura y pruebas

- Jerarquía plana; archivos de más de 500 líneas se subdividen por dominio.
- Todos los tests bajo `tests/`, consumiendo los paquetes reales.
- Fronteras externas (D1, Cloudflare) se prueban con fakes en memoria:
  `orm.New(mem.New())`.

## Stack

| Librería | Rol |
|---|---|
| [`tinywasm/user`](https://github.com/tinywasm/user) | Contrato estable `SubjectID`/`Subject`. |
| [`tinywasm/auth`](https://github.com/tinywasm/auth) | Autenticación, sesión, OAuth2 Google, escenarios locales. |
| [`tinywasm/rbac`](https://github.com/tinywasm/rbac) | Roles, permisos, `Can` — con `project_id` nativo desde la Etapa 2. |
| [`tinywasm/env`](https://github.com/tinywasm/env) | Config/variables de entorno, agnóstico (`Reader`/`Writer` inyectado). |
| [`tinywasm/goflare`](https://github.com/tinywasm/goflare) | Build y deploy a Cloudflare, runtime `edge`, binding D1. |
| [`tinywasm/router`](https://github.com/tinywasm/router) | Contrato de transporte; rutas privadas por defecto. |
| [`tinywasm/orm`](https://github.com/tinywasm/orm) | Persistencia sobre D1. |
| [`tinywasm/layout/login`](https://github.com/tinywasm/layout) | Pantalla previa a la sesión. |
| [`tinywasm/layout/platformd`](https://github.com/tinywasm/layout) | Chasis del panel de administración. |
| [`tinywasm/form`](https://github.com/tinywasm/form) | Generación y manejo de formularios en el panel. |
| [`tinywasm/components/actionbutton`](https://github.com/tinywasm/components) | Botones (variantes primary/secondary/danger); gana modo `Href` para el login. |
| [`tinywasm/json`](https://github.com/tinywasm/json) | Transporte tipado sin reflexión. |
| [`tinywasm/fmt`](https://github.com/tinywasm/fmt) | Reemplazo de `fmt`/`errors`/`strings`. |

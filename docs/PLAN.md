---
PLAN: "feat: panel de administración de iam — proyectos, roles, usuarios y auditoría, con acceso por IAM_ADMIN_EMAILS"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 14708373474994269570
PR: https://github.com/veltylabs/iam/pull/6
---

> Este plan se despacha con el flujo CodeJob. Ver skill: `agents-workflow`.
> No ejecutes `gopush` ni `codejob` — son herramientas del desarrollador local.

# PLAN — Panel de administración de `veltylabs/iam`

Si te pidieron "ejecuta el plan descrito en `docs/PLAN.md`", ejecutá **todas
las etapas en orden**. Cada una tiene su archivo, es autocontenida, y termina
con criterios verificables. Terminá una antes de empezar la siguiente. Nunca
mezcles cambios de una etapa en otra.

## Contexto

`veltylabs/iam` es el servicio central de identidad + RBAC de Velty: un
**Cloudflare Worker con panel** (Go a WASM con TinyGo para el Worker, y a WASM
para el panel), D1 propia (`velty-iam-db`), multi-proyecto por `project_id`.
Los consumidores (hoy `veltylabs/misitio`) le piden tokens por HTTP con un
`client_secret`.

**Hoy no hay forma de registrar un proyecto ni emitir su `client_secret` sin
correr código a mano contra la D1** (`config.CreateProject` solo se llama desde
tests). Este plan construye el panel. `docs/ARCHITECTURE.md` §8 describe el
diseño acordado — **leé §8 completa antes de empezar**.

## Transporte: REST, sin MCP

El panel habla con el Worker por **HTTP/REST** — handlers `router.HandlerFunc`
montados en `routes/routes.go`, y `tinywasm/fetch` + `tinywasm/json` en el
navegador. **NADA de MCP, `router.OpRegistry`/`Caller`, `tinywasm/view` ni
`crudview` en este plan** — ese patrón todavía no se probó en un Worker de
Cloudflare y queda para una ola futura junto con `misitio`.

## Arquitectura de carpetas — REGLA DURA

Misma arquitectura de capas que `veltylabs/misitio`
(`https://github.com/veltylabs/misitio/blob/main/docs/PLAN_ARQUITECTURA_MODULOS.md`).
Una sola flecha de dependencias, sin ciclos:

```
edge/  web/server.go  ──►  routes/  ──►  modules/admin/  ──►  config/
                                                              ▲
                web/client.go  ──►  modules/panel/  ──────────┘
```

| Carpeta | Contenido | Build tag |
|---|---|---|
| `config/` | **Hoja: NO importa `github.com/veltylabs/iam/`.** Constantes de ruta `PathAdmin*`, DTOs de cable (con `EncodeFields`/`DecodeFields` a mano), helpers de dominio (crear/rotar/listar/desactivar proyecto, listar roles/usuarios, gate `IsPanelAdmin`, auditoría), modelos. Importa `tinywasm/*` (ya usa `orm`, `rbac`, `auth`, `model`). | sin tag (+ `!wasm` donde ya lo hay) |
| `modules/admin/` | El dominio "administración de iam". Archivos **planos**, nombres canónicos: `backend.go` (consultas que componen `tinywasm/rbac`/`authority`), `handler.go` (los handlers HTTP + el gate `RequirePanelAdmin`). **SIN subcarpetas.** Importa `config/` y `tinywasm/{router,orm,rbac,auth/authority,model}`; **nunca `routes/`**, **nunca `dom`/`html`/`form`/`layout`/`svg`**. Compila al Worker. | sin tag |
| `modules/panel/` | **Todo lo que toca un renderer.** `view.go` = la **construcción de las vistas** (`projectsView()`, `rolesView()`, `usersView()`, `auditView()` — una función por área, TODAS en este archivo). Archivos por área (`projects.go`, `roles.go`, `users.go`, `audit.go`) = el **cableado interactivo** (fetch + forms + handlers de evento en `OnMount`). `support.go`, `constants.go`, `svg.go` (`//go:build !wasm`). **`routes/` NUNCA lo importa** — esa ausencia de import es la frontera que mantiene `dom`/`layout` fuera de `edge.wasm`. | `//go:build wasm` (`svg.go` es `!wasm`) |
| `routes/routes.go` | **Una sola tabla, un solo archivo.** Los `r.Get/r.Post` de `/admin/api/*` llamando a los handlers de `modules/admin/`, cada uno envuelto en `admin.RequirePanelAdmin`. Nada más. | sin tag |
| `edge/`, `web/` | Solo transporte y arranque. | según entrada |

**Prohibido:** un paquete `api/` (se absorbe en `config/` y se borra —
Etapa 1); tocar `client/` (SDK público); subcarpetas dentro de un módulo; que
un archivo que `routes/` importa transitivamente traiga `dom`/`layout`;
**vistas repartidas fuera de `view.go`**; estilo `tinywasm/components`
(`<name>.go`+`<name>.css`+`ssr.go` por componente — eso es para la librería de
componentes, no para módulos de app).

## Reglas de código (`AGENTS.md`)

1. **`iam` no reimplementa identidad/RBAC.** Si falta algo en `tinywasm/*`,
   **PARÁS y REPORTÁS**, no lo recreás local. Excepción: leer filas con los
   **codecs ORM ya exportados** de `tinywasm/rbac` (`ReadAllRole`, `Role_`,
   `Role`, `ReadAllUserRole`, `UserRole_`) sobre el `*orm.DB` compartido es
   composición permitida — NO agregues métodos a `tinywasm/rbac`.
2. **TinyGo en el Worker** — aplica a `config/`, `routes/`, `modules/admin/`
   (todo lo que compila para `edge/main.go`), **NO** a `modules/panel/` ni a
   `tests/`:
   - Sin `map[K]V`. Slices + búsqueda lineal.
   - Sin stdlib pesada: `fmt`, `errors`, `strconv`, `strings`, `log`, `os`,
     `encoding/json`, `reflect`, `context` stdlib. Usá `tinywasm/fmt`
     (`fmt.Errf`, `fmt.HasPrefix`, `fmt.Split`, `fmt.TrimSpace`,
     `fmt.Contains`), `tinywasm/json`, `tinywasm/context`, `tinywasm/env`.
   - `error` de retorno sí; `errors.New` no.
3. **`modules/panel/`**: sin stdlib igual (`tinywasm/fmt`/`time`/`json`), pero
   **sí** puede importar `dom`/`html`/`form`/`layout`/`svg`. Sin `front.go`;
   interactividad en `OnMount()`. SSR split: SVG/CSS en archivos `!wasm`.
4. **Nunca clases CSS sueltas.** Tokens (`var(--color-*)`, `var(--mag-*)`) sin
   fallback, en `!wasm`. Ver skill `components`.
5. Jerarquía plana; > 500 líneas se parte por dominio. Tests bajo `tests/`
   con `orm.New(mem.New())`.
6. **Idioma:** identificadores y errores del código en inglés; prosa y docs
   en español.
7. No versiones docs (`v1`/`v2`). No enlaces `PLAN.md` desde un doc permanente.

## Prerrequisito (una vez, antes de la Etapa 1)

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
go install github.com/tinywasm/orm/cmd/ormc@latest
```

Usá `gotest` (no `go test`).

## Decisiones tomadas (no re-abrir)

- **Transporte:** REST. Sin MCP / `view` / `crudview`.
- **Acceso al panel:** variable `IAM_ADMIN_EMAILS` (lista por coma). NO un rol
  RBAC (circular). `admin.RequirePanelAdmin` envuelve cada handler. `DESIGN.md` §2.
- **Sin diferencia local/prod.** La verificación de `client_secret` en
  `/api/token` no cambia y no tiene bypass. La corrección se prueba con tests;
  el panel se levanta local solo para revisar lo visual. `DESIGN.md` §3.
- **`iam` no conoce a sus consumidores.** Sin semilla de proyectos de dev.
- **Alcance:** proyectos + ciclo de vida del `client_secret` + roles + usuarios
  + auditoría. Listo para producción.

## Etapas

| # | Archivo | Entrega |
|---|---|---|
| 1 | [PLAN_STAGE_1_CONFIG.md](PLAN_STAGE_1_CONFIG.md) | `config/` absorbe `api/`; modelos (`active`, `audit_entry`); constantes `PathAdmin*`; DTOs de cable; helpers (`GenerateClientSecret`, rotar/desactivar/listar, `IsPanelAdmin`, `RecordAudit`, `MigrateSchema`) |
| 2 | [PLAN_STAGE_2_MODULE_ADMIN.md](PLAN_STAGE_2_MODULE_ADMIN.md) | `modules/admin/`: `backend.go` (listar roles/usuarios vía codecs de `rbac`), `handler.go` (`RequirePanelAdmin` + los 13 handlers) |
| 3 | [PLAN_STAGE_3_ROUTES_ENTRY.md](PLAN_STAGE_3_ROUTES_ENTRY.md) | `routes/routes.go` una tabla; `edge/main.go` y `web/server.go` con la firma nueva de `Register` |
| 4 | [PLAN_STAGE_4_PANEL.md](PLAN_STAGE_4_PANEL.md) | `modules/panel/` (`wasm`): `view.go` (las 4 vistas), archivos por área (cableado), `svg.go`; `web/client.go`; `web/public/` |
| 5 | [PLAN_STAGE_5_TESTS.md](PLAN_STAGE_5_TESTS.md) | Tests de handlers + `tests/layering_test.go` |
| 6 | [PLAN_STAGE_6_DOCS.md](PLAN_STAGE_6_DOCS.md) | Verificar docs ↔ código; quitar notas `STATUS`; `README.md`, `AGENTS.md` |

Al terminar: `gotest ./...` verde; a mano `GOOS=js GOARCH=wasm go list -deps
./edge/...` **no** contiene `modules/panel` ni `tinywasm/layout`/`dom`/`html`/`form`.

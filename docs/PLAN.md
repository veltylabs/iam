---
PLAN: "feat: panel de administración de iam — proyectos, roles, usuarios y auditoría, con acceso por IAM_ADMIN_EMAILS"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha con el flujo CodeJob. Ver skill: `agents-workflow`.
> No ejecutes `gopush` ni `codejob` — son herramientas del desarrollador
> local, fuera de tu alcance.

# PLAN — Panel de administración de `veltylabs/iam`

Si te pidieron "ejecuta el plan descrito en `docs/PLAN.md`", ejecutá **todas
las etapas de abajo, en orden (de arriba hacia abajo)**. Cada etapa tiene su
propio archivo, es autocontenida, y termina con criterios de aceptación
verificables. Terminá una (todos sus criterios en verde) antes de empezar la
siguiente. Nunca mezcles cambios de una etapa en otra.

## Contexto — qué es este repo y qué se está construyendo

`veltylabs/iam` es el servicio central de identidad + RBAC de Velty: un
**Cloudflare Worker con panel de administración** (Go compilado a WASM con
TinyGo para el Worker, y a WASM para el panel), datos en una D1 propia
(`velty-iam-db`), multi-proyecto por `project_id`. Los proyectos consumidores
(hoy `veltylabs/misitio`) le piden tokens por HTTP con un `client_secret`.

**Hoy no existe forma de registrar un proyecto ni de emitir su
`client_secret` sin correr código a mano contra la D1.** `config.CreateProject`
solo se llama desde tests. Este plan construye el panel que faltaba:
`AGENTS.md` Restricción #1 ya lo lista como parte legítima del repo
(`modules/projects`, `modules/roles`, `modules/users`, `web/client.go`), y
`docs/ARCHITECTURE.md` §8 ya describe el diseño acordado. Leé esa sección
completa antes de empezar.

## Reglas del repo que DEBES respetar (resumen; fuente: `AGENTS.md`)

1. **`iam` no reimplementa identidad/RBAC** — compone sobre `tinywasm/user` +
   `tinywasm/auth` + `tinywasm/rbac`. Si una de esas librerías no expone algo
   que necesitás, **parás y lo reportás** (no lo recreás local). Excepción
   explícita en este plan: leer filas con los **codecs ORM ya exportados** de
   `tinywasm/rbac` (`ReadAllRole`, `Role_`, `Role`, `ReadAllUserRole`,
   `UserRole_`) sobre el `*orm.DB` compartido **es composición, no
   reimplementación** — está permitido y NO debés "arreglarlo" agregando
   métodos a `tinywasm/rbac`.
2. **TinyGo en el Worker** — todo lo que compila para `edge/main.go` (que hoy
   incluye **todo `config/`**, porque `web/server.go` y `edge/main.go`
   comparten esa capa):
   - Sin `map[K]V`. Slices + búsqueda lineal, o structs de campos fijos.
   - Sin stdlib pesada: nada de `fmt`, `errors`, `strconv`, `strings`,
     `log`, `os`, `encoding/json`, `reflect`, `context` de stdlib.
   - Usá `github.com/tinywasm/fmt` (`fmt.Errf`, `fmt.HasPrefix`, `fmt.Split`,
     `fmt.TrimSpace`, `fmt.Contains`), `github.com/tinywasm/json`,
     `github.com/tinywasm/context`.
   - `error` como valor de retorno sí; construirlo con `errors.New` no
     (`fmt.Errf`).
   - Variables de entorno: `github.com/tinywasm/env` (`env.Get`, `env.Arg`) —
     nunca `os.Getenv`.
3. **`tests/` compila con Go estándar y NO está sujeto a la regla 2.** No le
   "arregles" los imports. Ahí sí podés usar `testing`, `net/http/httptest`,
   `strings`, `reflect`.
4. **Nunca clases CSS sueltas.** Todo estilo pasa por
   `tinywasm/widget/style` / tokens (`var(--color-primary)`,
   `var(--mag-pri)`, …) en un archivo `//go:build !wasm` (`css.go`). Ver
   skill `components`.
5. **Jerarquía plana.** Archivos > 500 líneas se subdividen por dominio.
   Todos los tests bajo `tests/`, consumiendo los paquetes reales. Fronteras
   externas (D1) se prueban con `orm.New(mem.New())`.
6. **Idioma:** código, identificadores y mensajes de error del código en
   **inglés**; comentarios de prosa y docs en **español**.
7. **No versiones los documentos** (nada de `v1`/`v2` dentro de archivos). **No
   enlaces `PLAN.md` desde un documento permanente.**

## Prerrequisito (correr una sola vez, antes de la Etapa 1)

```bash
go install github.com/tinywasm/devflow/cmd/gotest@latest
go install github.com/tinywasm/orm/cmd/ormc@latest
```

`gotest` corre toda la suite (`-vet`, `-race`, `-cover`, tests WASM). Usalo
siempre en vez de `go test`.

## Decisiones ya tomadas (no las re-abras)

- **Acceso al panel:** variable `IAM_ADMIN_EMAILS` (lista separada por coma).
  NO se usa un rol RBAC para entrar (sería circular). Ver `DESIGN.md` §2.
- **Sin diferencia de comportamiento local/prod.** La verificación de
  `client_secret` en `/api/token` no cambia y no tiene bypass. La corrección
  se prueba con tests; el panel se levanta local solo para revisar lo visual.
  Ver `DESIGN.md` §3.
- **`iam` no conoce a sus consumidores por nombre.** Sin semilla de proyectos
  de desarrollo. Un test crea el proyecto que necesita con
  `config.CreateProject` contra memoria.
- **Alcance:** proyectos + ciclo de vida del `client_secret` + roles por
  proyecto + asignación de usuarios + log de auditoría. Todo, listo para
  producción.

## Etapas

| # | Archivo | Qué entrega |
|---|---|---|
| 1 | [PLAN_STAGE_1_BACKEND.md](PLAN_STAGE_1_BACKEND.md) | Modelos (`active` en `project`, `audit_log`), helpers de `config/` (regenerar/desactivar/listar proyectos, listar roles, gate `IsPanelAdmin`, `RecordAudit`, generación de secreto) |
| 2 | [PLAN_STAGE_2_ROUTES.md](PLAN_STAGE_2_ROUTES.md) | Rutas `/admin/api/*` gated, con structs de request/response tipados; `routes.Register` gana `adminEmails []string` |
| 3 | [PLAN_STAGE_3_PANEL_UI.md](PLAN_STAGE_3_PANEL_UI.md) | `web/client.go` + `modules/projects|roles|users|audit` (`//go:build wasm`), chasis `platformd`, SSR split, iconos |
| 4 | [PLAN_STAGE_4_SERVE.md](PLAN_STAGE_4_SERVE.md) | Servir el panel: `web/public/`, assets en `edge/` y `web/server.go`, `deploy.yml` |
| 5 | [PLAN_STAGE_5_TESTS.md](PLAN_STAGE_5_TESTS.md) | Tests de `tests/`: proyectos, roles, usuarios, gate de acceso, auditoría, vistas de módulo sin navegador |
| 6 | [PLAN_STAGE_6_DOCS.md](PLAN_STAGE_6_DOCS.md) | Verificar docs contra la implementación; quitar las notas `STATUS`; actualizar `README.md` |

Al terminar todas las etapas, corré `gotest ./...` una última vez: todo en
verde. Verificá a mano `go build ./edge/...` con TinyGo (o el flujo del daemon
MCP) para confirmar que `edge.wasm` sigue < 1 MB y que el panel **no** se
coló en él.

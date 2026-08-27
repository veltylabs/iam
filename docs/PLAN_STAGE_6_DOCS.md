← [Etapa 5](PLAN_STAGE_5_TESTS.md) | [PLAN.md](PLAN.md)

# Etapa 6 — Documentación

Los docs permanentes de `iam` **ya describen este diseño** (se escribieron
antes de implementar, con notas `> STATUS`). Tu trabajo: **verificar que el
código coincide** y **quitar las notas `STATUS`**. No reescribas el diseño; si
el código terminó difiriendo, el documento manda (corregí el código) salvo que
el diseño haya cambiado por una razón válida, en cuyo caso ajustá el texto a
lo que hay (sin historia, sin `v1`/`v2`).

## 6.1 — `docs/ARCHITECTURE.md` §8

- Quitá el bloque `> STATUS (quitar esta nota…)` del encabezado de §8.
- Verificá contra el código: §8.1 (mismo Worker sirve el panel; `web/public/`
  existe; `/api/token` sin cambios), §8.2 (`IAM_ADMIN_EMAILS`; default local
  `LocalScenarios[0].Email`), §8.3 (áreas/operaciones ↔ `config.PathAdmin*` +
  handlers de `modules/admin/handler.go`), §8.4 (reparto de código:
  `config/` hoja, `modules/admin/` planos, `modules/panel/` con las vistas en
  `view.go`, `routes/routes.go` una tabla, `api/` borrado), §8.5 (rutas),
  §8.6 (`audit_entry` en `cmd/migrate`; `project.active`).
- Si algún nombre del texto no existe tal cual en el código
  (`RegenerateProjectSecret`, `SetProjectActive`, `IsPanelAdmin`,
  `RequirePanelAdmin`, `MigrateSchema`), unificá — renombrá en el código para
  que coincida con el documento.

## 6.2 — `docs/DESIGN.md` §2 y §3

Quitá las notas `> STATUS`. Verificá §2 (`IAM_ADMIN_EMAILS`, no RBAC) y §3
(tests para lo funcional; `iam` no conoce consumidores; sin rama `if dev`):
`grep -rn "dev\|local" config/ modules/admin/` no debe mostrar ninguna rama
que salte o relaje la verificación de `client_secret`.

## 6.3 — `docs/DEPLOY.md`

- Ya tiene `IAM_ADMIN_EMAILS` (5 variables). Verificá.
- La intro dice "`iam` es un Worker API puro, sin `web/public`". Cambialo:
  ahora es un Worker **con panel** (assets + `client.wasm`), como `misitio`;
  ajustá la frase sobre "el deploy nunca toca el protocolo de subida de assets".
- Fila de diagnóstico "403 en `/api/token`": añadir "…o el proyecto fue
  desactivado desde el panel".

## 6.4 — `README.md`

Sección **Estado**: reemplazá "Pendiente: un panel de administración propio…"
por una frase que diga que el panel existe (proyectos + `client_secret` +
roles + usuarios + auditoría), servido por el mismo Worker, acceso por
`IAM_ADMIN_EMAILS`, transporte REST. Si hay "Uso desde otro proyecto", añadí:
el `client_secret` se emite y se rota **desde el panel** (`/` en
`iam.velty.cl`), no con un script.

## 6.5 — `AGENTS.md`

- Restricción #1: actualizá al reparto real — `config/` **no importa nada del
  repo**; un módulo es archivos **planos** (`backend.go`/`handler.go`/`model.go`/
  `view.go`, **sin subcarpetas**); las vistas del panel van en
  `modules/panel/view.go`; `modules/panel/` lo importa solo el navegador,
  `routes/` no; `routes/routes.go` es una sola tabla; `api/` ya no existe.
  Nombrá los módulos reales: `admin` (server), `panel` (wasm).
- Tabla **Stack**: si el panel usa `tinywasm/layout/platformd` y `tinywasm/form`,
  agregá esas filas.

## 6.6 — Sin enlaces a `PLAN.md`

`grep -rn "PLAN.md\|PLAN_STAGE" README.md AGENTS.md docs/ARCHITECTURE.md docs/DESIGN.md docs/DEPLOY.md`
→ **vacío**.

## Criterios (Etapa 6)

- `grep -rn "STATUS (quitar esta nota" docs/` → vacío.
- `grep -rn "sin diseñar todavía\|Worker API puro" docs/` → vacío.
- Ningún doc permanente enlaza `PLAN.md`/`PLAN_STAGE_*`.
- `gotest ./...` verde tras cualquier renombre por consistencia doc↔código.
- `README.md` describe el panel como algo que existe.

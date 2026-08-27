← [Etapa 5](PLAN_STAGE_5_TESTS.md) | [PLAN.md](PLAN.md)

# Etapa 6 — Documentación

Los documentos permanentes de `iam` **ya describen este diseño** (se
escribieron antes de la implementación). Tu trabajo acá es **verificar que la
implementación coincide** y **quitar las notas `STATUS`**. No reescribas el
diseño; si el código terminó difiriendo del documento, el documento manda —
corregí el código o, si el diseño cambió por una razón válida, ajustá el
texto para que describa lo que hay (sin historia, sin `v1`/`v2`).

## 6.1 — `docs/ARCHITECTURE.md` §8

- Quitá el bloque `> STATUS (quitar esta nota cuando el panel esté
  implementado y publicado): …` del encabezado de §8.
- Verificá punto por punto contra el código:
  - §8.1 — el panel lo sirve el mismo Worker; `web/public/` existe; la
    verificación de `/api/token` no cambió.
  - §8.2 — `IAM_ADMIN_EMAILS`; default local = `config.LocalScenarios[0].Email`.
  - §8.3 — la tabla de módulos/operaciones coincide con las rutas de
    `api/api.go` y los handlers de `routes/admin*.go`.
  - §8.4 — rutas: `/`, `/assets/*` (o la ruta real de assets), `/admin/api/*`,
    y `/api/*` `/oauth/*` `/logout` intactas.
  - §8.5 — `audit_log` en `cmd/migrate`; `project.active`.
- Si algún nombre de símbolo del texto no existe tal cual en el código
  (`RegenerateProjectSecret`, `SetProjectActive`, `IsPanelAdmin`), unificá:
  el que se llame distinto en el código se renombra en el código para que
  coincida con el documento, o al revés si el del código es claramente mejor.

## 6.2 — `docs/DESIGN.md` §2 y §3

- Quitá las notas `> STATUS (…)`.
- Verificá que §2 (acceso por `IAM_ADMIN_EMAILS`, no RBAC) y §3 (tests para lo
  funcional, manual solo para lo visual; `iam` no conoce consumidores; sin
  rama `if dev`) describen lo implementado. En particular: `grep -rn "dev\|local"`
  en `config/` y `routes/` no debe encontrar ninguna rama que salte o relaje
  la verificación de `client_secret`.

## 6.3 — `docs/DEPLOY.md`

- Ya tiene `IAM_ADMIN_EMAILS` en la tabla de variables de ejecución y el
  conteo actualizado a 5. Verificá que sea correcto.
- En la intro dice "`iam` es un Worker API puro, sin `web/public`". Cambialo:
  ahora es un Worker **con panel** (assets estáticos + `client.wasm`), como
  `misitio`. Ajustá la frase sobre "el deploy nunca toca el protocolo de
  subida de assets".
- La fila de diagnóstico "Un consumidor recibe 403 en `/api/token`" ahora
  puede añadir: "…o el proyecto fue desactivado desde el panel".

## 6.4 — `README.md`

- Sección **Estado**: reemplazá "Pendiente: un panel de administración propio
  para `iam` — ver `docs/ARCHITECTURE.md` §8." por una frase que diga que el
  panel existe (proyectos + `client_secret` + roles + usuarios + auditoría),
  servido por el mismo Worker, con acceso por `IAM_ADMIN_EMAILS`.
- Si hay un bloque "Uso desde otro proyecto", añadí una línea: el
  `client_secret` se emite y se rota **desde el panel** (`/` en
  `iam.velty.cl`), ya no con un script.

## 6.5 — `AGENTS.md`

- Restricción #1 ya lista `modules/<módulo>` y `web/client.go` como contenido
  legítimo — verificá que los nombres de módulo del ejemplo (`projects`,
  `roles`, `users`) coinciden con las carpetas creadas (más `audit`). Añadí
  `audit` a esa enumeración si corresponde.
- La tabla **Stack** menciona `tinywasm/layout/login` y
  `tinywasm/components/actionbutton`. Si el panel terminó usando
  `tinywasm/layout/platformd` y `tinywasm/form`, agregá esas filas.

## 6.6 — Nada de enlaces a `PLAN.md`

`grep -rn "PLAN.md\|PLAN_STAGE" README.md AGENTS.md docs/ARCHITECTURE.md docs/DESIGN.md docs/DEPLOY.md`
→ **vacío**. Los planes se borran al cerrar el ciclo; un documento permanente
no los cita.

## Criterios de aceptación (Etapa 6)

- `grep -rn "STATUS (quitar esta nota" docs/` → vacío.
- `grep -rn "sin diseñar todavía\|Worker API puro" docs/` → vacío.
- Ningún documento permanente enlaza `PLAN.md`/`PLAN_STAGE_*`.
- `gotest ./...` sigue verde tras cualquier renombre hecho por consistencia
  documento↔código.
- `README.md` describe el panel como algo que existe, no como pendiente.

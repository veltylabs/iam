---
PLAN: "feat: bootstrap identity and RBAC engine ported from misitio"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.

# Plan — `veltylabs/iam`, motor de identidad + RBAC

Este es el **orquestador maestro**. Lee primero
[`docs/ARCHITECTURE.md`](ARCHITECTURE.md) — explica por qué existe este
repo, qué NO es, y el límite de responsabilidad frente a
`veltylabs/site_manager`. Es contexto necesario para no reintroducir
acoplamiento a `site_manager` en este repo.

## Resumen de decisiones ya tomadas

(Detalle y justificación completa en `docs/ARCHITECTURE.md` §3.)

1. Nombre: `github.com/veltylabs/iam`.
2. SSO real entre subdominios `*.velty.cl` (mecanismo exacto: Etapa 4).
3. Roles con ámbito por proyecto (`project_id`); partición física del
   esquema sin decidir todavía (Etapa 2).
4. `iam` resuelve identidad + rol de alto nivel por proyecto. La
   autorización fina de negocio (`SiteMember` y similares) se queda en cada
   app consumidora — `iam` no la reemplaza.

## Etapas

| # | Archivo | Qué hace | Estado |
|---|---|---|---|
| 1 | [PLAN_STAGE_1_PORT_IDENTITY.md](PLAN_STAGE_1_PORT_IDENTITY.md) | Portar el motor de identidad+RBAC (Google OAuth, rbac genérico, escenarios locales, tests, diagrama) desde `veltylabs/misitio`, sin ninguna dependencia de `site_manager`/`site_content` | **listo para ejecutar** |
| 2 | *(sin archivo aún)* | Particionar el motor por `project_id` — requiere decidir columna compartida vs. instancia lógica por proyecto antes de escribirse | pendiente de Q&A |
| 3 | *(sin archivo aún)* | Exponer la API HTTP de `iam` con autenticación Bearer para consumidores remotos — parte de la base ya sentada en [`tinywasm/mcp` `PLAN_bearer_auth_pending.md`](https://github.com/tinywasm/mcp/blob/main/docs/PLAN_bearer_auth_pending.md) | pendiente de Q&A |
| 4 | *(sin archivo aún)* | Sesión SSO real entre subdominios `*.velty.cl` (cookie de dominio padre vs. JWT) | pendiente de Q&A |

> **Si te dijeron "ejecuta el plan descrito en docs/PLAN.md": ejecuta
> únicamente la Etapa 1** — es la única con archivo de etapa completo. Las
> etapas 2-4 se detallan en rondas de preguntas separadas antes de tener su
> propio `PLAN_STAGE_N.md`; no las improvises a partir de esta tabla.

## Fuera de alcance en este repo

- Actualizar `veltylabs/misitio` para que consuma `iam` remotamente en vez
  de montar `authority.Module`/`rbac.Service` localmente. Es un plan
  distinto, dentro del repo `veltylabs/misitio` (tiene su propio
  `docs/PLAN.md`), que dependerá de que la Etapa 3 de este repo exista
  primero.
- Migrar `veltylabs/mjosefa-cms`. Mencionado como motivación en
  `docs/ARCHITECTURE.md` §1, no como trabajo de este plan.
- Cualquier cambio a `tinywasm/user`, `tinywasm/auth` o `tinywasm/rbac`. Si
  la Etapa 1 revela que falta algo en esas librerías, la reacción correcta
  es señalarlo, no copiar la función localmente — ver
  [`ARCHITECTURE.md`](ARCHITECTURE.md) §2.

## Después de la Etapa 1: `go test ./...` limpio, cero referencias cruzadas

Criterio de cierre de todo lo dispatchable hoy (repetido con más detalle en
el archivo de etapa): `go test ./...` en verde, y
`grep -rln "site_manager\|site_content\|SiteMember\|AccessRequest" config/ tests/`
sin resultados. Si ese grep encuentra algo, la Etapa 1 se coló hacia
lógica de negocio de `misitio` y hay que retirarlo.

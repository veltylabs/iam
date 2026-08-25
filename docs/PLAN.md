---
PLAN: "feat: bearer API tokens (project-scoped) and SSO cookie across *.velty.cl"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha vía el flujo CodeJob. Ver skill: agents-workflow.

# Plan — `veltylabs/iam`, API Bearer y SSO cross-dominio

Este es el **orquestador maestro**. Lee primero
[`docs/ARCHITECTURE.md`](ARCHITECTURE.md) §6-8 — documenta por qué son
**dos tokens distintos** (identidad vs. autorización), por qué el payload
de roles no se agrega a `tinyjwt.Claims`, y las decisiones de TTL/dominio
ya tomadas con el mantenedor.

## Resumen de decisiones ya tomadas (2026-08-24)

(Detalle y justificación completa en `docs/ARCHITECTURE.md` §6-7.)

1. Dos tokens: uno de **identidad** (cookie `.velty.cl`, solo `Sub`, TTL 7
   días) y uno de **autorización** (project-scoped, `ProjectID`+`Roles`
   embebidos, TTL 30 min por defecto — diferenciado por rol vía
   `rbac.Role.SessionTTL`).
2. El payload de autorización (`AuthClaims`) vive en `veltylabs/iam`, no en
   `tinywasm/auth` — `auth` nunca importa `rbac`. `tinywasm/jwt` gana
   `SignPayload`/`VerifyPayload` genéricos, aditivos, sin tocar `Claims`.
3. Sin revocación explícita — TTL corto + renovación automática.
4. Cada proyecto consumidor se autentica ante `iam` con `client_id`/
   `client_secret` propio (bcrypt, mismo mecanismo que
   `tinywasm/auth/email_password`) — la cookie de identidad prueba quién es
   el usuario, no qué app la reenvía.
5. SSO real: cookie de dominio padre `.velty.cl` (no JWT portado por
   redirect). `iam` vive en `iam.velty.cl`.

## Etapas

| # | Archivo | Qué hace | Estado |
|---|---|---|---|
| 1 | [PLAN_STAGE_1_PORT_IDENTITY.md](PLAN_STAGE_1_PORT_IDENTITY.md) | Motor de identidad+RBAC portado desde `misitio` | **ejecutada** (ver `docs/LAST_PLAN_EXECUTED.md`) |
| 2 | *(sin archivo — se ejecutó junto a la limpieza de auth/rbac)* | `project_id` nativo en `tinywasm/rbac` | **ejecutada** (ver `ARCHITECTURE.md` §5) |
| 3 | [PLAN_STAGE_3_BEARER_API.md](PLAN_STAGE_3_BEARER_API.md) | API HTTP: token de autorización project-scoped, primeras rutas de `iam` | **listo para ejecutar** |
| 4 | [PLAN_STAGE_4_SSO_COOKIE.md](PLAN_STAGE_4_SSO_COOKIE.md) | Cookie de identidad compartida entre `*.velty.cl` | **listo para ejecutar** |

**Las Etapas 3 y 4 son independientes entre sí** — ninguna bloquea a la
otra, se pueden ejecutar en cualquier orden o en paralelo. Cada una toca
archivos distintos salvo un punto de contacto explícito: ambas firman con
el mismo secreto HS256 de `iam` (ver Etapa 4 §4.2) y la Etapa 4 §4.3
describe cómo un consumidor llega al endpoint que la Etapa 3 expone — pero
ninguna de las dos necesita que la otra esté terminada para compilar y
pasar sus propios tests.

> **Si te dijeron "ejecuta el plan descrito en docs/PLAN.md": ejecuta
> ambas etapas 3 y 4**, cada una siguiendo su propio archivo de principio a
> fin (frontmatter, reglas de calidad, criterios de aceptación incluidos).

## Fuera de alcance en este repo

- El código consumidor en `misitio` (o cualquier otra app) que llama a
  `/api/token` de `iam` — plan aparte, dentro del repo de esa app, que
  depende de que la Etapa 3 exista primero.
- Migrar `veltylabs/mjosefa-cms`.
- Panel de administración de `iam` (crear proyectos/roles desde una UI) —
  sin diseñar todavía, necesita su propia ronda de preguntas.
- Cualquier cambio a `tinywasm/user`. Si una etapa revela que falta algo
  ahí, repórtalo — no lo copies localmente (ver `AGENTS.md` Restricción #2).

## Después de las Etapas 3 y 4: verificación cruzada

- `go test ./...` en `tinywasm/jwt`, `tinywasm/auth`, `tinywasm/rbac` e
  `iam` — los cuatro repos que estas dos etapas tocan.
- `grep -rn "ProjectID\|Roles\s*\[\]string" tinywasm/auth tinywasm/jwt --include="*.go"`
  → vacío en `tinywasm/jwt` (no debe conocer el concepto de roles) y vacío
  en `tinywasm/auth` salvo lo que ya existía antes de este plan.

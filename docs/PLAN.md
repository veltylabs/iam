---
PLAN: "perf!: el edge deja de pagar un viaje a Virginia por un health check"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 4806985898598155454
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# PLAN — `veltylabs/iam`: bajar la latencia y adelgazar el edge

> Si te dijeron "ejecuta el plan descrito en docs/PLAN.md", ejecuta **TODAS las
> etapas de abajo, en orden**. Cada una es autocontenida: termina una antes de
> empezar la siguiente. Nunca mezcles cambios de una etapa en otra.

> ⚠️ **La etapa 3 de este plan ("Adoptar la Action y soltar el tooling del
> `go.mod`") ya está hecha, fuera de este flujo.** Se ejecutó a mano en
> [veltylabs/iam#3](https://github.com/veltylabs/iam/pull/3), CI en verde:
> `.github/workflows/deploy.yml` corre `tinywasm/goflare@v1`, y de paso se
> resolvieron sus partes B (confirmada no-aplicable: `cmd/migrate/main.go`
> importa `goflare.NewD1Migrator` directo, así que goflare sigue siendo
> dependencia real de código) y C (`tinywasm/cloudflare` v0.0.4→v0.0.11,
> `tinywasm/auth` v0.0.10→v0.0.12 para no chocar con el `router` v0.1.28 que
> arrastra). `PLAN_STAGE_3_ACTION.md` se borró: su contenido ya vive en el PR
> mergeado. **Este plan ahora son solo las etapas 1 y 2.**

## Estado de las puertas — compruébalo antes de empezar

| Necesita | Versión mínima | Para qué |
|---|---|---|
| `github.com/tinywasm/auth` | la que borra `warmUp` (v0.0.11+) | etapa 1 |

Ya publicada (v0.0.12). No hay puerta bloqueando ninguna de las dos etapas.

## El diagnóstico — no lo repitas, ya está hecho

`/api/health` responde entre 163 y 363 ms. Medido contra la base real:

```
D1 velty-iam-db → running_in_region: ENAM      read_replication: disabled
consulta de prueba → served_by_colo: ATL, sql_duration_ms: 1.05
```

El Worker corre en Santiago; la base, en Norteamérica Este. `/api/health`
ejecuta `SELECT 1`. **El SQL cuesta 1 ms; los otros ~160 ms son el viaje.**

Y la sospecha de que "se coló stdlib en el binario" es **falsa**: cero paquetes
`tinywasm/*` o `veltylabs/*` importan stdlib prohibida. Los 87 paquetes de
stdlib del grafo del edge cuelgan todos de un solo import deliberado y
documentado, `tinywasm/crypto/hmac` → `crypto/sha256`. **No refactorices
`crypto`.**

Lo que sí hay es peso muerto **en este repo**, medido compilando de verdad:

| | crudo | gzip |
|---|---|---|
| hoy | 600.380 B | 212.539 B |
| sin `config/login.go` (cero llamadores) | 586.388 B (−13.992) | 206.885 B |
| además, bcrypt fuera del edge | 570.517 B (−29.863 total) | 198.971 B |

## Las tablas están vacías — aprovéchalo

```sql
SELECT COUNT(*) FROM project;  -- 0
SELECT COUNT(*) FROM session;  -- 0
SELECT COUNT(*) FROM "user";   -- 0
```

**No hay datos que migrar.** Cualquier cambio de formato de hash es gratis hoy.

## Reglas de calidad — obligatorias en cada etapa

**Nada de strings mágicos.** Toda cadena repetida (ruta, clave de entorno,
prefijo) es una constante nombrada. El repo ya tiene el sitio: las rutas viven
en [`api/api.go`](../api/api.go) para romper el ciclo `config`↔`routes`.

**Los dos objetivos de compilación de este repo:**

| Objetivo | Archivos | Reglas |
|---|---|---|
| `wasm` | `edge/`, y todo `config/`, `routes/`, `api/` **sin** `//go:build !wasm` | **Nada de stdlib.** `tinywasm/fmt` en vez de `errors`/`fmt`/`strconv`/`strings`. |
| `!wasm` | `web/`, `cmd/`, `tests/`, y los `*_local.go` de `config/` | La stdlib es correcta y esperada. |

> ⚠️ **Anti-footgun.** `config/` se compila para **ambos**. Un import añadido
> ahí sin build tag entra en el Worker. Es exactamente el defecto que arregla la
> etapa 2 — no lo reintroduzcas.

**Verificación al terminar cada etapa:**

```sh
go build ./...
go vet ./...
GOOS=js GOARCH=wasm go vet ./edge/...
go test ./tests/...
```

> ⚠️ Usa `go vet`, **no** `go build`, para el objetivo wasm: `go build ./edge/...`
> choca con el nombre del directorio `edge/` al escribir su salida.

## Etapas

> ⚠️ **`docs/PLAN_STAGE_1_PORT_IDENTITY.md` NO forma parte de este plan.** Es el
> resto de un plan anterior ya ejecutado (commit `793f3c9`), y comparte prefijo
> de numeración por accidente. Las etapas de este plan son **exactamente** las
> tres de la tabla de abajo. No lo ejecutes, no lo edites y no lo borres.

| Orden | Etapa | Asunto |
|---|---|---|
| 1 | [PLAN_STAGE_1_HEALTH.md](PLAN_STAGE_1_HEALTH.md) | `/api/health` deja de consultar D1; nace `/api/health/db` |
| 2 | [PLAN_STAGE_2_EDGE_SLIMMING.md](PLAN_STAGE_2_EDGE_SLIMMING.md) | Sacar del Worker el código muerto y bcrypt |

Al terminar, corre la verificación completa una última vez: todo en verde.

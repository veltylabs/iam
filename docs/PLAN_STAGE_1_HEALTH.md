Etapa 1 de 3 | Siguiente → [Etapa 2](PLAN_STAGE_2_EDGE_SLIMMING.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 1 — Un health check que mida al Worker, no a la red

## El código actual

[`routes/routes.go`](../routes/routes.go), línea 51:

```go
// Health devuelve el manejador HTTP para la verificación de estado.
func Health(db *orm.DB) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")

		if db == nil || db.RawConn() == nil {
			ctx.WriteStatus(503)
			_, _ = ctx.Write([]byte("{\"ok\":false}"))
			return
		}
		if err := db.RawConn().Exec("SELECT 1"); err != nil {
			ctx.WriteStatus(503)
			_, _ = ctx.Write([]byte("{\"ok\":false}"))
			return
		}
		ctx.WriteStatus(200)
		_, _ = ctx.Write([]byte("{\"ok\":true}"))
	}
}
```

Ese `SELECT 1` cuesta 1 ms de SQL y ~160 ms de viaje a Virginia. Y responde una
pregunta que nadie hizo: quien consulta `/api/health` quiere saber **si el
Worker está vivo**, no si la base es alcanzable desde este PoP en este instante.

Mezclar las dos cosas tiene un segundo costo, peor que la latencia: un hipo de
red hacia D1 hace que el Worker se declare caído aunque esté perfectamente sano,
y cualquier monitor externo conectado a esa ruta dispara una falsa alarma.

## Lo que hay que hacer

### 1. Rutas — en `api/api.go`

```go
const (
	PathHealth       = "/api/health"
	PathHealthDB     = "/api/health/db"
	PathToken        = "/api/token"
	PathUsersResolve = "/api/users/resolve"
)
```

Reexpórtala en `routes/routes.go` junto a las demás:

```go
const PathHealthDB = api.PathHealthDB
```

### 2. Los cuerpos de respuesta también son constantes

Hoy `{"ok":true}` y `{"ok":false}` son literales escapados repartidos por el
manejador. Extráelos:

```go
const (
	bodyHealthOK   = `{"ok":true}`
	bodyHealthFail = `{"ok":false}`
)
```

Con comillas invertidas se acaban las barras de escape.

### 3. `Health` — liveness, sin argumentos

```go
// Health responde si este Worker esta vivo y sirviendo. A proposito NO toca la
// base: quien consulta esta ruta pregunta por el Worker, y meter una consulta a
// D1 aqui costaba ~160 ms de viaje a la region de la base ademas de convertir
// un hipo de red en una falsa alarma de caida. La alcanzabilidad de la base
// tiene su propia ruta, PathHealthDB.
func Health() router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		ctx.WriteStatus(200)
		_, _ = ctx.Write([]byte(bodyHealthOK))
	}
}
```

Fíjate en que **pierde el parámetro `db`**. Es intencional y es la garantía
estructural de que nadie le vuelva a añadir una consulta: sin la base a mano, no
hay nada que consultar.

### 4. `HealthDB` — la comprobación que sí consulta

```go
// HealthDB comprueba que la base responde. Es deliberadamente una ruta aparte:
// cuesta un viaje completo a la region donde vive D1, asi que se consulta
// cuando se quiere esa respuesta, no en cada latido de un monitor.
func HealthDB(db *orm.DB) router.HandlerFunc
```

Mismo cuerpo que el `Health` de hoy: 503 con `bodyHealthFail` si `db` es nil, si
`RawConn()` es nil o si `Exec("SELECT 1")` falla; 200 con `bodyHealthOK` si no.
El `"SELECT 1"` va a una constante, `const healthProbeSQL = "SELECT 1"`.

### 5. El registro — en `Register`

```go
	r.Get(PathHealth, Health()).Public()
	r.Get(PathHealthDB, HealthDB(db)).Public()
```

## Criterios de aceptación

- `grep -n "func Health(" routes/routes.go` → sin parámetros.
- `grep -c "SELECT 1" routes/routes.go` → **1** (solo la constante).
- `grep -rn '{\\"ok\\"' routes/` → vacío: no quedan literales escapados.
- `go build ./...`, `go vet ./...`, `GOOS=js GOARCH=wasm go vet ./edge/...` limpios.
- `go test ./tests/...` en verde.

## Tests — en `tests/`

1. `TestHealthDoesNotTouchDB` — **el test que da sentido a la etapa**. Llama al
   manejador que devuelve `routes.Health()` con un contexto falso y comprueba
   200 más `{"ok":true}`. Que compile sin una `*orm.DB` ya prueba la mitad; la
   otra mitad es que responde.
2. `TestHealthDBReportsFailure` — `routes.HealthDB(nil)` responde 503 con
   `{"ok":false}`.
3. `TestHealthDBReportsSuccess` — contra la base en memoria que ya usan los
   tests, responde 200.

Si algún test existente golpea `PathHealth` esperando que valide la base,
**actualízalo para que golpee `PathHealthDB`**. No restaures la consulta.

## Verificación en producción — parte del cierre, no un extra

Después del despliegue:

1. Reactiva Workers Observability, que **se apaga en cada `goflare deploy`**:
   ```
   PATCH /accounts/{id}/workers/scripts/iam/settings
   {"observability":{"enabled":true,"head_sampling_rate":1}}
   ```
2. Lanza 5 peticiones espaciadas a `/api/health` y 5 a `/api/health/db`.
3. Lee `wallTimeMs` de cada una en Observability.

Lo que hay que ver: `/api/health` **por debajo de 20 ms**, y `/api/health/db`
alrededor de los 163 ms de hoy (esa ruta sigue pagando el viaje, y está bien que
lo pague: es lo que mide).

**Si `/api/health` no baja de 20 ms, repórtalo en vez de cerrar la etapa.**
Significaría que el costo estaba en otra parte y el diagnóstico de este plan
está incompleto.

## Lo que NO hay que hacer

- **No** borres la comprobación de la base. Múdala, que no es lo mismo.
- **No** metas `/api/health/db` detrás de autenticación. Es una sonda operativa
  y no filtra nada: responde un booleano.
- **No** añadas caché al resultado de `HealthDB`. Una comprobación de salud
  cacheada no comprueba nada.

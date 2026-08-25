← [Etapa 3](PLAN_STAGE_3_BEARER_API.md) (independiente, no bloquea ni depende de esta)

# Etapa 4 — Sesión SSO real entre subdominios `*.velty.cl`

> Lee primero [`ARCHITECTURE.md`](ARCHITECTURE.md) §7. Este archivo asume
> ese contexto.

## Objetivo

Que un usuario que se loguea una vez en `iam.velty.cl` tenga sesión
reconocible en cualquier `*.velty.cl` sin volver a pasar por Google.

## La regla que gobierna esta etapa

```
La cookie de identidad la lee y firma SOLO iam. Ningún consumidor la
verifica directamente — ni tiene el secreto para hacerlo.
```

Esto es deliberado, no una limitación a resolver después: compartir el
secreto HS256 con cada consumidor significaría que cualquiera de ellos
comprometido podría forjar una identidad para cualquier usuario en
`*.velty.cl`. La cookie es la señal de "hay sesión SSO"; para saber quién
es y qué puede hacer, un consumidor siempre llama a `iam` (el endpoint
`/api/token` de la Etapa 3, que sí corre dentro de `iam` y por tanto sí
tiene el secreto).

## Qué se construye — verificado contra el código real

`router.Cookie` (`tinywasm/router/cookie.go`) ya declara `Domain string`
(*"omit for current domain"*) — el contrato ya existe, nadie lo usa
todavía. `auth/session/jwt.Strategy.Issue`/`Revoke`
(`tinywasm/auth/session/jwt/jwt.go`) construyen el `router.Cookie` sin
poblar ese campo. Se cierra el hueco de forma **aditiva**: un valor por
defecto vacío preserva el comportamiento actual para cualquier consumidor
de `jwt.Strategy` que no llame el nuevo método (hoy: los tests de `auth`
mismo).

### 4.1 — `tinywasm/auth/session/jwt/jwt.go`

```go
type Strategy struct {
	secret     []byte
	ttl        int
	bearer     bool
	cookieName string
	domain     string // nuevo: "" (default) = cookie atada al host exacto,
	                   // igual que hoy. ".velty.cl" = compartida entre subdominios.
	notify     auth.SecurityNotifier
	users      auth.IdentityStore
}

// WithDomain sets the cookie's Domain attribute — for a session meant to
// be shared across subdomains of a parent domain (e.g. ".velty.cl").
// Ignored in bearer mode (no cookie is set). Empty string (default)
// leaves Domain unset: the browser scopes the cookie to the exact host
// that issued it, unchanged from before this method existed.
func (s *Strategy) WithDomain(domain string) *Strategy { s.domain = domain; return s }
```

Y en `Issue`/`Revoke`, agregar `Domain: s.domain` al `router.Cookie{...}`
que ya construyen — **una sola línea en cada uno**, no reescribas el resto
de esos métodos:

```go
// en Issue:
ctx.SetCookie(router.Cookie{
	Name: s.cookieName, Value: token, Domain: s.domain, HttpOnly: true, Secure: true,
	SameSite: router.SameSiteStrict, MaxAge: s.ttl, Path: "/",
})

// en Revoke:
ctx.SetCookie(router.Cookie{Name: s.cookieName, Value: "", Domain: s.domain, Path: "/", MaxAge: -1, HttpOnly: true})
```

`SameSite: router.SameSiteStrict` **no cambia** — queda igual que hoy. No
lo relajes a `Lax`/`None`: `SameSite` compara el sitio registrable
(`velty.cl`), no el host exacto, así que `Strict` ya viaja correctamente
entre `iam.velty.cl` y `misitio.velty.cl` (mismo sitio, distinto host). Si
en algún punto ves un caso donde la cookie no llega y la solución parece
ser relajar SameSite, para y pregunta — probablemente el problema es otro
(dominio mal escrito, Secure en un origen http, etc.), no SameSite.

### 4.2 — `veltylabs/iam/config` — construir la sesión con dominio compartido

Donde `iam` construye su `authority.Module` (el archivo que orquesta
`NewProductionAuth`, ver Etapa 1 y Etapa 3 §3.5), reemplazar la estrategia
default (`cookie.Strategy`, la que `authority.New` monta automáticamente)
por la de dominio compartido:

```go
const (
	SSOCookieDomain = ".velty.cl"
	SSOSessionTTL   = 86400 * 7 // 7 dias, ver ARCHITECTURE.md §7
)

strategy, err := jwt.New(secret, SSOSessionTTL, authMod, authMod)
if err != nil {
	return nil, nil, err
}
strategy.WithDomain(SSOCookieDomain)
authMod.SetStrategy(strategy)
```

`secret` es el mismo secreto HS256 que `IssueAuthToken` (Etapa 3) usa para
firmar el token de autorización (`Claims` con `Aud`/`Scope` poblados vía
`jwt.NewScopedClaims`) — **un solo secreto para todo `iam`**, no dos. Vive
donde ya vive cualquier otro secreto de `iam` (variable de entorno vía
`tinywasm/env`/`osenv`, igual que `GOOGLE_CLIENT_SECRET` — ver Etapa 1,
`config/auth.go`). Añade la constante correspondiente (`EnvJWTSecret =
"JWT_SECRET"`) junto a las demás `Env*` ya existentes en ese archivo, no
en uno nuevo.

**No confundas este secreto con `client_secret` de un proyecto (Etapa 3
§3.2).** Son cosas distintas: el secreto HS256 firma tokens (identidad y
autorización), es interno de `iam` y nunca sale de ahí; `client_secret`
autentica a una APP consumidora ante `iam` y sí se le entrega a esa app.

### 4.3 — CORS para que un consumidor pueda leer la sesión

Un consumidor (ej. `misitio.velty.cl`) no verifica la cookie — llama a
`POST iam.velty.cl/api/token` (Etapa 3) desde el navegador del usuario, con
`fetch(..., {credentials: "include"})`, para que el navegador adjunte la
cookie de dominio compartido. Eso es una llamada **cross-origin**: `iam`
necesita responder con cabeceras CORS que permitan credenciales — lo cual
exige un origen explícito, nunca `*`
(`Access-Control-Allow-Origin` + credentials no admite wildcard, es una
regla del propio estándar CORS, no de este ecosistema).

`iam` no conoce de antemano cada subdominio consumidor futuro — el origen
permitido debe resolverse dinámicamente: si el `Origin` de la request
termina en `.velty.cl` (o es exactamente `velty.cl`), se refleja de vuelta
en `Access-Control-Allow-Origin`; si no, se omite la cabecera (el navegador
bloquea la respuesta). Esto vive en `routes/routes.go`, como middleware
sobre las rutas que un consumidor llama desde el navegador (`/api/token`),
no sobre `/oauth/*` (esas nunca se llaman vía `fetch`, son navegación
normal del navegador).

**Fuera de alcance de esta implementación:** el detalle exacto de cómo
`misitio` consume esto (dónde en su código hace el `fetch`, qué hace con
el token recibido) es el plan de migración de `misitio`, en su propio
repo — mencionado como fuera de alcance desde la Etapa 1. Esta etapa solo
deja a `iam` listo para responder correctamente a esa llamada.

---

## Archivos a crear/modificar

```
tinywasm/auth/session/jwt/jwt.go   // modificar: domain field, WithDomain, Issue/Revoke
veltylabs/iam/config/auth.go       // modificar: EnvJWTSecret, SSOCookieDomain/TTL, SetStrategy
veltylabs/iam/routes/routes.go     // modificar (creado en Etapa 3): CORS en /api/token
veltylabs/iam/tests/sso_test.go    // nuevo
```

## Reglas de calidad

- **Una sola constante de dominio** (`SSOCookieDomain`), reusada en
  cualquier lugar que necesite `.velty.cl` — no lo escribas como literal
  en un segundo sitio.
- **El secreto HS256 nunca tiene un valor por defecto hardcodeado.** Igual
  que `EnvGoogleClientID`/etc. en la Etapa 1: si `EnvJWTSecret` no está
  seteada, `iam` falla al arrancar — no arranca con un secreto vacío o de
  ejemplo (`jwt.New` ya rechaza `secret` vacío con `ErrJWTSecretRequired`,
  pero no dejes que un valor no-vacío-pero-de-ejemplo se cuele en el
  código fuente).

## Criterios de aceptación

- [ ] `go test ./...` en `tinywasm/auth` e `iam`, todo verde.
- [ ] Un test en `auth/session/jwt` prueba que `WithDomain(".velty.cl")`
      produce un `router.Cookie` con `Domain == ".velty.cl"` en `Issue`, y
      que sin llamarlo `Domain` sigue vacío (comportamiento no roto para
      quien no usa el método nuevo).
- [ ] Un test confirma que `Revoke` también lleva el `Domain` correcto —
      una cookie de borrado con el dominio equivocado no borra nada.
- [ ] `grep -n "SameSite: router.SameSiteLax\|SameSiteNone" tinywasm/auth/session/jwt/jwt.go` → vacío: se mantiene `Strict`.
- [ ] Un test en `iam/tests` confirma que `POST /api/token` responde
      `Access-Control-Allow-Origin` reflejando un `Origin: https://misitio.velty.cl`
      de request, y que un `Origin` fuera de `*.velty.cl` no lo recibe.

## Fuera de alcance

- El código consumidor en `misitio` (o cualquier otra app) que llama a
  `/api/token` — plan aparte, en el repo de esa app.
- Revocación de la cookie de identidad antes de sus 7 días (ver
  ARCHITECTURE.md §6.3 — misma decisión de TTL corto + renovación que la
  Etapa 3, aplicada aquí a la sesión de identidad).

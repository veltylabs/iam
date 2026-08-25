← [Etapa 1](PLAN_STAGE_1_PORT_IDENTITY.md) | Etapa 2 ya ejecutada (ver `docs/ARCHITECTURE.md` §5) | Siguiente → [Etapa 4](PLAN_STAGE_4_SSO_COOKIE.md) (independiente, no bloquea ni depende de esta)

# Etapa 3 — API HTTP con tokens de autorización por proyecto

> Lee primero [`ARCHITECTURE.md`](ARCHITECTURE.md) §6 — explica por qué son
> **dos tokens** (identidad vs. autorización) y por qué el payload nuevo no
> se agrega a `tinyjwt.Claims`. Este archivo asume ese contexto.

## Objetivo

Que una app consumidora (ej. `misitio`) pueda pedirle a `iam` un token que
pruebe "el usuario X tiene estos roles en mi proyecto", sin que `iam`
monte `authority.Module`/`rbac.Service` de nuevo en cada app, y sin que
`tinywasm/jwt` o `tinywasm/auth` tengan que conocer el concepto de rol.

## La regla que gobierna esta etapa

```
tinywasm/jwt solo conoce vocabulario JWT estándar (audience, scope).
Nunca ve las palabras "project" ni "role" — esas las pone iam al llenar
los campos, no al definirlos.
```

Esto no es una preferencia estética: es la regla `auth` nunca importa
`rbac` que ya rige todo el ecosistema (ver `tinywasm/auth`
`docs/ARCHITECTURE.md`), aplicada un nivel más abajo — a `tinywasm/jwt`,
del que `auth` depende. Si el tipo `Claims` tuviera campos llamados
`ProjectID`/`Roles`, esa capa base "sabría" conceptualmente que existe
RBAC, reintroduciendo tan silenciosamente como el vestigio ya limpiado en
la Etapa 1/2 el mismo acoplamiento. `Aud`/`Scope` son vocabulario JWT
genérico — `iam` es quien decide que `Aud` significa "project_id" y
`Scope` significa "códigos de rol"; `tinywasm/jwt` nunca lo sabe.

## Qué se construye — verificado contra el código real de `tinywasm/jwt`

`Claims{Sub, Exp, Iat}` en `tinywasm/jwt/jwt.go` gana dos campos, con
nombres del vocabulario **estándar** de JWT/OAuth2 — no de RBAC:

```go
type Claims struct {
	Sub   string   // subject: who the token authenticates
	Exp   int64    // expiry, unix seconds
	Iat   int64    // issued at, unix seconds

	// Aud is the RFC 7519 "audience" claim: who/what the token is scoped
	// to. "" means unscoped — an identity-only token (unchanged meaning
	// from before this field existed).
	Aud string

	// Scope lists what the subject is allowed to do within Aud. nil means
	// no scope claims — same as Aud, an identity-only token leaves this
	// empty. tinywasm/jwt does not interpret these strings; a caller
	// scoping a token to a project fills Aud with a project id and Scope
	// with whatever role vocabulary that project uses (see
	// veltylabs/iam's use in config/token.go) — this package never says
	// "role" or "project", only "audience" and "scope".
	Scope []string
}
```

`EncodeFields`/`DecodeFields` ganan las dos líneas correspondientes (sigue
el mismo patrón que `sub`/`exp`/`iat` ya tienen — `Scope` es un array,
mismo patrón `w.Array("scope", len(...))`/`r.Array("scope")` que usa
cualquier otro campo `[]string` en el ecosistema, ej.
`api.RoleInfo`-adjacent code en `misitio`).

`NewClaims(subject string, ttl int) Claims` **no cambia** — sigue
devolviendo `Aud`/`Scope` en su zero value (`""`/`nil`), así que todo
consumidor existente (`auth/session/jwt.Strategy.Issue`,
`GenerateAPIToken`) sigue produciendo tokens de identidad pura, sin tocar
una línea de su propio código. Se añade, aditiva, una segunda
constructora para el caso con alcance:

```go
// NewScopedClaims builds a claim set like NewClaims, additionally scoped
// to aud with the given scope — for tokens that authorize actions within
// a specific audience (e.g. a project), not just identity.
func NewScopedClaims(subject, aud string, scope []string, ttl int) Claims {
	c := NewClaims(subject, ttl)
	c.Aud = aud
	c.Scope = scope
	return c
}
```

**`Sign`/`Verify` no cambian en absoluto** — ya firman/verifican
`Claims` completo, y `Claims` ahora simplemente tiene dos campos más.
No hay tipo nuevo, no hay mecanismo de firma paralelo: un token de
autorización se firma con el mismo `jwt.Sign(secret, claims)` de siempre,
sobre un `Claims` construido con `NewScopedClaims` en vez de `NewClaims`.

---

## Archivos a crear/modificar

```
tinywasm/jwt/jwt.go              // modificar: Claims gana Aud/Scope, NewScopedClaims
veltylabs/iam/config/projects.go     // nuevo — modelo Project (client credentials)
veltylabs/iam/config/token.go        // nuevo — emisión del token de autorización
veltylabs/iam/routes/routes.go       // nuevo — primera vez que iam tiene rutas HTTP
veltylabs/iam/web/server.go          // nuevo — servidor de desarrollo
veltylabs/iam/edge/main.go           // nuevo — Worker de producción
veltylabs/iam/tests/token_test.go    // nuevo
```

## Pasos

### 3.1 — `veltylabs/iam/config/projects.go`

Modelo nuevo, propio de `iam` (no de `rbac`: `rbac` no conoce credenciales
de aplicación, solo roles/permisos). Sigue el mismo patrón `model.Definition`
que `tinywasm/rbac/models.go` — correr `ormc` después de escribir esto para
generar `models_orm.go`.

```go
package config

import "github.com/tinywasm/model"

var ProjectModel = model.Definition{
	Name: "project",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text()},
		{Name: "client_secret_hash", Type: model.Text()},
		{Name: "created_at", Type: model.Int()},
	},
}
```

Y en `config/migrate.go` (nuevo, mismo patrón que `rbac/migrate.go`):

```go
package config

import (
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
)

func initProjectSchema(db *orm.DB) error {
	ddlCompiler, ok := db.RawConn().(ddl.Compiler)
	if !ok {
		return nil
	}
	return ddl.New(db.RawConn(), ddlCompiler).Sync(&Project{})
}
```

Llamar `initProjectSchema(db)` desde donde se construye el backend de iam
(el archivo que orquesta `NewProductionAuth`/`rbac.New` — créalo si no
existe todavía como `config/backend.go`, mismo patrón que
`misitio/config/backend.go`).

`CreateProject`/`VerifyProjectSecret` usan `tinywasm/crypto/bcrypt`
— **el mismo mecanismo que ya usa `tinywasm/auth/email_password`** para
contraseñas, no inventes otro:

```go
package config

import (
	"github.com/tinywasm/crypto/bcrypt"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/time"
)

// CreateProject registra un proyecto nuevo y devuelve el client_secret EN
// CLARO una sola vez — no se puede recuperar después, solo regenerar.
func CreateProject(db *orm.DB, id, name, plainSecret string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainSecret), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.Create(&Project{
		Id: id, Name: name, ClientSecretHash: string(hash),
		CreatedAt: time.Now() / 1e9,
	})
}

// VerifyProjectSecret compara en tiempo constante vía bcrypt — nunca ==.
func VerifyProjectSecret(db *orm.DB, projectID, plainSecret string) (bool, error) {
	qb := db.Query(&Project{}).Where(Project_.Id).Eq(projectID)
	rows, err := ReadAllProject(qb)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(rows[0].ClientSecretHash), []byte(plainSecret)) == nil, nil
}

var ErrProjectNotFound = fmt.Err("project", "not", "found")
```

### 3.2 — `veltylabs/iam/config/token.go`

Orquesta auth (identidad ya resuelta por la cookie/sesión activa) + rbac
(roles del proyecto) + el `SessionTTL` más restrictivo, y firma con el
`jwt.Sign` **existente**, sobre un `Claims` construido con
`jwt.NewScopedClaims` — **no** con
`tinywasm/auth/session/jwt.GenerateAPIToken` (ese es para tokens sin
scope, ver ARCHITECTURE.md §6.1).

```go
package config

import (
	"github.com/tinywasm/jwt"
	"github.com/tinywasm/rbac"
)

const DefaultAuthTokenTTL = 30 * 60 // 30 minutos — ver ARCHITECTURE.md §6.3

// IssueAuthToken firma un token de autorizacion project-scoped para
// userID, usando el SessionTTL mas restrictivo entre sus roles en
// projectID (0 => DefaultAuthTokenTTL). secret es el mismo secreto HS256
// que usa el resto de la sesion de iam. Aud lleva projectID, Scope lleva
// los codigos de rol — vocabulario JWT estandar, ver ARCHITECTURE.md §6.2.
func IssueAuthToken(rbacSvc *rbac.Service, secret []byte, projectID, userID string) (string, error) {
	roles, err := rbacSvc.GetUserRoles(projectID, userID)
	if err != nil {
		return "", err
	}
	ttl := DefaultAuthTokenTTL
	roleCodes := make([]string, len(roles))
	for i, r := range roles {
		roleCodes[i] = r.Code
		if r.SessionTTL > 0 && (ttl == DefaultAuthTokenTTL || int(r.SessionTTL) < ttl) {
			ttl = int(r.SessionTTL)
		}
	}
	claims := jwt.NewScopedClaims(userID, projectID, roleCodes, ttl)
	return jwt.Sign(secret, claims)
}
```

Esto asume `rbac.Role` ya tiene el campo `SessionTTL int64` — agrégalo en
`tinywasm/rbac/models.go` (`RoleModel`, campo nuevo `session_ttl`,
`Type: model.Int()`, sin `NotNull` — 0 es un valor válido que significa
"usa el default"), corre `ormc`, publica una nueva versión de
`tinywasm/rbac` con `gopush`, y actualiza la dependencia en `iam` antes de
escribir este archivo. **No dupliques el campo en otro lado**: `SessionTTL`
vive en `Role`, no en `Claims` ni en `Project`.

Del lado de quien verifica (fuera de alcance de este repo, pero para que
el diseño quede completo): `jwt.Verify(secret, token)` devuelve el mismo
`Claims` de siempre — `claims.Aud`/`claims.Scope` ya están ahí, sin type
assertions ni un segundo tipo que decodificar.

### 3.3 — Rutas (`veltylabs/iam/routes/routes.go`)

Primera vez que `iam` expone HTTP. Sigue el patrón de
`misitio/routes/routes.go` (`Register(r router.Router, deps...)`, un
`api/` hoja con los paths — créalo si aún no existe).

```
POST /api/token   — {project_id, client_secret} + cookie de sesión activa
                     → {token: "<jwt firmado>"}. 401 sin sesión; 403 si
                     project_id/client_secret no valida.
```

El handler NO acepta el `userID` en el body — lo lee de `ctx.UserID()`
(ya puesto por el middleware `Authn` tras `authMod.Authenticate()`), igual
que hace `routes.Me` en `misitio`. Aceptar un userID del body sería la
misma clase de bug que "el `site_id` sale de la membresía, nunca del
cuerpo" en `misitio/docs/diagrams/autenticacion.md` — aquí es "el userID
sale de la sesión, nunca del cuerpo".

```go
func Token(db *orm.DB, rbacSvc *rbac.Service, secret []byte) router.HandlerFunc {
	return func(ctx router.Context) {
		userID := ctx.UserID()
		if userID == "" {
			ctx.WriteStatus(401)
			return
		}
		var body TokenRequest // {ProjectID, ClientSecret string}
		if err := ctx.Decode(&body); err != nil {
			ctx.WriteStatus(400)
			return
		}
		ok, err := VerifyProjectSecret(db, body.ProjectID, body.ClientSecret)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !ok {
			ctx.WriteStatus(403)
			return
		}
		token, err := IssueAuthToken(rbacSvc, secret, body.ProjectID, userID)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&TokenResponse{Token: token})
	}
}
```

`TokenRequest`/`TokenResponse` con `Encodable`/`Decodable` — mismo patrón
que `routes.MeResponse` en `misitio`, no reinventes el mecanismo de codec.

### 3.4 — `web/server.go` / `edge/main.go`

Mismo patrón que `misitio/web/server.go`/`edge/main.go` (ya leídos y
portados en la Etapa 1): construir el backend (auth+rbac+projects),
inyectar `Authn: authMod.Authenticate()`, montar `routes.Register(...)`.
`edge/main.go` necesita su propia D1 — **no reutiliza `veltydb` de
`misitio`**: `iam` tiene su propia base (ver `AGENTS.md` Restricción #5).
Provisionar esa D1 y su binding es un paso de infraestructura, no de
código — coordínalo con el mantenedor antes de desplegar, no lo asumas.

---

## Reglas de calidad

- **`tinywasm/jwt` sigue sin importar nada de `rbac` ni conocer el
  concepto de "rol" o "proyecto".** `Claims.Aud`/`Claims.Scope` son
  `string`/`[]string` genéricos con nombres estándar JWT/OAuth2 — que
  `iam` los llene con `projectID` y códigos de rol es una decisión de
  quien FIRMA (`config/token.go`), no algo que `jwt` sabe ni valida. Si
  sientes la tentación de renombrarlos a `ProjectID`/`Roles` dentro de
  `tinywasm/jwt` "para que quede más claro", no lo hagas — eso reintroduce
  exactamente el acoplamiento que este diseño evita (ver `ARCHITECTURE.md`
  §6.2, las 3 opciones consideradas y por qué se descartaron la 1 y la 2).
- **No hay un segundo tipo de claims que decodificar.** `jwt.Verify` sigue
  devolviendo el mismo `Claims` de siempre — no crees un
  `AuthClaims`/`ProjectClaims` en `iam` (ni en ningún otro repo) para
  "envolver" `Aud`/`Scope`; ya están en el tipo base, sin type assertions.
- **`client_secret` nunca se guarda en claro** — solo su hash (`bcrypt`,
  mismo mecanismo que `email_password`). `CreateProject` lo devuelve UNA
  vez, en el momento de creación; no hay un `GetProjectSecret`.
- **Sin duplicar la constante de TTL**: `DefaultAuthTokenTTL` vive una sola
  vez en `config/token.go`.

## Criterios de aceptación

- [ ] `go test ./...` en `tinywasm/jwt`, `tinywasm/rbac` e `iam`, todo verde.
- [ ] Un test en `tinywasm/jwt` prueba `NewScopedClaims` + `Sign` + `Verify`:
      el roundtrip preserva `Aud` y `Scope` exactos (con `Scope` de más de
      un elemento). La doctrina "an API is not published until a
      consumer-shaped test proves it"
      (`tinywasm/sitec/docs/CONSTRUCTION_HARNESS.md`) aplica aquí: si es
      incómodo de escribir, el diseño tiene un defecto, no el test.
- [ ] Un test en `tinywasm/jwt` confirma que `NewClaims` (el constructor ya
      existente, sin `Aud`/`Scope`) sigue produciendo un token que `Verify`
      decodifica con `Aud == ""` y `Scope == nil` — comportamiento no roto
      para quien no usa los campos nuevos.
- [ ] Un test en `iam/tests` prueba el flujo completo: usuario con un rol
      que tiene `SessionTTL` propio → `IssueAuthToken` usa ESE TTL, no el
      default.
- [ ] Un test prueba que `POST /api/token` con `client_secret` incorrecto
      devuelve 403, y que el `userID` del token emitido es siempre el de
      la sesión (`ctx.UserID()`), nunca el que llegue en el body si
      alguien intenta mandarlo.
- [ ] `grep -rn "Roles\s*\[\]string\|ProjectID" /home/cesar/Dev/Project/tinywasm/auth --include="*.go"` → vacío: `auth` sigue sin conocer roles ni proyectos.
- [ ] `grep -n "Roles\s*\[\]string\|ProjectID\s*string" /home/cesar/Dev/Project/tinywasm/jwt/jwt.go` → vacío: `jwt` gana `Aud`/`Scope` (nombres estándar JWT/OAuth2), nunca el vocabulario específico de rbac.

## Fuera de alcance

- Revocación explícita de tokens (ver ARCHITECTURE.md §6.3 — TTL corto +
  renovación es la decisión tomada).
- La cookie de identidad cross-dominio — es la Etapa 4, independiente de
  esta.
- El panel de administración para crear proyectos/roles desde una UI — hoy
  `CreateProject`/`rbacSvc.CreateRole` se llaman directo (script, test, o
  a mano); la UI es un tema aparte sin diseñar todavía.

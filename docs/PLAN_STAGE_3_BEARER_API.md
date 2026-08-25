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
El motor de firma (tinywasm/jwt) no sabe qué hay dentro del payload.
auth no sabe qué es un rol. Solo iam — la única composition root que
importa auth Y rbac — construye el payload que combina ambos.
```

Esto no es una preferencia estética: es la regla `auth` nunca importa
`rbac` que ya rige todo el ecosistema (ver `tinywasm/auth`
`docs/ARCHITECTURE.md`). Un tipo `AuthClaims{..., Roles []string}` dentro
de `tinywasm/auth` la rompería tan silenciosamente como el vestigio que ya
se limpió en la Etapa 1/2 — por eso `AuthClaims` vive en **`iam`**, no en
`auth/session/jwt` (que sí es donde vive `GenerateAPIToken`, un caso
distinto: ese token solo lleva identidad, nunca roles).

## Qué se construye — verificado contra el código real de `tinywasm/jwt`

`Claims{Sub, Exp, Iat}` con su `EncodeFields`/`DecodeFields` en
`tinywasm/jwt/jwt.go` **no cambia**. Se añade, en el mismo archivo, de
forma estrictamente aditiva:

```go
// BaseClaims is satisfied by any payload that carries the fields Sign/
// Verify need to check expiry and refuse an empty subject — regardless of
// whatever extra fields the caller's payload adds. Claims satisfies it
// trivially; a type that embeds Claims satisfies it for free (Go method
// promotion), with zero repeated code.
type BaseClaims interface {
	model.Encodable
	Base() Claims
}

// Base lets Claims itself satisfy BaseClaims.
func (c Claims) Base() Claims { return c }

// SignPayload signs any BaseClaims payload — Sign(secret, Claims) stays
// the shortcut for the common case (Claims alone), unchanged, calling
// this internally.
func SignPayload(secret []byte, payload BaseClaims) (string, error) {
	base := payload.Base()
	if base.Sub == "" {
		return "", ErrEmptySubject
	}
	if len(secret) == 0 {
		return "", ErrEmptySecret
	}
	var h string
	if err := json.Encode(header{Alg: algHS256, Typ: typJWT}, &h); err != nil {
		return "", err
	}
	var p string
	if err := json.Encode(payload, &p); err != nil {
		return "", err
	}
	signingInput := base64.URLEncode([]byte(h)) + "." + base64.URLEncode([]byte(p))
	return signingInput + "." + sign(secret, signingInput), nil
}

// VerifyPayload authenticates a token and decodes it into `into` (any
// BaseClaims — call with a pointer, e.g. &AuthClaims{}). Same two-channel
// contract as Verify: error means the CALLER is broken, Outcome means what
// the TOKEN is.
func VerifyPayload(secret []byte, token string, into BaseClaims) (Outcome, error) {
	if len(secret) == 0 {
		return Forged, ErrEmptySecret
	}
	parts := fmt.Split(token, ".")
	if len(parts) != 3 {
		return Forged, nil
	}
	expected := sign(secret, parts[0]+"."+parts[1])
	if !hmac.HMACEqual([]byte(parts[2]), []byte(expected)) {
		return Forged, nil
	}
	raw, err := base64.URLDecode(parts[1])
	if err != nil {
		return Forged, nil
	}
	dec, ok := into.(model.Decodable)
	if !ok {
		return Forged, fmt.Err("jwt", "payload", "not-decodable")
	}
	if err := json.Decode(string(raw), dec); err != nil {
		return Forged, nil
	}
	base := into.Base()
	if base.Exp <= 0 || base.Sub == "" {
		return Forged, nil
	}
	if now() > base.Exp+Leeway {
		return Expired, nil
	}
	return Valid, nil
}
```

`into` debe ser un pointer que implementa tanto `BaseClaims` (para leer
`Base()` tras decodificar) como `model.Decodable` (para que `DecodeFields`
lo pueble) — `*AuthClaims` en la Etapa 3 satisface ambos por embedding, ver
§1.2.

**No dupliques la lógica de `Sign`/`Verify` existentes.** `SignPayload`/
`VerifyPayload` son una generalización; si al implementarlos notas que
`Sign`/`Verify` podrían reescribirse en términos de estas dos (`Sign(secret,
c) == SignPayload(secret, c)` porque `Claims` ya satisface `BaseClaims`),
hazlo — menos código, mismo comportamiento, y una sola ruta que probar.

---

## Archivos a crear/modificar

```
tinywasm/jwt/jwt.go              // modificar: BaseClaims, Base(), SignPayload, VerifyPayload
veltylabs/iam/config/authclaims.go   // nuevo
veltylabs/iam/config/projects.go     // nuevo — modelo Project (client credentials)
veltylabs/iam/config/token.go        // nuevo — emisión del token de autorización
veltylabs/iam/routes/routes.go       // nuevo — primera vez que iam tiene rutas HTTP
veltylabs/iam/web/server.go          // nuevo — servidor de desarrollo
veltylabs/iam/edge/main.go           // nuevo — Worker de producción
veltylabs/iam/tests/token_test.go    // nuevo
```

## Pasos

### 3.1 — `veltylabs/iam/config/authclaims.go`

```go
package config

import (
	"github.com/tinywasm/jwt"
	"github.com/tinywasm/model"
)

// AuthClaims is the authorization token's payload: identity (embedded
// jwt.Claims) plus the project-scoped roles it grants. It lives here, not
// in tinywasm/auth, because auth never imports rbac — only this
// composition root sees both (see ARCHITECTURE.md §6.2).
type AuthClaims struct {
	jwt.Claims
	ProjectID string
	Roles     []string
}

func (c AuthClaims) EncodeFields(w model.FieldWriter) {
	c.Claims.EncodeFields(w)
	w.String("project_id", c.ProjectID)
	arr := w.Array("roles", len(c.Roles))
	for _, r := range c.Roles {
		arr.String(r)
	}
	arr.Close()
}

func (c *AuthClaims) DecodeFields(r model.FieldReader) {
	c.Claims.DecodeFields(r)
	c.ProjectID, _ = r.String("project_id")
	if ar, ok := r.Array("roles"); ok {
		c.Roles = make([]string, ar.Len())
		for i := 0; i < ar.Len(); i++ {
			c.Roles[i] = ar.String(i)
		}
	}
}
```

`IsNil`/`Base` llegan gratis por el embedding de `jwt.Claims` (promoción de
métodos) — no los reescribas.

### 3.2 — `veltylabs/iam/config/projects.go`

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

### 3.3 — `veltylabs/iam/config/token.go`

Orquesta auth (identidad ya resuelta por la cookie/sesión activa) + rbac
(roles del proyecto) + el `SessionTTL` más restrictivo, y firma con
`jwt.SignPayload` — **no** con `tinywasm/auth/session/jwt.GenerateAPIToken`
(ese es para tokens sin roles, ver ARCHITECTURE.md §6.1).

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
// que usa el resto de la sesion de iam.
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
	claims := jwt.NewClaims(userID, ttl)
	return jwt.SignPayload(secret, &AuthClaims{Claims: claims, ProjectID: projectID, Roles: roleCodes})
}
```

Esto asume `rbac.Role` ya tiene el campo `SessionTTL int64` — agrégalo en
`tinywasm/rbac/models.go` (`RoleModel`, campo nuevo `session_ttl`,
`Type: model.Int()`, sin `NotNull` — 0 es un valor válido que significa
"usa el default"), corre `ormc`, publica una nueva versión de
`tinywasm/rbac` con `gopush`, y actualiza la dependencia en `iam` antes de
escribir este archivo. **No dupliques el campo en otro lado**: `SessionTTL`
vive en `Role`, no en `AuthClaims` ni en `Project`.

### 3.4 — Rutas (`veltylabs/iam/routes/routes.go`)

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

### 3.5 — `web/server.go` / `edge/main.go`

Mismo patrón que `misitio/web/server.go`/`edge/main.go` (ya leídos y
portados en la Etapa 1): construir el backend (auth+rbac+projects),
inyectar `Authn: authMod.Authenticate()`, montar `routes.Register(...)`.
`edge/main.go` necesita su propia D1 — **no reutiliza `veltydb` de
`misitio`**: `iam` tiene su propia base (ver `AGENTS.md` Restricción #5).
Provisionar esa D1 y su binding es un paso de infraestructura, no de
código — coordínalo con el mantenedor antes de desplegar, no lo asumas.

---

## Reglas de calidad

- **`tinywasm/jwt` no gana ningún import nuevo hacia `model`-adyacentes que
  no tuviera ya** (`model.Encodable`/`Decodable` ya eran parte de su
  superficie vía `Claims`). Si `SignPayload`/`VerifyPayload` te piden un
  import que el paquete no tenía, es señal de que el diseño se desvió del
  de este plan — revisa antes de agregarlo.
- **`AuthClaims` no vive en `tinywasm/auth`.** Si en algún punto sientes
  la tentación de moverlo ahí "porque ya existe `GenerateAPIToken`", relee
  §6.2 de `ARCHITECTURE.md` — es la regla `auth` nunca importa `rbac`,
  no una preferencia de organización de archivos.
- **`client_secret` nunca se guarda en claro** — solo su hash (`bcrypt`,
  mismo mecanismo que `email_password`). `CreateProject` lo devuelve UNA
  vez, en el momento de creación; no hay un `GetProjectSecret`.
- **Sin duplicar la constante de TTL**: `DefaultAuthTokenTTL` vive una sola
  vez en `config/token.go`.

## Criterios de aceptación

- [ ] `go test ./...` en `tinywasm/jwt`, `tinywasm/rbac` e `iam`, todo verde.
- [ ] Un test en `tinywasm/jwt` prueba `SignPayload`/`VerifyPayload` con un
      tipo de payload custom (no solo `Claims`) — la doctrina "an API is
      not published until a consumer-shaped test proves it"
      (`tinywasm/sitec/docs/CONSTRUCTION_HARNESS.md`) aplica aquí: si es
      incómodo de escribir, el diseño tiene un defecto, no el test.
- [ ] Un test en `iam/tests` prueba el flujo completo: usuario con un rol
      que tiene `SessionTTL` propio → `IssueAuthToken` usa ESE TTL, no el
      default.
- [ ] Un test prueba que `POST /api/token` con `client_secret` incorrecto
      devuelve 403, y que el `userID` del token emitido es siempre el de
      la sesión (`ctx.UserID()`), nunca el que llegue en el body si
      alguien intenta mandarlo.
- [ ] `grep -rn "Roles\s*\[\]string\|ProjectID" /home/cesar/Dev/Project/tinywasm/auth --include="*.go"` → vacío: `auth` sigue sin conocer roles ni proyectos.

## Fuera de alcance

- Revocación explícita de tokens (ver ARCHITECTURE.md §6.3 — TTL corto +
  renovación es la decisión tomada).
- La cookie de identidad cross-dominio — es la Etapa 4, independiente de
  esta.
- El panel de administración para crear proyectos/roles desde una UI — hoy
  `CreateProject`/`rbacSvc.CreateRole` se llaman directo (script, test, o
  a mano); la UI es un tema aparte sin diseñar todavía.

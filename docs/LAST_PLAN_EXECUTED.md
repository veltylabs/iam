---
PLAN: "feat(client): el middleware de identidad vive en iam, no en cada consumidor"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha con el flujo CodeJob. Ver skill: agents-workflow.

# Plan — que consumir `iam` sea una línea, no un archivo por proyecto

## Por qué existe este plan

`client/` hoy entrega dos llamadas HTTP sueltas (`FetchAuthzToken`,
`ResolveUser`) y nada más. Todo lo que hace falta para **usarlas de verdad** lo
reescribe cada consumidor. En `veltylabs/misitio` eso son tres archivos que no
tienen ni una línea de misitio dentro:

| Archivo en misitio | Qué hace | De quién es la responsabilidad |
|---|---|---|
| `config/iamauth.go` | middleware: lee la cookie SSO → `FetchAuthzToken` → `SetUserID` → cachea | de `iam` |
| `authctx/authctx.go` | guarda la `Identity` en el `router.Context` (8 claves, porque el store es solo-strings) | de `iam` |
| `config/authzcache.go` | cachea el scope por usuario para que la compuerta no llame a iam en cada petición | de `iam` |

Ese `authctx/` existe además por un ciclo de imports de misitio, no por diseño:
es un paquete hoja creado para que `config/` y `routes/` pudieran compartir la
identidad. El próximo proyecto que consuma `iam` lo va a reescribir entero, con
sus propias claves de contexto y sus propios bugs.

**Lo que NO se mueve:** la política. `iam` devuelve **códigos de rol**; qué
autoriza cada código es del consumidor
([`docs/ARCHITECTURE.md`](ARCHITECTURE.md) §3.4). Este plan sube el transporte y
la sesión; la tabla `rol → permiso` se queda donde está.

## Anti-footguns

- `client/` se compila **dentro del binario edge (WASM, TinyGo) del
  consumidor**. Sin `map[K]V`, sin `strings`/`strconv`/`errors`/`fmt` de la
  stdlib (usa `tinywasm/fmt`), sin `reflect`. Lo dice el encabezado del propio
  paquete: un consumidor no arrastra `tinywasm/orm` ni `tinywasm/rbac` por
  preguntar quién es un usuario — **eso no se puede romper.**
- `client/` es lo único de este repo que importan otros. No metas aquí nada de
  `config/`, `routes/` ni `edge/`.
- El `client_secret` **nunca** viaja al navegador. Todo lo que agrega este plan
  corre en el servidor del consumidor.
- El repo es de `veltylabs`: código e identificadores en inglés, comentarios y
  documentación en español.

---

## Etapa 1 — La identidad en el contexto

Archivo nuevo: **`client/context.go`**.

Es el contenido de `misitio/authctx/authctx.go`, portado tal cual. **Copia el
código real, no lo describas ni lo reescribas de memoria** — está reproducido
íntegro al final de este plan, en "Código de referencia".

```go
// SetIdentity guarda en el contexto lo que iam resolvió para esta petición.
func SetIdentity(ctx router.Context, id Identity)

// FromContext lee lo que SetIdentity guardó. ok es false cuando nunca se
// llamó a SetIdentity en esta petición.
func FromContext(ctx router.Context) (Identity, bool)
```

Las claves son constantes **no exportadas** del paquete. El consumidor no las
necesita: su contrato son las dos funciones. Exportarlas invita a que alguien
escriba una identidad a mano.

Conserva el comentario que explica *por qué* la `Identity` viaja como ocho
claves escalares y no como un blob: `router.Context` tiene un almacén
solo-strings, y `Scope` (el único campo de slice) va y vuelve por un join/split
con coma, seguro porque un código de rol nunca contiene una coma. Ese
razonamiento es la mitad del valor del archivo.

## Etapa 2 — El caché de scope

Archivo nuevo: **`client/cache.go`**.

Es `misitio/config/authzcache.go`, portado tal cual (también reproducido al
final). Renombra el tipo a `scopeCache` y hazlo **no exportado**: nadie lo
construye por su cuenta, lo crea `New` en la etapa 3. Un caché exportado es un
caché que alguien va a llenar a mano.

Mantén intacto:

- Sin `map[K]V` — slice y barrido lineal. El comentario dice por qué (compila
  al Worker; los usuarios concurrentes de un panel son decenas).
- `sync.RWMutex`. Sí, `sync` es stdlib y sí está permitida aquí: TinyGo la
  compila y el paquete ya la necesita. **No la "arregles".**
- El barrido de entradas vencidas en cada `Set`.

## Etapa 3 — `Consumer`: lo que un proyecto instancia una vez

Archivo nuevo: **`client/consumer.go`**.

```go
// Nombres convencionales de las variables de entorno. Están aquí para que
// todos los consumidores usen las mismas y nadie invente un tercer nombre.
const (
	EnvBaseURL      = "IAM_BASE_URL"
	EnvClientSecret = "IAM_CLIENT_SECRET"
)

// Config es lo que un proyecto necesita para hablarle a iam.
type Config struct {
	BaseURL      string // https://iam.velty.cl, o el servidor de desarrollo
	ProjectID    string // identifica al proyecto ante iam
	ClientSecret string // NUNCA llega al navegador
}

// Consumer es un proyecto que consume iam. Autentica peticiones contra iam y
// recuerda el scope que iam devolvió, para que el autorizador del proyecto lo
// lea sin un segundo viaje.
//
// Un proyecto crea UNO al arrancar y lo comparte entre su servidor local y su
// Worker: son el mismo objeto con la misma configuración, y ahí está la razón
// de que exista — antes cada punto de entrada armaba el middleware por su
// cuenta, con cuatro argumentos que había que mantener sincronizados a mano.
type Consumer struct {
	cfg   Config
	cache *scopeCache
}

// New valida la configuración y falla rápido si falta algo. Arrancar sin poder
// hablarle a iam es peor que no arrancar: cada ruta protegida respondería 403
// para siempre y nadie sabría por qué.
func New(cfg Config) (*Consumer, error)

// ConfigFromEnv arma una Config leyendo EnvBaseURL y EnvClientSecret.
// projectID es del proyecto, no del entorno: es una constante suya, no algo
// que se despliega distinto por ambiente.
func ConfigFromEnv(projectID string) Config

// Authn identifica al llamante. Reemplaza a authority.Authenticate() en un
// proyecto que delega su identidad en iam.
//
// Una cookie SSO ausente o rechazada es lo NORMAL (sesión vencida, nadie ha
// entrado todavía): deja al llamante anónimo y sigue. Nunca es un 500 ni un
// error que se le muestre a la petición.
func (c *Consumer) Authn() router.Middleware

// Scope devuelve los códigos de rol vigentes que iam entregó para userID.
// ok es false cuando no hay entrada vigente — y eso DENIEGA: la ausencia de
// una respuesta no es un permiso.
//
// Devuelve códigos, no permisos. Qué autoriza cada código es política del
// proyecto, no de iam (ver docs/ARCHITECTURE.md §3.4).
func (c *Consumer) Scope(userID string) ([]string, bool)
```

Mensajes de error, textuales, como constantes:

```go
const (
	ErrMsgMissingBaseURL      = "iam client: Config.BaseURL is required"
	ErrMsgMissingProjectID    = "iam client: Config.ProjectID is required"
	ErrMsgMissingClientSecret = "iam client: Config.ClientSecret is required"
)
```

`Authn` es exactamente el cuerpo de `misitio/config/iamauth.go` (al final del
plan), con `c.cfg` en lugar de los cuatro parámetros sueltos y `c.cache` en
lugar del caché inyectado.

**`Consumer` no expone un `Authorize`.** Sería la política, y la política es del
consumidor. El proyecto escribe su propio `func(userID, resource, action) bool`
que llama a `Scope` y la cruza con su tabla. Ese reparto ya está decidido en
`ARCHITECTURE.md` §3.4 y este plan no lo cambia.

## Etapa 3.b — Asignar un rol a un usuario de un proyecto consumidor

`config/bootstrap.go` expone `EnsureRole(rbacSvc, authMod, projectID, roleID, roleCode, roleName, roleDescription, emails)`:
siembra un rol y se lo asigna a una **lista de correos conocida de antemano**. Eso sirve para
sembrar administradores, y no sirve para lo que un consumidor necesita: darle un rol a un
usuario que acaba de aparecer.

`veltylabs/misitio` lo necesita ya — cuando un administrador acepta una solicitud y crea la
membresía, tiene que concederle a ese usuario el rol base de su proyecto. Sin eso, toda ruta
protegida de misitio responde `403` para siempre (el hallazgo que abrió esta ola).

Añade a **`client/consumer.go`**:

```go
// AssignRole concede roleCode al usuario en ESTE proyecto. Idempotente: si ya
// lo tiene, no es un error.
//
// Es la contraparte de Scope: un consumidor no solo lee los roles que iam le
// entrega, tambien necesita concederlos cuando su propio dominio decide que
// alguien pasa a tener acceso. Sin esto, el consumidor solo podria sembrar
// roles por lista de correos conocida de antemano.
func (c *Consumer) AssignRole(userID string, roleCode model.RoleCode) error
```

y del otro lado el endpoint que la atiende, junto a los que ya sirven
`FetchAuthzToken`/`ResolveUser`, autenticado con el mismo `client_secret` y **acotado al
`project_id` del que llama**: un consumidor jamás concede un rol en otro proyecto. Si el rol no
existe en ese proyecto, se crea (mismo criterio idempotente que `EnsureRole`).

Mensajes de error, textuales:

```go
const (
	ErrMsgAssignRoleUserRequired = "iam client: AssignRole: userID is required"
	ErrMsgAssignRoleCodeRequired = "iam client: AssignRole: roleCode is required"
)
```

**No** expongas una variante que reciba `projectID`: el proyecto es el del `Consumer`, y un
parámetro ahí es una escalada de privilegios esperando a ocurrir.

## Etapa 4 — Tests

Bajo **`tests/`**, junto a `client_test.go`.

Archivo nuevo **`tests/consumer_test.go`**:

| Test | Afirma |
|---|---|
| `TestNewRejectsEmptyConfig` | los tres campos vacíos, uno por uno, devuelven el mensaje exacto |
| `TestAuthnAnonymousWithoutCookie` | sin cookie SSO el handler siguiente corre con `ctx.UserID() == ""` y **no** hay error |
| `TestAuthnAnonymousWhenIAMRejects` | con `BaseURL` inalcanzable, el handler siguiente corre igual, anónimo |
| `TestIdentityRoundTrip` | `SetIdentity` + `FromContext` devuelve la misma `Identity`, con `Scope` de dos elementos intacto |
| `TestFromContextWithoutIdentity` | contexto limpio → `ok` es false |
| `TestScopeDeniesWhenAbsent` | `Scope` de un usuario desconocido → `ok` false |
| `TestScopeDeniesWhenExpired` | entrada con `expires` en el pasado → `ok` false |
| `TestScopeOverwritesSameUser` | dos `Set` del mismo usuario dejan **una** entrada, la última |
| `TestAssignRoleRejectsEmptyArgs` | `userID` o `roleCode` vacíos devuelven el mensaje exacto |
| `TestAssignRoleIsIdempotent` | asignar dos veces el mismo rol no es un error |
| `TestAssignRoleIsScopedToTheProject` | la petición que sale lleva el `project_id` del `Consumer`, y no hay forma de pasar otro |

Usa `github.com/tinywasm/router/mock` para los contextos: probar un
`router.Middleware` sin router prueba la función, no el middleware.

## Etapa 5 — Limpieza de planes huérfanos

`docs/` arrastra tres archivos de un ciclo ya cerrado:

```
docs/PLAN_STAGE_1_HEALTH.md
docs/PLAN_STAGE_1_PORT_IDENTITY.md
docs/PLAN_STAGE_2_EDGE_SLIMMING.md
```

Los tres empiezan con una línea de navegación que apunta a `docs/PLAN.md` — un
archivo que `codejob` borró al publicar, y que `AGENTS.md` prohíbe enlazar
justamente por esto. Además `PLAN_STAGE_2` apunta a `PLAN_STAGE_3_ACTION.md`,
que tampoco existe.

**Bórralos.** Su contenido ya está en el código y en el historial de git; lo
único que aportan hoy son cuatro enlaces muertos. Cualquier razonamiento de
diseño que valga la pena conservar va a `docs/ARCHITECTURE.md` **antes** de
borrarlos, no se pierde con ellos.

## Etapa 6 — Documentación

- **`docs/ARCHITECTURE.md`** — en la sección del cliente, describe `Consumer`
  como la forma de consumir iam, con el ejemplo de cuatro líneas de abajo.
  Refuerza el reparto: iam entrega **códigos de rol**, el consumidor entrega la
  política. Ese párrafo es el que evita que el próximo proyecto pida "que iam
  devuelva permisos".
- **`README.md`** — la sección de uso desde otro proyecto pasa a ser:

  ```go
  iam, err := iamclient.New(iamclient.ConfigFromEnv(ProjectID))
  if err != nil { /* no arrancar */ }

  r := edge.NewRouter(edge.Config{
      Authn:     iam.Authn(),
      Authorize: myPolicy(iam),   // política del proyecto, no de iam
  })
  ```

- **No enlaces `docs/PLAN.md`** desde ningún documento permanente.

## Criterios de aceptación

- [ ] `go build ./...` y `go vet ./...` limpios.
- [ ] `GOOS=js GOARCH=wasm go vet ./edge/... ./client/...` limpios.
- [ ] `gotest ./...` en verde.
- [ ] `grep -rn "map\[" client/` → vacío.
- [ ] `grep -rn "\"strings\"\|\"strconv\"\|\"errors\"\|\"reflect\"" client/` →
      vacío; el único `fmt` es `github.com/tinywasm/fmt`.
- [ ] `grep -rn "orm\.\|rbac\." client/` → vacío: el consumidor sigue sin
      arrastrar la maquinaria de iam.
- [ ] `ls docs/PLAN_STAGE_*.md` → no existe ninguno.
- [ ] `grep -rn "PLAN.md" docs/*.md README.md` → vacío.
- [ ] `Consumer` **no** tiene método `Authorize`.
- [ ] `AssignRole` no acepta un `projectID`: usa el del `Consumer`.
- [ ] `grep -rn "AssignRole" client/` muestra la firma sin `projectID`.
- [ ] `docs/ARCHITECTURE.md` y `README.md` muestran el uso con `Consumer`.

## Fuera de alcance

La política de roles (es del consumidor), cualquier cambio a `FetchAuthzToken`
o `ResolveUser`, y todo lo que esté fuera de `client/`, `tests/` y `docs/`.

## Etapas

| # | Etapa | Archivos |
|---|---|---|
| 1 | Identidad en el contexto | `client/context.go` |
| 2 | Caché de scope | `client/cache.go` |
| 3 | `Consumer` | `client/consumer.go` |
| 3.b | `AssignRole` + su endpoint | `client/consumer.go`, `routes/`, `config/` |
| 4 | Tests | `tests/consumer_test.go` |
| 5 | Limpieza de planes huérfanos | `docs/PLAN_STAGE_*.md` |
| 6 | Documentación | `docs/ARCHITECTURE.md`, `README.md` |

---

## Código de referencia

Esto es el código **real** que hoy vive en `veltylabs/misitio` y que este plan
sube a `client/`. Pórtalo, no lo reinventes. Lo único que cambia son los
nombres de paquete y que los cuatro parámetros sueltos pasan a ser campos de
`Consumer`.

### `misitio/authctx/authctx.go` → `client/context.go`

```go
const (
	ctxKeySub    = "iam_identity_sub"
	ctxKeyExp    = "iam_identity_exp"
	ctxKeyIat    = "iam_identity_iat"
	ctxKeyAud    = "iam_identity_aud"
	ctxKeyScope  = "iam_identity_scope"
	ctxKeyEmail  = "iam_identity_email"
	ctxKeyName   = "iam_identity_name"
	ctxKeyAvatar = "iam_identity_avatar"

	scopeSep = ","
)

func SetIdentity(ctx router.Context, id Identity) {
	ctx.SetValue(ctxKeySub, id.Claims.Sub)
	ctx.SetValue(ctxKeyExp, fmt.Convert(id.Claims.Exp).String())
	ctx.SetValue(ctxKeyIat, fmt.Convert(id.Claims.Iat).String())
	ctx.SetValue(ctxKeyAud, id.Claims.Aud)
	ctx.SetValue(ctxKeyScope, fmt.Convert(id.Claims.Scope).Join(scopeSep).String())
	ctx.SetValue(ctxKeyEmail, id.Email)
	ctx.SetValue(ctxKeyName, id.Name)
	ctx.SetValue(ctxKeyAvatar, id.Avatar)
}

func FromContext(ctx router.Context) (Identity, bool) {
	sub := ctx.Value(ctxKeySub)
	if sub == "" {
		return Identity{}, false
	}

	exp, _ := fmt.Convert(ctx.Value(ctxKeyExp)).Int64()
	iat, _ := fmt.Convert(ctx.Value(ctxKeyIat)).Int64()
	var scope []string
	if s := ctx.Value(ctxKeyScope); s != "" {
		scope = fmt.Split(s, scopeSep)
	}

	return Identity{
		Claims: tinyjwt.Claims{
			Sub:   sub,
			Exp:   exp,
			Iat:   iat,
			Aud:   ctx.Value(ctxKeyAud),
			Scope: scope,
		},
		Email:  ctx.Value(ctxKeyEmail),
		Name:   ctx.Value(ctxKeyName),
		Avatar: ctx.Value(ctxKeyAvatar),
	}, true
}
```

`Identity` y `tinyjwt` ya viven en este paquete: dentro de `client/` se
escriben `Identity` y `tinyjwt.Claims` sin prefijo de paquete ajeno.

### `misitio/config/iamauth.go` → el cuerpo de `Consumer.Authn`

```go
func IAMAuthn(iamBaseURL, projectID, clientSecret string, cache *AuthzCache) router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx router.Context) {
			cookie, ok := ctx.Cookie(iamclient.SSOCookieName)
			if !ok {
				next(ctx)
				return
			}
			id, err := iamclient.FetchAuthzToken(iamBaseURL, projectID, clientSecret, cookie.Value)
			if err != nil {
				next(ctx)
				return
			}
			ctx.SetUserID(id.Claims.Sub)
			authctx.SetIdentity(ctx, id)
			cache.Set(id.Claims.Sub, id.Claims.Scope, id.Claims.Exp)
			next(ctx)
		}
	}
}
```

### `misitio/config/authzcache.go` → `client/cache.go`

```go
type authzEntry struct {
	userID  string
	scope   []string
	expires int64
}

type AuthzCache struct {   // → scopeCache, no exportado
	mu      sync.RWMutex
	entries []authzEntry
}

func NewAuthzCache() *AuthzCache { return &AuthzCache{} }

func (c *AuthzCache) Set(userID string, scope []string, expires int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := nowUnix()
	kept := c.entries[:0]
	for _, e := range c.entries {
		if e.expires > now && e.userID != userID {
			kept = append(kept, e)
		}
	}
	c.entries = append(kept, authzEntry{userID: userID, scope: scope, expires: expires})
}

func (c *AuthzCache) Scope(userID string) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := nowUnix()
	for _, e := range c.entries {
		if e.userID == userID && e.expires > now {
			return e.scope, true
		}
	}
	return nil, false
}

func nowUnix() int64 { return time.Now() / 1e9 }
```

`time` es `github.com/tinywasm/time`, **no** la stdlib: `time.Now()` devuelve
nanosegundos como `int64`, de ahí el `/ 1e9`.

---
PLAN: "fix(security): close the open redirect, add security headers, block sibling-subdomain CSRF, audit denials"
EXECUTOR: jules
REVIEWER: none
---

> Este plan se despacha con el flujo CodeJob. Ver skill: `agents-workflow`.
> No ejecutes `gopush` ni `codejob` — son herramientas del desarrollador local.
>
> **NO DESPACHAR TODAVÍA.** Este archivo espera a que se mergee el PR #6 (el
> plan del panel de administración, hoy con `STATUS: review` en `docs/PLAN.md`).
> Cuando eso pase, se renombra a `docs/PLAN.md` y recién ahí se despacha.
> Depende además de que estén publicadas las versiones nuevas de
> `tinywasm/dom`, `tinywasm/auth`, `tinywasm/rbac`, `tinywasm/jwt`,
> `tinywasm/crypto` y `tinywasm/router` — ver
> [plan maestro](https://github.com/tinywasm/docs/blob/main/IAM_SECURITY_HARDENING_MASTER_PLAN.md).

# PLAN — `veltylabs/iam`: endurecimiento para producción

## Contexto

Auditoría de seguridad completa de `iam` y de su cadena aguas arriba
(2026-09-02). Los hallazgos de las librerías compartidas se arreglan en sus
propios repos; **este plan sólo cubre lo que es de `iam`**.

Leé antes de escribir código:

- [`AGENTS.md`](../AGENTS.md) — las restricciones de este repo.
- [`docs/ARCHITECTURE.md`](ARCHITECTURE.md) — el diseño.
- [CONSTRUCTION_HARNESS.md](https://github.com/tinywasm/app-releases/blob/main/docs/CONSTRUCTION_HARNESS.md)
  — la doctrina del ecosistema.

Las dos reglas de `AGENTS.md` que más veces se violan en un plan como éste:

1. **Restricción #1 — este repo no tiene lógica de identidad propia.** Es una
   raíz de composición sobre `tinywasm/user` + `auth` + `rbac`.
2. **Restricción #2 — nunca recrear localmente un símbolo que falta.** Si una
   librería no expone lo que hace falta, se para y se arregla aguas arriba.
   Este plan ya consumió esa regla: los arreglos upstream están hechos y
   publicados. **Usalos; no vuelvas a escribir su equivalente local.**

## Compuertas (versiones ya publicadas antes de empezar)

`go get` a la última versión publicada de cada una, y **borrá el código local
que reemplazan**:

| Dependencia | Símbolo que este plan usa | Reemplaza a |
|---|---|---|
| `tinywasm/router` | `router.QueryParam(path, key)` | `getQueryParam` en `modules/admin/handler.go` |
| `tinywasm/rbac` | `RevokeRoleByCode`, `AssignRoleByCode`, `UsersInRole`, `RoleUserCount`, `DeleteRoleByCode`, `ErrRoleNotFound`, `ErrDuplicateRoleCode` | el `db.Delete(&rbac.UserRole{}, …)` a mano |
| `tinywasm/jwt` | `Claims.Expired()`, `Claims.AllowsAudience(aud)`, `DecodeFresh` | los chequeos que hoy no existen en `client/` |
| `tinywasm/dom` | `Text()` ya escapa; `Trust`/`Raw` para marcado deliberado | nada — el panel se vuelve seguro solo |
| `tinywasm/auth` | firma nueva de `StateStore` (state+nonce) | nada en `iam`, pero hay que recompilar |
| `tinywasm/crypto` | `rand.Secret()`, `rand.SecretN(n)` | el `make([]byte,30)+Read+encode` de `config/projects.go` |

---

## Hallazgo I-1 (Alto) · Open redirect en `isVeltyDomain`

`config/auth.go`:

```go
func isVeltyDomain(url string) bool {
	const prefix = "https://"
	if !fmt.HasPrefix(url, prefix) { return false }
	host := url[len(prefix):]
	if idx := fmt.Index(host, "/"); idx >= 0 { host = host[:idx] }
	return host == SSOCookieDomain[1:] || fmt.HasSuffix(host, SSOCookieDomain)
}
```

Corta el host **sólo en `/`**. Un navegador termina el host también en `?`, en
`#`, en `\` (que normaliza a `/`) y en `@` (userinfo).

**Reproducido** con la función real:

```
"https://evil.com#.velty.cl"    -> true
"https://evil.com\.velty.cl"    -> true
"https://evil.com?x.velty.cl"   -> true
"https://evil.com/x"            -> false
```

El comentario del propio archivo dice *"isVeltyDomain es el único guardián
contra un open-redirect"*. Ese guardián se salta con un `#`.

Impacto: un atacante manda a la víctima a
`https://iam.velty.cl/oauth/google?redirect_uri=https://evil.com%23.velty.cl`;
la víctima se loguea de verdad en `iam` y el navegador termina en `evil.com`,
que ahora es una página de aterrizaje creíble post-login (phishing), y recibe
el `Referer` de `iam.velty.cl`.

## Hallazgo I-2 (Alto) · Ninguna cabecera de seguridad

Ni `Content-Security-Policy`, ni `X-Frame-Options`/`frame-ancestors`, ni
`X-Content-Type-Options`, ni `Referrer-Policy`, ni `Strict-Transport-Security`.
El panel de administración de un servicio de identidad se puede meter en un
`<iframe>` de cualquier sitio (clickjacking), y una CSP habría contenido el
XSS de `tinywasm/dom` aunque el escapado hubiera fallado.

## Hallazgo I-3 (Alto) · CSRF desde un subdominio hermano

La cookie SSO se emite con `Domain=".velty.cl"` (`config/auth.go`,
`SSOCookieDomain`) y `SameSite=Strict`. `SameSite` razona por **sitio**, no
por origen: `misitio.velty.cl` es *same-site* respecto de `iam.velty.cl`. O
sea que cualquier página servida desde **cualquier** subdominio de
`velty.cl` — un consumidor comprometido, un XSS en `misitio`, un subdominio
abandonado — puede disparar `POST /admin/api/projects/rotate` con la cookie
del administrador adjunta.

`routes/routes.go` documenta correctamente por qué no hay CORS (el llamador es
siempre un servidor). Pero la ausencia de CORS **no protege**: CORS gobierna
la lectura de la respuesta, no el envío de la petición. Y no hay token CSRF ni
validación de `Origin`.

## Hallazgo I-4 (Medio) · Revocación que no invalida el caché

`modules/admin/handler.go`, `RevokeUserHandler`, borra la fila a mano en vez
de usar el `Service`:

```go
if err := db.Delete(&rbac.UserRole{},
	storage.Eq(rbac.UserRole_.ProjectId, req.ProjectId), …); err != nil {
```

Eso saltea `rbac.Service.RevokeRole` y por lo tanto **no invalida `ucache`**:
los permisos revocados se siguen concediendo hasta el desalojo FIFO.

Relacionado: `CreateRoleHandler` no valida que el `code` no exista ya, y
`AssignRole` en `routes/routes.go` crea el rol usando el `code` como `id`
mientras el panel usa `ids.NewID()` — los dos caminos pueden crear el mismo
`code` con `id` distinto.

## Hallazgo I-5 (Medio) · `hashProjectSecret` acepta secreto vacío

`config/projects.go`:

```go
func projectSecretKey() []byte {
	return hmac.HMACSHA256([]byte(env.Get(EnvJWTSecret)), []byte(projectSecretKeyLabel))
}
```

Si `JWT_SECRET` no está, esto no falla: hace HMAC de la cadena vacía y
devuelve una clave **fija y derivable por cualquiera**. `tinywasm/jwt` ya tomó
la decisión correcta para el mismo problema (`ErrEmptySecret`: *"HMAC over an
empty key is valid math: it produces a token that verifies"*); acá falta.

Además `projectSecretKey()` se recalcula en **cada** verificación, y en wasm
`env.Get` es un viaje por `syscall/js` por llamada.

## Hallazgo I-6 (Medio) · La auditoría no registra denegaciones

`config/audit.go` registra las nueve mutaciones exitosas. No registra ningún
intento fallido: ni el 403 de `RequirePanelAdmin` (alguien con sesión válida
que no es admin tocando el panel), ni el 403 de `/api/token` por
`client_secret` inválido. Un servicio de identidad cuyo registro sólo tiene
éxitos no sirve para investigar nada.

## Hallazgo I-7 (Medio) · `getQueryParam` recreado localmente

`modules/admin/handler.go` tiene su propio parser de query string, con los
mismos defectos que tenía el de `tinywasm/auth`: exige `len(kv)==2` y no
percent-decodifica. Es exactamente lo que prohíbe la Restricción #2.

## Hallazgo I-8 (Robustez) · `/admin/api/*` fuera de la convención worker-first

`tinywasm/goflare` declara `WorkerFirstRoutes = []string{"/api/*", "/oauth/*"}`
como convención del ecosistema, y no expone forma de extenderla. Las rutas del
panel de `iam` viven bajo `/admin/api/*`, fuera de esa lista, así que su
llegada al Worker depende de la precedencia entre el router de assets estáticos
y el script — que además está configurado con
`not_found_handling: single-page-application`.

**No lo doy por explotado ni por roto: hay que verificarlo.** Pero la
dependencia es innecesaria y se elimina sin costo alineándose con la
convención.

---

## Etapa 1 · Cerrar el open redirect (I-1)

Archivo: `config/auth.go`.

Reescribir `isVeltyDomain` para extraer el host **igual que lo haría un
navegador**. Constantes nuevas, ninguna literal suelta:

```go
const (
	httpsPrefix = "https://"
	// hostTerminators son los caracteres que terminan el host en una URL tal
	// como la interpreta un navegador. Cortar sólo en "/" — lo que hacía la
	// versión anterior — deja pasar https://evil.com#.velty.cl: el sufijo
	// ".velty.cl" queda dentro del "host" para esta función y fuera de él
	// para el navegador, que va a evil.com.
	hostTerminators = "/?#\\"
	// userInfoSep separa userinfo@host. Todo lo que está ANTES es del
	// atacante: https://x.velty.cl@evil.com apunta a evil.com.
	userInfoSep = '@'
)
```

Algoritmo obligatorio, en este orden:

1. Rechazar si contiene un byte de control (`< 0x20` o `0x7F`). Un `\t`, un
   `\n` o un `\r` embebido cambia cómo parsea el navegador y nunca aparece en
   una URL legítima.
2. Exigir el prefijo `https://` **en minúscula exacta**. No aceptes `HTTPS://`
   ni `//`: un `redirect_uri` legítimo de este ecosistema siempre lo escribe
   así, y aceptar variantes sólo agranda la superficie.
3. Cortar en el **primer** byte que aparezca en `hostTerminators`.
4. Si lo que queda contiene `userInfoSep`, quedarse con lo que está **después
   del último** `@`.
5. Rechazar si el resultado está vacío.
6. Pasar a minúsculas (los hosts son case-insensitive).
7. Aceptar sólo si `host == "velty.cl"` o `fmt.HasSuffix(host, ".velty.cl")`.

**No uses `net/url`**: `config/` compila para el Worker con TinyGo.

Conservá el comentario que ya está arriba de la función advirtiendo que nunca
se relaje el criterio, y agregale que la lista `hostTerminators` es exhaustiva
a propósito.

## Etapa 2 · Cabeceras de seguridad (I-2)

Archivo nuevo: `routes/headers.go`.

```go
package routes

// Cabeceras de seguridad emitidas en TODAS las respuestas de iam. Los valores
// son constantes y no configurables: un servicio de identidad no tiene un
// modo "menos seguro" que valga la pena poder activar.
const (
	HeaderCSP            = "Content-Security-Policy"
	HeaderFrameOptions   = "X-Frame-Options"
	HeaderContentType    = "X-Content-Type-Options"
	HeaderReferrerPolicy = "Referrer-Policy"
	HeaderHSTS           = "Strict-Transport-Security"
	HeaderPermissions    = "Permissions-Policy"
)

const (
	// cspValue: el panel es Go compilado a WASM servido desde el mismo
	// origen. 'wasm-unsafe-eval' es lo que exige instanciar un módulo WASM;
	// NO es 'unsafe-eval' y no habilita eval() de JavaScript.
	// frame-ancestors 'none' es la versión moderna de X-Frame-Options y la
	// que realmente aplican los navegadores actuales.
	cspValue = "default-src 'self'; " +
		"script-src 'self' 'wasm-unsafe-eval'; " +
		"style-src 'self'; " +
		"img-src 'self' data: https://lh3.googleusercontent.com; " +
		"connect-src 'self'; " +
		"font-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'"

	frameOptionsValue   = "DENY"
	contentTypeValue    = "nosniff"
	referrerPolicyValue = "strict-origin-when-cross-origin"
	// hstsValue: dos años, con subdominios. iam sólo se sirve por HTTPS
	// detrás de Cloudflare; no hay escenario en que una respuesta suya deba
	// aceptarse por HTTP.
	hstsValue        = "max-age=63072000; includeSubDomains"
	permissionsValue = "camera=(), microphone=(), geolocation=(), payment=()"
)

// SecurityHeaders es el middleware que las emite. Se instala con r.Use() en
// Register, antes de cualquier ruta.
func SecurityHeaders() router.Middleware
```

`img-src` incluye `https://lh3.googleusercontent.com` porque los avatares de
Google vienen de ahí (`OAuthUserInfo.Avatar`). Verificá contra el HTML servido
si hace falta algún otro origen y **documentá cada uno** con un comentario —
una CSP con orígenes sin justificar se degrada sola con el tiempo.

Instalación, en `routes/Register`, **primera línea del cuerpo**:

```go
r.Use(SecurityHeaders())
```

**Anti-footgun:** el middleware corre por detrás de la compuerta de acceso y
sólo para rutas con handler; los assets estáticos del panel los sirve
Cloudflare, no el Worker. Eso significa que **el HTML del shell no lleva estas
cabeceras**. Documentalo en `docs/DEPLOY.md` (Etapa 8) como un paso de
configuración de Cloudflare, y **no intentes resolverlo desde Go** — no hay
símbolo para eso y recrearlo acá violaría la Restricción #2.

## Etapa 3 · CSRF desde subdominios hermanos (I-3)

Archivo nuevo: `modules/admin/origin.go`.

Las rutas de `/admin/api/*` que mutan estado exigen que la petición venga del
propio `iam`. Dos señales, en orden:

```go
const (
	HeaderOrigin      = "Origin"
	HeaderSecFetchSite = "Sec-Fetch-Site"

	// secFetchSameOrigin es el único valor aceptable: "same-site" NO alcanza,
	// y ésa es exactamente la razón por la que este archivo existe — la
	// cookie SSO es Domain=.velty.cl, así que misitio.velty.cl es same-site
	// respecto de iam.velty.cl y podría disparar estas rutas con la sesión
	// del administrador adjunta.
	secFetchSameOrigin = "same-origin"
)

// RequireSameOrigin envuelve un handler de mutación: 403 si la petición no
// vino del propio origen de iam.
//
// Acepta si Sec-Fetch-Site es "same-origin" (lo mandan todos los navegadores
// vigentes y no es falsificable desde JavaScript), o si Origin coincide
// exactamente con expectedOrigin. Una petición SIN ninguna de las dos
// cabeceras se RECHAZA: el panel siempre las manda, y lo único que llega sin
// ellas es un cliente que no es el panel.
func RequireSameOrigin(expectedOrigin string, h func(ctx router.Context, adminEmail string)) func(ctx router.Context, adminEmail string)
```

`expectedOrigin` sale de una variable de entorno nueva, en `config/api.go`:

```go
// EnvPanelOrigin es el origen exacto desde el que se sirve el panel
// ("https://iam.velty.cl"). Sin ella, iam NO arranca: adivinarla y errarle
// deja el panel inutilizable, y aceptarla ausente deja la puerta abierta.
const EnvPanelOrigin = "IAM_PANEL_ORIGIN"
```

`config.NewProductionBackend` falla rápido si falta, igual que ya hace con
`GOOGLE_CLIENT_ID` y compañía. `config.NewLocalBackend` usa
`http://localhost:8080` como valor por defecto, con comentario diciendo que es
sólo para desarrollo.

Aplicalo en `routes/routes.go` a **las siete rutas POST** de `/admin/api/*`
(`projects`, `projects/rotate`, `projects/active`, `roles`, `roles/ttl`,
`roles/delete`, `users/assign`, `users/revoke`) — no a los GET, que no mutan.

**Cuidado con el orden de anidado**: `RequirePanelAdmin` primero (afuera),
`RequireSameOrigin` adentro. Así el 401/403 de identidad se decide antes, y el
rechazo por origen queda registrado con el email del administrador ya
resuelto, que es lo que la Etapa 5 necesita auditar.

```go
r.Post(config.PathAdminProjects,
	admin.RequirePanelAdmin(authMod, adminEmails,
		admin.RequireSameOrigin(panelOrigin,
			admin.CreateProjectHandler(db, ids)))).Public()
```

Esa expresión se repite ocho veces. **Extraé un helper local** en
`routes/routes.go` para no repetirla (DRY):

```go
// adminPost registra una ruta de mutación del panel con las dos guardas que
// TODA mutación necesita, en el orden correcto. Es el único camino para
// montar un POST de /admin/api: agregar uno sin pasar por acá es agregarlo
// sin protección CSRF.
func adminPost(r router.Router, path string, authMod *authority.Module, adminEmails []string, origin string, h func(router.Context, string))
```

## Etapa 4 · Revocación, roles y `code` (I-4)

Archivo: `modules/admin/handler.go`.

**Contexto que cambia el resto de esta etapa**: la versión publicada de
`tinywasm/rbac` que trae `RevokeRoleByCode`/`AssignRoleByCode`/etc. TAMBIÉN
cambió lo que devuelve `GetRoleByCode` cuando el `code` no existe —
`rbac.ErrRoleNotFound` en vez de `orm.ErrNotFound` (una mejora deliberada:
`orm.ErrNotFound` es un detalle de implementación del ORM que no debería
filtrarse por la API pública de `rbac`). **Todo call site de este repo que
compara `err == orm.ErrNotFound` después de una llamada a `GetRoleByCode`
—directa o indirecta— tiene que pasar a comparar `err == rbac.ErrRoleNotFound`,
lo haya tocado o no la lista de abajo.** Es fácil de pasar por alto porque el
código sigue compilando (los dos son valores `error`) y sólo falla en
runtime: un 500 donde antes había un 404.

1. `RevokeUserHandler`: **borrá** el `db.Delete(&rbac.UserRole{}, …)` y el
   import de `github.com/tinywasm/storage`. Usá
   `rbacSvc.RevokeRoleByCode(req.ProjectId, u.Id, model.RoleCode(req.Code))`.
   Mapeá `rbac.ErrRoleNotFound` → 404.
2. `AssignUserHandler`: usá `rbacSvc.AssignRoleByCode(...)`, que ya devuelve
   `ErrRoleNotFound` — se va el `GetRoleByCode` + chequeo manual.
3. `CreateRoleHandler`: mapeá `rbac.ErrDuplicateRoleCode` → **409**. Constante
   nueva en `config/api.go` si hace falta nombrar el estado.
4. `DeleteRoleHandler`: usá `rbacSvc.DeleteRoleByCode(...)` y mapeá
   `rbac.ErrRoleNotFound` → 404.
5. `SetRoleTTLHandler`: **NO tiene equivalente `*ByCode`** — `SetRoleSessionTTL`
   sigue tomando `(projectID, id)`, así que este handler conserva su llamada a
   `rbacSvc.GetRoleByCode(...)` para resolver el `id`. Lo único que cambia es
   el chequeo: `if err == orm.ErrNotFound` pasa a
   `if err == rbac.ErrRoleNotFound`. Es el gap más fácil de dejar pasar de
   toda esta etapa porque el handler no se toca en ningún otro sentido — no
   te lo saltees.
6. `ListRolesHandler` / `ListRoles` en `modules/admin/backend.go`: reemplazá
   el conteo manual de `UserRole` por `rbacSvc.RoleUserCount(projectID, code)`.
   No llama a `GetRoleByCode`, así que no hay chequeo de error que migrar acá.
7. `ListRoleUsersHandler` / `ListRoleUsers` en `modules/admin/backend.go`:
   en `ListRoleUsers`, reemplazá la resolución manual de `role.Id` +
   `db.Query(&rbac.UserRole{}, …)` por `rbacSvc.UsersInRole(projectID, code)`
   directo — devuelve `[]string` de ids de usuario, y `rbac.ErrRoleNotFound`
   si el `code` no existe (ya no hace falta el `GetRoleByCode` previo).
   Quedate sólo con la resolución de perfiles vía `authMod.GetUser`, que es
   lo único que `rbac` no puede hacer. Y en `ListRoleUsersHandler`, el
   `if err == orm.ErrNotFound` que envuelve esa llamada pasa a
   `if err == rbac.ErrRoleNotFound`.

Archivo: `routes/routes.go`, handler `AssignRole`. Hoy crea el rol si no existe
usando el `code` como `id`, lo que produce el `code` duplicado del hallazgo
R-1. Cambialo a `rbacSvc.AssignRoleByCode(...)` y devolvé **404** si el rol no
existe: un proyecto consumidor no define roles, los usa. Definirlos es del
panel. Actualizá el comentario de `AssignRoleRequest` para decirlo, y
actualizá `docs/ARCHITECTURE.md` (Etapa 8) porque **esto cambia el contrato
público de `/api/roles/assign`**.

`client/consumer.go` (`Consumer.AssignRole`) tiene que reflejar el cambio:
documentá que un `roleCode` inexistente ahora devuelve error, y agregá el
error tipado correspondiente junto a `ErrMsgAssignRoleUserRequired`.

**Verificación de cierre de esta etapa**: `grep -rn "orm.ErrNotFound"
modules/admin/ routes/routes.go` sólo puede dar coincidencias en chequeos que
NO son sobre un rol (ej. `config.ErrProjectNotFound` es un error distinto,
propio de `iam`, y no se toca) — ninguna debe estar comparando el resultado
de `GetRoleByCode`, `AssignRoleByCode`, `RevokeRoleByCode`, `DeleteRoleByCode`
o `UsersInRole`.

## Etapa 5 · Secretos y auditoría (I-5, I-6)

### 5.1 · `JWT_SECRET` vacío deja de ser válido

`config/projects.go`:

```go
// ErrMissingJWTSecret: sin JWT_SECRET no hay clave con la que derivar el
// hash de un client_secret. HMAC sobre una clave vacía es matemática válida:
// produce un hash que verifica, con una clave que cualquiera puede derivar.
// Fallar es la única respuesta correcta — misma decisión que jwt.ErrEmptySecret.
var ErrMissingJWTSecret = fmt.Err("iam", "JWT_SECRET", "required")
```

`projectSecretKey()` pasa a `func projectSecretKey() ([]byte, error)` y
devuelve `ErrMissingJWTSecret` si `env.Get(EnvJWTSecret) == ""`.
`hashProjectSecret` y `VerifyProjectSecret` propagan el error. `CreateProject`
y `RegenerateProjectSecret` también.

**Importante:** `VerifyProjectSecret` ya devuelve `(bool, error)`. Un error de
clave faltante tiene que salir por el canal de `error` (→ 500), **nunca** como
`false, nil` (→ 403): un 403 le dice al llamador "tu secreto está mal" cuando
el problema es del servidor.

Reemplazá también el cuerpo de `GenerateClientSecret` por
`rand.SecretN(clientSecretBytes)` con
`const clientSecretBytes = 30` — el helper nuevo de `tinywasm/crypto`, en vez
del `make([]byte,30)` + `Read` + `URLEncode` a mano.

### 5.2 · Auditar las denegaciones

`config/audit.go`, acciones nuevas:

```go
AuditPanelDenied  = "panel.access_denied"   // sesión válida, email fuera de IAM_ADMIN_EMAILS
AuditOriginDenied = "panel.origin_denied"   // mutación desde un origen ajeno
AuditTokenDenied  = "token.secret_invalid"  // client_secret inválido en /api/token
```

Emitilas desde:

- `RequirePanelAdmin`, en la rama `!config.IsPanelAdmin(...)`, con
  `target = u.Email`.
- `RequireSameOrigin`, en el rechazo, con `target = adminEmail` y
  `detail` = el valor de `Origin` recibido, **truncado a 200 bytes**
  (`const auditDetailMax = 200`): es entrada del atacante y no puede inflar
  una fila de la base sin techo.
- `routes.Token`, en la rama `!ok`, con `target = body.ProjectID` y actor
  `""` (no hay admin). **Nunca escribas el `client_secret` recibido** en la
  auditoría, ni entero ni truncado.

`RequirePanelAdmin` y `RequireSameOrigin` necesitan `db` e `ids` para poder
auditar — pasáselos como parámetros; no crees un estado global.

`RecordAudit` hoy devuelve `error` y todos los llamadores lo ignoran con un
`fmt.Println`. Dejalo así **en las mutaciones** (una auditoría rota no debe
tumbar una operación exitosa) y hacé lo mismo en las denegaciones: el 403 se
devuelve igual.

## Etapa 6 · `router.QueryParam` (I-7)

`modules/admin/handler.go`: **borrá** `getQueryParam` completa y usá
`router.QueryParam(ctx.Path(), "project_id")` / `"code"`.

Criterio verificable: `grep -rn "func getQueryParam" .` → vacío.

## Etapa 7 · Rutas del panel bajo `/api/` (I-8)

Antes de tocar nada, **verificá el comportamiento real**: desplegá a un Worker
de prueba y comprobá si `GET /admin/api/me` llega al Worker o si Cloudflare
devuelve `index.html`. Registrá el resultado en el cuerpo del PR.

Independientemente del resultado, alineá las rutas con la convención del
ecosistema, que es gratis y elimina la dependencia:

`config/api.go`, mover el prefijo de `/admin/api/` a `/api/admin/`:

```go
PathAdminMe            = "/api/admin/me"
PathAdminProjects      = "/api/admin/projects"
PathAdminProjectRotate = "/api/admin/projects/rotate"
PathAdminProjectActive = "/api/admin/projects/active"
PathAdminRoles         = "/api/admin/roles"
PathAdminRoleTTL       = "/api/admin/roles/ttl"
PathAdminRoleDelete    = "/api/admin/roles/delete"
PathAdminRoleUsers     = "/api/admin/roles/users"
PathAdminUserAssign    = "/api/admin/users/assign"
PathAdminUserRevoke    = "/api/admin/users/revoke"
PathAdminAudit         = "/api/admin/audit"
```

Las constantes ya están centralizadas y tanto `routes/` como `modules/panel/`
las usan, así que **no debería haber ni un literal que cambiar**. Si aparece
uno, ése es el bug: reemplazalo por la constante.

`tinywasm/goflare` declara `/api/*` como worker-first, así que con esto el
panel deja de depender de la precedencia de assets. **No modifiques
`goflare`** para agregar `/admin/api/*` a su lista: la convención existe y
alinearse con ella es más barato que cambiarla.

## Etapa 8 · Tests

Todos bajo `tests/`, consumiendo los paquetes reales, con las helpers que ya
existen en `tests/setup_test.go` (`setup`, `setupBackend`, `setupPanel`,
`setupLocal`, `migrateTestDB`). Base en memoria: `orm.New(mem.New())`.
`tests/` compila con Go estándar y **no** está sujeto a las restricciones de
TinyGo — no le "arregles" los imports.

### Helpers y mocks a agregar en `tests/setup_test.go`

| Helper | Para qué |
|---|---|
| `setupPanelWithOrigin(t, adminEmail, origin string)` | Como `setupPanel`, fijando `IAM_PANEL_ORIGIN` con `t.Setenv` — lo necesitan todos los tests de la Etapa 3. |
| `adminCtx(t, b, email string) *mock.Context` | Devuelve un `mock.Context` con la sesión ya emitida para `email` y con `Sec-Fetch-Site: same-origin` puesto. Es el "camino feliz" que casi todos los tests nuevos necesitan; sin él cada test repite seis líneas. |
| `crossSiteCtx(t, b, email string) *mock.Context` | Igual pero con `Sec-Fetch-Site: cross-site` y `Origin: https://misitio.velty.cl` — el atacante hermano. |
| `lastAudit(t, db) config.AuditEntry` | Última fila de auditoría, para afirmar sobre ella sin repetir la consulta. |

### Tests obligatorios

| Test | Archivo | Fija |
|---|---|---|
| `TestIsVeltyDomain_RejectsFragmentBypass` | `oauth_redirect_test.go` | I-1: `https://evil.com#.velty.cl` → rechazado |
| `TestIsVeltyDomain_RejectsBackslashBypass` | ídem | I-1: `https://evil.com\.velty.cl` |
| `TestIsVeltyDomain_RejectsQueryBypass` | ídem | I-1: `https://evil.com?x.velty.cl` |
| `TestIsVeltyDomain_RejectsUserInfoBypass` | ídem | I-1: `https://x.velty.cl@evil.com` |
| `TestIsVeltyDomain_RejectsControlChars` | ídem | I-1: `https://evil.com\n.velty.cl` |
| `TestIsVeltyDomain_RejectsSuffixLookalike` | ídem | `https://notvelty.cl` y `https://velty.cl.evil.com` |
| `TestIsVeltyDomain_AcceptsRealConsumers` | ídem | `https://velty.cl`, `https://misitio.velty.cl/panel`, `https://a.b.velty.cl` |
| `TestSecurityHeaders_OnEveryRoute` | `headers_test.go` (nuevo) | I-2: recorre `r.Routes()` e invoca **cada** ruta con handler, afirmando las seis cabeceras. Un test que las chequea en una sola ruta no fija nada. |
| `TestCSPForbidsUnsafeInline` | `headers_test.go` | I-2: el valor de CSP no contiene `unsafe-inline` ni `unsafe-eval` (sí puede contener `wasm-unsafe-eval`) |
| `TestAdminMutationRejectsCrossSite` | `csrf_test.go` (nuevo) | I-3: los ocho POST con `crossSiteCtx` → 403, y el estado **no cambió** |
| `TestAdminMutationRejectsMissingOriginSignals` | `csrf_test.go` | I-3: sin `Origin` ni `Sec-Fetch-Site` → 403 |
| `TestAdminMutationAcceptsSameOrigin` | `csrf_test.go` | I-3: el camino feliz sigue funcionando |
| `TestAdminGetsDoNotRequireOrigin` | `csrf_test.go` | I-3: los GET no se rompieron |
| `TestRevokeInvalidatesRBACCache` | `admin_users_test.go` | **I-4, el central**: asignar → `HasPermission` true → revocar por el handler → `HasPermission` false, **misma instancia de `Service`** |
| `TestCreateRoleDuplicateCode409` | `admin_roles_test.go` | I-4 |
| `TestAssignRoleUnknownCode404` | `assign_role_test.go` | I-4: `/api/roles/assign` ya no crea roles |
| `TestVerifyProjectSecret_MissingJWTSecretIs500Not403` | `project_secret_test.go` | I-5: el error sale por el canal de `error` |
| `TestCreateProject_FailsWithoutJWTSecret` | `project_secret_test.go` | I-5 |
| `TestAudit_RecordsPanelDenial` | `audit_test.go` | I-6: no-admin con sesión → fila `panel.access_denied` con su email |
| `TestAudit_RecordsOriginDenial` | `audit_test.go` | I-6 |
| `TestAudit_RecordsInvalidClientSecret` | `audit_test.go` | I-6 |
| `TestAudit_NeverStoresClientSecret` | `audit_test.go` | I-6: tras un `/api/token` fallido, **ninguna** fila de auditoría contiene el secreto enviado |
| `TestAudit_TruncatesHostileDetail` | `audit_test.go` | I-6: un `Origin` de 10 000 bytes se guarda truncado a `auditDetailMax` |
| `TestAdminPathsAreUnderAPIPrefix` | `layering_test.go` | I-8: toda constante `PathAdmin*` empieza con `/api/` |
| `TestNoLocalQueryParser` | `layering_test.go` | I-7: `grep` en el fuente de `modules/admin/` sin `func getQueryParam` |

### Test consumer-shaped obligatorio

En `tests/hardening_test.go` (nuevo):

```
TestHostileConsumerCannotInjectIntoPanel
```

Reproduce la cadena de ataque completa que motivó toda esta auditoría, con el
stack real:

1. Crear un proyecto y obtener su `client_secret`.
2. Llamar `POST /api/users/resolve` con
   `name = "<img src=x onerror=alert(1)>"` — un consumidor legítimo, con
   secreto válido, mandando datos hostiles.
3. Asignarle un rol al usuario desde el panel.
4. Pedir `GET /api/admin/roles/users` como administrador.
5. Afirmar que el `name` vuelve **tal cual** en el JSON (`iam` no sanea
   datos, y hace bien: sanear en el borde equivocado corrompe el dato).
6. Renderizar ese `RoleUser` con el mismo código del panel y afirmar que la
   salida HTML **no contiene** `<img` — o sea, que `tinywasm/dom` escapa.

El paso 6 no puede correr desde `tests/` porque `modules/panel` es
`//go:build wasm`. **Resolvelo así**: afirmá el paso 5 en `tests/`, y para el
paso 6 escribí el assert en el propio `tinywasm/dom` (ya está pedido allá como
`TestAdminTable_RendersHostileUserDataInert`). En `tests/hardening_test.go`
dejá un comentario apuntando a ese test por nombre, para que la cadena quede
trazable. **No inventes un renderizador de HTML en `tests/`** para poder
afirmar el paso 6 localmente — eso sería recrear `tinywasm/dom` (Restricción #2).

## Restricciones de código (leer antes de escribir)

Estas aplican a **`config/`, `routes/`, `modules/`, `client/`, `edge/`** — todo
lo que compila TinyGo para el Worker. **`tests/` y `cmd/migrate/` compilan con
Go estándar y NO están sujetos**: no les "arregles" los imports.

| Regla | Detalle |
|---|---|
| **Sin mapas** | Prohibido `map[K]V`. Slices + búsqueda lineal, o structs de campos fijos. |
| **Sin stdlib pesada** | Nada de `fmt`, `errors`, `strconv`, `strings`, `log`, `os`, `net/url`. Usa `github.com/tinywasm/fmt`. Variables de entorno: `github.com/tinywasm/env`, nunca `os.Getenv`. |
| **`context` de tinywasm** | `github.com/tinywasm/context`, no el de la stdlib. |
| **`error` sí, `errors` no** | Devolver `error` está bien; construirlo con `errors.New` no. |
| **JSON sin reflexión** | `github.com/tinywasm/json`, nunca `encoding/json`. |
| **Sin `reflect`** | En ninguna forma, ni transitiva. |
| **Nunca clases CSS sueltas** | Prohibido `Attr("class", "...")` y `.Class("...")` con clases inventadas. Todo estilo pasa por `tinywasm/widget/style` en un `css.go` con `//go:build !wasm`. **`modules/panel/` viola esto hoy** (`iam-table`, `iam-panel-view`, `iam-secret-box`, `iam-status-banner`, sin ningún `RenderCSS`). Este plan **no** lo arregla — está fuera de su alcance — pero **no agregues clases nuevas**. |
| **Sin `internal/`** | No crees carpetas `internal/`. |
| **Sin subcarpetas en `modules/`** | `modules/admin/*.go` y `modules/panel/*.go`, planos. |
| **Una sola tabla de rutas** | `routes/routes.go`, un solo archivo, un solo `Register`. |
| **Sin literales repetidos** | Toda ruta, nombre de cabecera, clave de entorno y mensaje es una constante nombrada. |
| **No versiones documentos** | Nada de `v1`/`v2` dentro de los archivos. |
| **No enlaces `PLAN.md`** | Ningún documento permanente puede citar este archivo. |

Idioma: **código e identificadores en inglés**; **documentación y comentarios
de prosa en español**.

## Etapa 9 · Documentación

- `docs/ARCHITECTURE.md`:
  - §"Redirección post-login": qué termina un host y por qué la lista es
    exhaustiva. Incluí los cuatro bypasses como ejemplos de lo que se rechaza.
  - Sección nueva **"CSRF y la cookie compartida"**: por qué `SameSite=Strict`
    no alcanza con `Domain=.velty.cl`, y por qué la guarda es `same-origin` y
    no `same-site`.
  - Sección nueva **"Cabeceras de seguridad"**: los seis valores, la
    justificación de cada origen de la CSP, y la advertencia de que el shell
    HTML lo sirve Cloudflare y no lleva estas cabeceras.
  - Actualizar el contrato de `/api/roles/assign`: **ya no crea roles**.
  - Actualizar el prefijo de las rutas del panel a `/api/admin/`.
- `docs/DEPLOY.md`:
  - Variable de entorno nueva `IAM_PANEL_ORIGIN` en la tabla de "variables de
    ejecución", marcada como obligatoria, con "el Worker no arranca" en la
    columna de qué pasa si falta.
  - Paso nuevo: configurar las cabeceras de seguridad para los **assets
    estáticos** en Cloudflare (Transform Rules o `_headers`), con los mismos
    valores que emite `routes/headers.go`.
- `README.md`: en §Estado, reemplazar el párrafo actual por uno que diga que
  el servicio pasó una auditoría de seguridad y que la cadena aguas arriba se
  corrigió. **No enlaces este plan.**
- `AGENTS.md`: agregar `IAM_PANEL_ORIGIN` donde se listan las variables.

## Criterios de aceptación

1. `go vet ./...` y `go test ./tests/...` verdes.
2. `GOOS=js GOARCH=wasm go build ./edge/ ./web/` compila.
3. `grep -rn "func getQueryParam" .` → vacío.
4. `grep -rn "storage.Eq(rbac.UserRole_" .` → vacío.
4b. `grep -rn "GetRoleByCode" modules/admin/handler.go` → sólo dentro de
    `SetRoleTTLHandler`, y su chequeo de error es `rbac.ErrRoleNotFound`, no
    `orm.ErrNotFound` (ver nota al inicio de la Etapa 4).
5. `grep -rn "\"/admin/api" .` → vacío.
6. `grep -rn "map\[" config/ routes/ modules/ client/ edge/` → vacío.
7. `grep -rn "os.Getenv\|encoding/json\|net/url" config/ routes/ modules/ client/ edge/` → vacío.
8. Toda ruta POST de `/api/admin/` se registra a través del helper `adminPost`
   — ninguna llama a `r.Post` directamente. Verificable a ojo en
   `routes/routes.go`, que sigue siendo un solo archivo.
9. `IsPanelAdmin` sigue siendo comparación exacta y case-sensitive: **no** la
   cambies a case-insensitive en este plan; si hace falta, es otra decisión y
   otro plan.
10. Los 25 tests de la tabla existen y pasan.
11. El cuerpo del PR registra el resultado de la verificación de la Etapa 7.

## Etapas

| # | Archivos | Entrega |
|---|---|---|
| 1 | `config/auth.go` | Open redirect cerrado (I-1) |
| 2 | `routes/headers.go`, `routes/routes.go` | Cabeceras de seguridad (I-2) |
| 3 | `modules/admin/origin.go`, `config/api.go`, `config/backend*.go`, `routes/routes.go` | Guarda de origen + `adminPost` (I-3) |
| 4 | `modules/admin/handler.go`, `modules/admin/backend.go`, `routes/routes.go`, `client/consumer.go` | API de `rbac` por `code` (I-4) |
| 5 | `config/projects.go`, `config/audit.go`, `modules/admin/*` | Secreto obligatorio y auditoría de denegaciones (I-5, I-6) |
| 6 | `modules/admin/handler.go` | `router.QueryParam` (I-7) |
| 7 | `config/api.go` | Rutas bajo `/api/admin/` (I-8) |
| 8 | `tests/*` | Tests, helpers y el consumer-shaped |
| 9 | `docs/ARCHITECTURE.md`, `docs/DEPLOY.md`, `README.md`, `AGENTS.md` | Documentación |

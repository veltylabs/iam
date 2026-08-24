# Etapa 1 — Portar el motor de identidad + RBAC desde `misitio`

> Lee primero [`ARCHITECTURE.md`](ARCHITECTURE.md) y el resumen de
> decisiones en [`PLAN.md`](PLAN.md). Este archivo asume ese contexto.

## Objetivo

`veltylabs/misitio` ya tiene, hoy, un motor de identidad+RBAC funcionando y
probado: construye `authority.Module` (Google OAuth) + `rbac.Service` sobre
`tinywasm/auth`/`tinywasm/rbac`. Esta etapa **porta ese motor** a este repo,
**sin arrastrar nada que dependa de `site_manager` o `site_content`** (la
autorización fina de negocio de `misitio` — ver
[`ARCHITECTURE.md`](ARCHITECTURE.md) §3.4).

Al final de esta etapa, `iam` compila y sus tests pasan **en aislamiento**:
ni una importación de `veltylabs/site_manager` ni `veltylabs/site_content`
en todo el repo.

## La regla que gobierna esta etapa

```
Se porta el MOTOR (construcción de identidad + mecánica de roles/permisos).
NUNCA se porta la POLÍTICA (qué roles/permisos concretos existen, qué
recursos de negocio protegen). La política sigue siendo de cada app
consumidora — ver tinywasm/rbac: "Policy belongs to the consumer".
```

Esto no es una regla nueva de este plan: ya es la doctrina publicada de
`tinywasm/rbac` (`docs/ARCHITECTURE.md` de ese repo, sección "Concepts":
*"Policy belongs to the consumer: `Register` builds permissions from the
app's `RBACObject` handlers; no role or resource constants live here."*).
Esta etapa simplemente la aplica un nivel más arriba: si `rbac.Service` no
debe conocer los recursos de una app, `iam` (que envuelve `rbac.Service`)
tampoco.

## Qué se porta y qué NO — verificado contra el código real de `misitio`

Esta tabla es el resultado de leer el código actual de `misitio` (no el
`PLAN_STAGE_2_IDENTIDAD.md` original de ese repo, que describe una
arquitectura **anterior** al split `auth`/`rbac` del 2026-08-23 y ya no
coincide con el código — úsalo solo como contexto histórico, nunca como
fuente de la firma actual).

| Origen en `misitio` | Se porta a `iam` | Cómo |
|---|---|---|
| [`config/auth.go`](https://github.com/veltylabs/misitio/blob/main/config/auth.go) — `NewProductionAuth` | **Sí** | Motor puro: Google OAuth + `authority.Module` + `rbac.Service`. Sin `EnsureAdminRoleRBAC`, sin `cloudflare.Env`, sin `IsLocalMode`/`BypassLogin` (mecanismos de desarrollo específicos del panel rico de `misitio`, que `iam` no tiene). Ver §1.1. |
| [`config/auth_local.go`](https://github.com/veltylabs/misitio/blob/main/config/auth_local.go) — `NewLocalAuth` | **Sí** | Motor puro: escenarios locales sin Google. Sin la asignación hardcodeada de `RoleIDVeltyAdmin` (ese rol es política de `misitio`). Ver §1.2. |
| [`config/login.go`](https://github.com/veltylabs/misitio/blob/main/config/login.go) — `LoginScreen`/`RenderHTML` | **Sí, adaptado** | Sin el receiver `*Panel` (no existe panel en `iam`), branding genérico "Velty" en vez de "misitio", y **sin** el contrato de método reservado de `sitec` (`RenderHTML()` con ese nombre exacto — eso es para sitios estáticos; `iam` no es un sitio estático). Ver §1.3. |
| `config/admin.go` — `EnsureAdminRoleRBAC` | **Solo la mecánica genérica** (crear rol + asignar a emails, idempotente) | Los permisos `ResourceAccessReq`/`ResourceSite` que ese archivo crea son política de negocio de `site_manager` — **no se portan**. Se extrae solo el esqueleto reusable como `EnsureRole`. Ver §1.4. |
| `docs/PLAN_STAGE_2_IDENTIDAD.md` | No se copia | Es contexto histórico desactualizado (precede al split `auth`/`rbac`). `ARCHITECTURE.md` de este repo ya documenta la versión actual. |
| `docs/diagrams/autenticacion.md` | No en esta etapa | Describe un flujo HTTP (`GET /`, `/api/me`, redirects) que no existe todavía en `iam` — las rutas son la Etapa 3. Diseñar ese diagrama ahora sería inventar URLs sin haberlas decidido. Referencia para cuando llegue esa etapa: [`diagrams/autenticacion.md`](https://github.com/veltylabs/misitio/blob/main/docs/diagrams/autenticacion.md). |
| `tests/auth_test.go` (casos A1-A5) | **Solo el motor, no el handler HTTP** | A1/A2 prueban un handler `/api/me` que no existe aún en `iam` (Etapa 3). A4/A5 prueban `AccessRequest`/`SiteMember`, 100% `site_manager`. Se portan como pruebas de motor sin HTTP — ver §2. |
| `tests/local_auth_test.go` | **Sí, adaptado** | Misma lógica (dos escenarios, permisos difieren por rol), con un recurso de ejemplo neutro en vez de `api.ResourceAccessReq`. Ver §2. |
| `tests/admin_test.go` | **No** | 100% `site_manager`: acepta solicitudes, crea sitios, asigna `SiteMember`. Se queda en `misitio`. |
| `tests/tenant_test.go` | **No** | Prueba el aislamiento por `site_id`/`SiteMember` de `misitio` — es el "tenant" de **bajo nivel** (cliente final), no el "proyecto" de `iam`. Ver [`ARCHITECTURE.md`](ARCHITECTURE.md) §4 sobre por qué esos dos conceptos no deben mezclarse. |
| `routes/admin.go`, `routes/routes.go`, `config/backend.go` | **No** | Componen `Backend{Auth,RBAC,SM,SC}` — `SM`/`SC` son `site_manager`/`site_content`. `iam` no tiene un `Backend` así; su composición se limita a `Auth`+`RBAC` (ver §1.5). |

## Simplificación deliberada: sin `client.wasm`

`misitio` divide su panel en `web/client.go` (WASM, panel rico) y
`web/server.go`/`edge/main.go` (`!wasm`, servidor). `iam` en esta etapa
**no tiene panel rico** — solo el motor y, más adelante (Etapa 3), una API
HTTP + una pantalla de login server-side. No hay razón para replicar la
división wasm/`!wasm` de `misitio` aquí: no crear `web/client.go` ni
presupuesto de tamaño WASM en esta etapa.

## Regla WASM-safe: sí aplica a `config/`, no a `tests/`

`tinywasm/auth`/`tinywasm/rbac` son WASM-safe (compilan en `wasm` y
`!wasm`) aunque `iam` no tenga cliente WASM todavía — es una propiedad que
conviene conservar por si la Etapa 3 la necesita. **Consecuencia para esta
etapa:** todo archivo en `config/` usa `github.com/tinywasm/fmt` en vez de
`fmt`/`errors`/`strings` de la librería estándar — igual que
`misitio/config/auth.go`. **`tests/*.go` no tiene esa restricción** — usa
`testing`, `strings`, etc. de la librería estándar libremente, igual que
`misitio/tests/*.go`. No "corrijas" los imports de `tests/` a
`tinywasm/fmt`: ahí la librería estándar es la convención correcta.

---

## Archivos a crear

```
config/auth.go            // nuevo
config/auth_local.go      // nuevo
config/login.go           // nuevo
config/bootstrap.go       // nuevo
tests/setup_test.go       // nuevo
tests/auth_test.go        // nuevo
tests/local_auth_test.go  // nuevo
tests/bootstrap_test.go   // nuevo
```

## Archivo a eliminar

```
iam.go   // placeholder vacío generado por gonew (type Iam struct{}) — sin
         // uso; el paquete raíz del módulo queda sin archivos .go propios,
         // todo vive en config/. Válido en Go: el módulo solo se importa
         // por subpaquete hasta que la Etapa 3 defina una superficie
         // pública real.
```

---

## Pasos

### 1.1 — `config/auth.go`

```go
package config

import (
	"os"

	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/auth/oauth2"
	"github.com/tinywasm/auth/oauth2/provider/google"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
)

const (
	CookieSession         = "iam_session"
	TTLSession            = 86400 * 7
	EnvGoogleClientID     = "GOOGLE_CLIENT_ID"
	EnvGoogleClientSecret = "GOOGLE_CLIENT_SECRET"
	EnvGoogleRedirectURL  = "GOOGLE_REDIRECT_URL"
)

// NewProductionAuth arma el motor de identidad+RBAC para producción (Google
// OAuth). Falla rápido si falta cualquier variable: arrancar con OAuth roto
// en silencio es peor que no arrancar. No crea roles ni permisos — eso es
// política de cada app consumidora (ver bootstrap.go para el mecanismo
// genérico de asignación por email).
func NewProductionAuth(db *orm.DB, ids model.IDGenerator) (*authority.Module, *rbac.Service, error) {
	clientID := os.Getenv(EnvGoogleClientID)
	if clientID == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientID)
	}
	clientSecret := os.Getenv(EnvGoogleClientSecret)
	if clientSecret == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleClientSecret)
	}
	redirectURL := os.Getenv(EnvGoogleRedirectURL)
	if redirectURL == "" {
		return nil, nil, fmt.Errf("auth: missing required environment variable %s", EnvGoogleRedirectURL)
	}

	authMod, err := authority.New(db, auth.Config{
		IDs:        ids,
		CookieName: CookieSession,
		TokenTTL:   TTLSession,
		TrustProxy: true,
	})
	if err != nil {
		return nil, nil, err
	}

	rbacSvc, err := rbac.New(db)
	if err != nil {
		return nil, nil, err
	}

	g := &google.GoogleProvider{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
	}
	authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{g}))
	return authMod, rbacSvc, nil
}
```

**Diferencias deliberadas frente al original de `misitio`** (no las
deshagas creyendo que es un error de copiado):

- `os.Getenv` en vez de `github.com/tinywasm/goflare/cloudflare.Env`:
  `config/` en esta etapa no se despliega a Cloudflare (no hay
  `edge/main.go` todavía) y no debe acoplarse a esa dependencia antes de
  necesitarla. Cuando la Etapa 3/despliegue lo requiera, esa capa se añade
  en el punto de entrada del Worker, no aquí.
- Sin `IsLocalMode`, sin `BypassLogin`/`init()` leyendo `web/.bypass`: son
  mecanismos para iterar el diseño visual del panel rico de `misitio`.
  `iam` no tiene panel rico en esta etapa.
- Sin llamada a `EnsureAdminRoleRBAC` (ni a ningún bootstrap de rol) dentro
  del constructor: el motor no decide qué roles existen. Ver `bootstrap.go`
  (§1.4) para el mecanismo, y nótese que **nadie lo invoca todavía** en
  este repo — quien lo invoque y con qué roles es Etapa 2/3 o decisión de
  cada consumidor.

### 1.2 — `config/auth_local.go`

```go
//go:build !wasm

package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/auth/local"
	"github.com/tinywasm/auth/oauth2"
	googlemock "github.com/tinywasm/auth/oauth2/provider/google/mock"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/user"
)

// LocalScenarios son identidades de desarrollo determinísticas para probar
// iam en aislamiento, sin secretos de Google. Sin roles asignados: cada
// test decide qué rol/permiso probar (ver bootstrap_test.go).
var LocalScenarios = []local.Scenario{
	{ID: user.SubjectID("local-admin"), Name: "Admin Local", Email: "admin@iam.local"},
	{ID: user.SubjectID("local-viewer"), Name: "Viewer Local", Email: "viewer@iam.local"},
}

// NewLocalAuth arma el motor de identidad+RBAC para desarrollo, sin Google.
func NewLocalAuth(db *orm.DB, ids model.IDGenerator) (*authority.Module, *rbac.Service, error) {
	return NewLocalAuthWithScenarios(db, ids, LocalScenarios)
}

// NewLocalAuthWithScenarios permite a los tests inyectar escenarios propios.
func NewLocalAuthWithScenarios(db *orm.DB, ids model.IDGenerator, scenarios []local.Scenario) (*authority.Module, *rbac.Service, error) {
	authMod, err := authority.New(db, auth.Config{
		IDs:        ids,
		CookieName: CookieSession,
		TokenTTL:   TTLSession,
		TrustProxy: true,
	})
	if err != nil {
		return nil, nil, err
	}
	rbacSvc, err := rbac.New(db)
	if err != nil {
		return nil, nil, err
	}
	for _, sc := range scenarios {
		if _, err := authMod.GetOrCreateSubject(sc.ID, sc.Email, sc.Name, sc.Avatar); err != nil {
			return nil, nil, err
		}
	}
	mockUser := auth.OAuthUserInfo{ID: string(scenarios[0].ID), Email: scenarios[0].Email, Name: scenarios[0].Name, Avatar: scenarios[0].Avatar}
	mockProv := &googlemock.MockProvider{User: mockUser}
	authMod.Enable(oauth2.New(authMod, authMod, authMod, []auth.OAuthProvider{mockProv}))
	loc := local.New(scenarios, authMod, authMod, local.WithAfterLogin("/"))
	authMod.Enable(loc)
	return authMod, rbacSvc, nil
}
```

**Diferencia deliberada:** el original de `misitio` asigna
`RoleIDVeltyAdmin` a los escenarios marcados `Roles: []string{"Velty Admin"}`
dentro del propio constructor. Ese rol es política de `misitio`. Aquí
`LocalScenarios` no lleva `Roles`, y asignar un rol a un escenario para
probar RBAC es responsabilidad de cada test (ver §2, `local_auth_test.go`).

### 1.3 — `config/login.go`

```go
package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/oauth2/provider/google"
	"github.com/tinywasm/html"
	"github.com/tinywasm/layout/login"
)

// PathLogo apunta al isotipo de Velty. Placeholder vacío: no hay todavía un
// mecanismo para que iam sirva sus propios assets estáticos (llega con la
// Etapa 3, API HTTP).
const PathLogo = ""

// LoginScreen es la pantalla compartida de todos los proyectos Velty: sin
// marca de ningún proyecto en particular, porque el mismo login sirve a
// todos bajo SSO (Etapa 4).
func LoginScreen() *login.Login {
	return &login.Login{
		Title:    "Velty",
		Subtitle: "Iniciar sesión",
		LogoMark: PathLogo,
		Form:     html.A(auth.PathOAuthStart(google.ProviderName)).Attr("class", "btn btn-primary").Text("Iniciar sesión con Google"),
	}
}
```

**Diferencias deliberadas:** sin receiver `*Panel` (no existe ese tipo en
`iam`); título/subtítulo genéricos "Velty" en vez de "misitio"/"velty.cl".
**No repliques** el método `RenderHTML()` con el contrato de nombre
reservado que reconoce `sitec` (ver `misitio/docs/ARCHITECTURE.md` §3): eso
existe porque `misitio` es rastreado por `sitec` para generar un sitio
estático, e `iam` no lo es. Servir `LoginScreen().RenderHTML()` desde un
handler HTTP normal es trabajo de la Etapa 3 — **verifica la firma real de
`login.Login.RenderHTML()` en `tinywasm/layout/login` en ese momento**, no
está confirmada en este plan.

### 1.4 — `config/bootstrap.go`

Extracción de la mecánica genérica de
[`misitio/config/admin.go`](https://github.com/veltylabs/misitio/blob/main/config/admin.go)
`EnsureAdminRoleRBAC`, **sin** los permisos `ResourceAccessReq`/`ResourceSite`
(esos son política de `site_manager` y no se portan).

```go
package config

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/rbac"
)

// EnsureRole crea el rol (si no existe) y lo asigna de forma idempotente a
// cada email de la lista, creando el usuario en auth si todavía no existe.
// No crea ni asigna permisos: adjuntar permisos al rol es responsabilidad
// de quien llama — cada app declara su propia política (ver
// ARCHITECTURE.md §1: "Policy belongs to the consumer" de tinywasm/rbac).
func EnsureRole(rbacSvc *rbac.Service, authMod *authority.Module, roleID, roleCode, roleName, roleDescription string, emails []string) error {
	if rbacSvc == nil {
		return fmt.Errf("bootstrap: rbac service is nil")
	}
	if authMod == nil {
		return fmt.Errf("bootstrap: authority module is nil")
	}
	if err := rbacSvc.CreateRole(roleID, roleCode, roleName, roleDescription); err != nil && !isAlreadyExistsErr(err) {
		return fmt.Errf("bootstrap: error creando rol %s: %v", roleID, err)
	}
	for i := 0; i < len(emails); i++ {
		email := fmt.TrimSpace(emails[i])
		if email == "" {
			continue
		}
		u, err := authMod.UserByEmail(email)
		if err != nil {
			u, err = authMod.CreateUser(email, email, "")
			if err != nil {
				return fmt.Errf("bootstrap: error creando usuario para %s: %v", email, err)
			}
		}
		if err := rbacSvc.AssignRole(u.Id, roleID); err != nil && !isAlreadyExistsErr(err) {
			return fmt.Errf("bootstrap: error asignando rol a %s: %v", email, err)
		}
	}
	return nil
}

func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return fmt.Contains(msg, "already") || fmt.Contains(msg, "exists") || fmt.Contains(msg, "UNIQUE")
}
```

Nadie en este repo llama todavía a `EnsureRole` fuera de sus propios tests
(§2) — es el mecanismo genérico, listo para que la Etapa 2/3 (o el plan de
migración de `misitio`, en su propio repo) lo use con su propia política.

### 1.5 — Sin `Backend` compuesto en esta etapa

No crear un `config/backend.go` tipo `Backend{Auth, RBAC, SM, SC}` como el
de `misitio`: `iam` no conoce `SM`/`SC` (§ tabla arriba). Si algo necesita
agrupar `*authority.Module` + `*rbac.Service` juntos, los tests de esta
etapa los reciben como dos valores de retorno separados (ver `setup_test.go`
abajo) — no hace falta un struct hasta que un consumidor real lo pida.

---

## 2 — Tests

Los tests de esta etapa prueban el **motor**, no un handler HTTP: las rutas
son la Etapa 3. Si una firma exacta de una API externa no está confirmada
más abajo, no la inventes — usa solo las firmas que este documento cita
explícitamente (todas verificadas contra el código real de `misitio` el
2026-08-24).

### 2.1 — `tests/setup_test.go`

```go
package tests

import (
	"testing"

	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/storage/mem"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
)

// setup arma el motor de producción (Google mock vía variables de entorno
// de prueba) contra una base en memoria.
func setup(t *testing.T) (*orm.DB, *authority.Module, *rbac.Service, *unixid.UnixID) {
	t.Helper()
	t.Setenv(config.EnvGoogleClientID, "test-client-id")
	t.Setenv(config.EnvGoogleClientSecret, "test-client-secret")
	t.Setenv(config.EnvGoogleRedirectURL, "http://localhost:8080/oauth/callback/google")

	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		t.Fatalf("unixid: %v", err)
	}
	authMod, rbacSvc, err := config.NewProductionAuth(db, ids)
	if err != nil {
		t.Fatalf("NewProductionAuth: %v", err)
	}
	return db, authMod, rbacSvc, ids
}

// setupLocal arma el motor de desarrollo local, sin variables GOOGLE_*.
func setupLocal(t *testing.T) (*orm.DB, *authority.Module, *rbac.Service, *unixid.UnixID) {
	t.Helper()
	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		t.Fatalf("unixid: %v", err)
	}
	authMod, rbacSvc, err := config.NewLocalAuth(db, ids)
	if err != nil {
		t.Fatalf("NewLocalAuth: %v", err)
	}
	return db, authMod, rbacSvc, ids
}
```

### 2.2 — `tests/auth_test.go`

```go
package tests

import (
	"testing"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/storage/mem"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
)

// El motor de producción falla rápido si falta cualquier variable de
// Google: arrancar con OAuth roto en silencio es peor que no arrancar.
func TestProductionAuthFailsWithoutGoogleEnv(t *testing.T) {
	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		t.Fatalf("unixid: %v", err)
	}
	t.Setenv(config.EnvGoogleClientID, "")
	t.Setenv(config.EnvGoogleClientSecret, "")
	t.Setenv(config.EnvGoogleRedirectURL, "")

	if _, _, err := config.NewProductionAuth(db, ids); err == nil {
		t.Errorf("expected error when Google OAuth env vars are missing")
	}
}

// CreateUser + UserByEmail hacen round-trip.
func TestCreateUserRoundTrip(t *testing.T) {
	_, authMod, _, _ := setup(t)

	created, err := authMod.CreateUser("user@example.com", "Test User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	found, err := authMod.UserByEmail("user@example.com")
	if err != nil {
		t.Fatalf("UserByEmail: %v", err)
	}
	if found.Id != created.Id {
		t.Errorf("expected UserByEmail to return the same user, got Id %s want %s", found.Id, created.Id)
	}
}

// GetOrCreateSubject es idempotente: la segunda llamada con el mismo ID no
// crea un segundo usuario.
func TestGetOrCreateSubjectIdempotent(t *testing.T) {
	_, authMod, _, _ := setup(t)

	first, err := authMod.GetOrCreateSubject("subject-1", "s1@example.com", "Subject One", "")
	if err != nil {
		t.Fatalf("GetOrCreateSubject (1st): %v", err)
	}
	second, err := authMod.GetOrCreateSubject("subject-1", "s1@example.com", "Subject One", "")
	if err != nil {
		t.Fatalf("GetOrCreateSubject (2nd): %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("expected the same SubjectID on repeat calls, got %s and %s", first.ID, second.ID)
	}
}
```

### 2.3 — `tests/local_auth_test.go`

```go
package tests

import (
	"testing"

	"github.com/tinywasm/model"
)

const (
	exampleResource model.Resource = "example"
	roleExampleID                  = "role_example_admin"
	roleExampleCode                = "example_admin"
	permExampleRead                = "example:read"
)

func TestLocalScenariosStartWithoutGoogleEnv(t *testing.T) {
	// setupLocal no depende de GOOGLE_*; si esto compila y pasa, el
	// requisito está probado (setupLocal ya construye el motor).
	_, _, _, _ = setupLocal(t)
}

// Dos escenarios locales, permisos distintos por rol: el mismo mecanismo
// que verifica misitio/tests/local_auth_test.go, sin site_manager.
func TestLocalScenariosPermissionsDifferByRole(t *testing.T) {
	_, authMod, rbacSvc, _ := setupLocal(t)

	adminSubj, err := authMod.GetOrCreateSubject("local-admin", "admin@iam.local", "Admin Local", "")
	if err != nil {
		t.Fatalf("admin subject: %v", err)
	}
	viewerSubj, err := authMod.GetOrCreateSubject("local-viewer", "viewer@iam.local", "Viewer Local", "")
	if err != nil {
		t.Fatalf("viewer subject: %v", err)
	}

	if err := rbacSvc.CreateRole(roleExampleID, roleExampleCode, "Example Admin", ""); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := rbacSvc.CreatePermission(permExampleRead, "Read example", exampleResource, model.Read); err != nil {
		t.Fatalf("CreatePermission: %v", err)
	}
	if err := rbacSvc.AssignPermission(roleExampleID, permExampleRead); err != nil {
		t.Fatalf("AssignPermission: %v", err)
	}
	if err := rbacSvc.AssignRole(string(adminSubj.ID), roleExampleID); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	if !rbacSvc.Can(string(adminSubj.ID), exampleResource, model.Read) {
		t.Errorf("admin should have %s:Read", exampleResource)
	}
	if rbacSvc.Can(string(viewerSubj.ID), exampleResource, model.Read) {
		t.Errorf("viewer should NOT have %s:Read", exampleResource)
	}
	if adminSubj.ID == viewerSubj.ID {
		t.Errorf("admin and viewer should have distinct SubjectIDs")
	}
}
```

### 2.4 — `tests/bootstrap_test.go`

```go
package tests

import (
	"testing"

	"github.com/tinywasm/model"
	"github.com/veltylabs/iam/config"
)

func TestEnsureRoleIdempotentAndAssignsEmails(t *testing.T) {
	_, authMod, rbacSvc, _ := setup(t)

	if err := rbacSvc.CreatePermission(permExampleRead, "Read example", exampleResource, model.Read); err != nil {
		t.Fatalf("CreatePermission: %v", err)
	}

	email := "op1@velty.cl"
	if err := config.EnsureRole(rbacSvc, authMod, roleExampleID, roleExampleCode, "Example Admin", "", []string{email}); err != nil {
		t.Fatalf("EnsureRole (1st): %v", err)
	}
	if err := rbacSvc.AssignPermission(roleExampleID, permExampleRead); err != nil {
		t.Fatalf("AssignPermission: %v", err)
	}
	// Segunda llamada: idempotente, no debe fallar ni duplicar.
	if err := config.EnsureRole(rbacSvc, authMod, roleExampleID, roleExampleCode, "Example Admin", "", []string{email}); err != nil {
		t.Fatalf("EnsureRole (2nd): %v", err)
	}

	u, err := authMod.UserByEmail(email)
	if err != nil {
		t.Fatalf("UserByEmail(%s): %v", email, err)
	}
	if !rbacSvc.Can(u.Id, exampleResource, model.Read) {
		t.Errorf("expected %s to have %s:Read via %s", email, permExampleRead, roleExampleID)
	}
}

func TestEnsureRoleRejectsNilDeps(t *testing.T) {
	if err := config.EnsureRole(nil, nil, roleExampleID, roleExampleCode, "Example Admin", "", nil); err == nil {
		t.Errorf("expected error when rbac service and authority module are nil")
	}
}
```

---

## Reglas de calidad de código (aplican a todo lo que crees en esta etapa)

- **Sin strings repetidos como literales en lógica.** Nombres de cookie,
  claves de variables de entorno, IDs de rol/permiso: constantes con
  nombre, no literales sueltos — ya seguido arriba (`CookieSession`,
  `EnvGoogleClientID`, `roleExampleID`, etc.). No introduzcas un literal
  nuevo donde ya exista una constante para ese valor.
- **Sin duplicación entre `config/` y `tests/`.** Los tests importan las
  constantes exportadas de `config` (`config.EnvGoogleClientID`, etc.); no
  redeclares el mismo valor como string suelto en un test.
- **`isAlreadyExistsErr` vive una sola vez**, en `config/bootstrap.go`. No
  la dupliques en otro archivo de `config/`.

## Criterios de aceptación

- [ ] `go test ./...` — todo verde.
- [ ] `grep -rln "site_manager\|site_content\|SiteMember\|AccessRequest" config/ tests/` → **vacío**. Si algo aparece, se coló lógica de negocio de `misitio` que no pertenece aquí.
- [ ] `grep -rn "misitio_session" .` → **vacío**: el nombre de cookie es `iam_session`, no el de `misitio`.
- [ ] `grep -rn "\"fmt\"\|\"errors\"\|\"strings\"" config/*.go` → **vacío**: `config/` es WASM-safe, usa `tinywasm/fmt`.
- [ ] `ls iam.go` → el archivo **no existe** (placeholder de `gonew` eliminado).
- [ ] Ninguna importación de `github.com/veltylabs/site_manager` ni `github.com/veltylabs/site_content` en todo el repo.
- [ ] `config/bootstrap.go`'s `EnsureRole` no crea ni asigna ningún permiso — solo rol + asignación por email.

## Fuera de alcance (no lo hagas en esta etapa)

- Rutas HTTP, servidor, `edge/main.go`, cualquier despliegue a Cloudflare — Etapa 3.
- Columna o partición por `project_id` — Etapa 2, sin diseñar todavía.
- Tocar `veltylabs/misitio` para que consuma este servicio — plan aparte, otro repo.
- Tocar `tinywasm/user`, `tinywasm/auth` o `tinywasm/rbac`.

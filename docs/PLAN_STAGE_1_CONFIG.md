[PLAN.md](PLAN.md) | Siguiente → [Etapa 2](PLAN_STAGE_2_MODULE_ADMIN.md)

# Etapa 1 — `config/`: hoja, con lo compartido

`config/` compila para el Worker (regla TinyGo) y **no importa nada de
`github.com/veltylabs/iam/`**. Ya importa `tinywasm/orm`, `rbac`,
`auth/authority`, `model`, `jwt` — permitido.

## 1.1 — Absorber `api/` y borrarlo

`api/api.go` lo importa solo `routes/routes.go`. Mové su contenido a
**`config/api.go`** (`package config`), **borrá `api/`**, y cambiá
`api.PathX` / `api.BindingD1` → `config.PathX` / `config.BindingD1` en
`routes/routes.go`, `edge/main.go`, `cmd/migrate/main.go`, `tests/`.
`grep -rn "veltylabs/iam/api" .` → vacío; `ls api` → falla.

## 1.2 — Modelos: `config/models.go`

`ProjectModel`: agregá `Widget: input.Text()` a `name`, y un campo nuevo:

```go
// active: 1 = activo, 0 = desactivado. Fila sin valor (previa a esta
// migración) se trata como ACTIVA — ver 1.5. Sin widget: no se edita en form.
{Name: "active", Type: model.Int()},
```

Nuevo modelo (calcá el estilo del archivo):

```go
var AuditEntryModel = model.Definition{
	Name: "audit_entry",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "actor_email", Type: model.Text()},
		{Name: "action", Type: model.Text()},
		{Name: "target", Type: model.Text()},
		{Name: "detail", Type: model.Text()},
		{Name: "created_at", Type: model.Int()},
	},
}
```

Regenerá `config/models_orm.go`: `cd config && go generate ./...` (agregá
`//go:generate ormc` arriba de `models.go` si falta). Si `ormc` falla acá,
escribí a mano `AuditEntry`/`AuditEntryList`/`ReadAllAuditEntry`/`AuditEntry_`
y el campo `Active int64` en `Project` + sus líneas en
`Pointers()`/`EncodeFields()`/`DecodeFields()`/`Project_`, **calcando** el
`Project` existente. Header `// DO NOT EDIT` intacto.

## 1.3 — Constantes de ruta: `config/api.go`

Junto a las que vienen de `api/`:

```go
const (
	PathAdminMe            = "/admin/api/me"
	PathAdminProjects      = "/admin/api/projects"        // GET lista · POST crea
	PathAdminProjectRotate = "/admin/api/projects/rotate" // POST
	PathAdminProjectActive = "/admin/api/projects/active" // POST
	PathAdminRoles         = "/admin/api/roles"           // GET ?project_id= · POST crea
	PathAdminRoleTTL       = "/admin/api/roles/ttl"       // POST
	PathAdminRoleDelete    = "/admin/api/roles/delete"    // POST
	PathAdminRoleUsers     = "/admin/api/roles/users"     // GET ?project_id=&code=
	PathAdminUserAssign    = "/admin/api/users/assign"    // POST
	PathAdminUserRevoke    = "/admin/api/users/revoke"    // POST
	PathAdminAudit         = "/admin/api/audit"           // GET
)
```

(Verbos distintos sobre el mismo recurso → sub-paths: el router enruta por
path. Mismo patrón que `misitio`.)

## 1.4 — DTOs de cable: `config/panelapi.go` (nuevo)

Cruzan servidor↔panel: `modules/admin/` (Worker) los codifica y `modules/panel/`
(navegador) los decodifica. Por eso viven en `config/`. Cada uno con
`IsNil` + `EncodeFields` + `DecodeFields` **a mano**, al estilo de los structs
de `routes/routes.go` actuales. Para slices, mirá `MeResponse` de
`https://github.com/veltylabs/misitio/blob/main/routes/routes.go`
(`w.Array(name, n)` + `arr.Object(&x)` + `arr.Close()`).

```go
package config

type AdminMeResponse struct{ Email, Name string }

type AdminProjectView struct {
	Id, Name  string
	Active    bool
	CreatedAt int64
}
type AdminProjectsResponse struct{ Projects []AdminProjectView }
type AdminCreateProjectRequest struct{ Name string }
type AdminSecretResponse struct {
	Id           string
	ClientSecret string // EN CLARO — única vez
}
type AdminProjectIDRequest struct{ ProjectId string }
type AdminSetActiveRequest struct {
	ProjectId string
	Active    bool
}

type RoleView struct {
	Code, Name, Description string
	SessionTtl, UserCount  int64
}
type AdminRolesResponse struct{ Roles []RoleView }
type AdminCreateRoleRequest struct{ ProjectId, Code, Name, Description string }
type AdminRoleTTLRequest struct {
	ProjectId, Code string
	SessionTtl      int64
}
type AdminRoleRefRequest struct{ ProjectId, Code string }

type RoleUser struct{ Email, Name, Sub string }
type AdminRoleUsersResponse struct{ Users []RoleUser }
type AdminUserRoleRequest struct{ ProjectId, Code, Email string }
type AdminAssignResponse struct{ Sub string }

type AdminAuditEntry struct {
	ActorEmail, Action, Target, Detail string
	CreatedAt                          int64
}
type AdminAuditResponse struct{ Entries []AdminAuditEntry }
```

## 1.5 — Helpers de proyecto: `config/projects.go`

Ya tiene `hashProjectSecret`, `CreateProject`, `VerifyProjectSecret`,
`ErrProjectNotFound`, `MigrateProjects`. Añadí / cambiá:

```go
const clientSecretPrefix = "iam_sk_"

// GenerateClientSecret: 30 bytes del CSPRNG del ecosistema en base64 url-safe,
// prefijo reconocible. NUNCA math/rand ni derivado del tiempo.
func GenerateClientSecret() (string, error) // github.com/tinywasm/crypto/rand — Read([]byte) error

// RegenerateProjectSecret: nuevo hash; el secreto anterior deja de validar de
// inmediato. ErrProjectNotFound si no existe.
func RegenerateProjectSecret(db *orm.DB, projectID, newPlainSecret string) error

// SetProjectActive: activo/inactivo. Inactivo NO emite tokens. ErrProjectNotFound si no existe.
func SetProjectActive(db *orm.DB, projectID string, active bool) error

// ListProjects: todos, orden created_at asc.
func ListProjects(db *orm.DB) (ProjectList, error)
```

- `CreateProject`: `Active: 1`.
- `VerifyProjectSecret`: si `row.Active == 0` → `false, nil`.
- Renombrá `MigrateProjects` → `MigrateSchema`; sincroniza `&Project{}` **y**
  `&AuditEntry{}`; después del `Sync`, un `conn.Exec("UPDATE project SET active
  = 1 WHERE active IS NULL OR active = 0")` idempotente (comentario: "filas
  previas a la columna `active` se consideran activas; tras esto `active = 0`
  = desactivado a mano"). Actualizá `cmd/migrate/main.go`, `web/server.go`,
  `tests/setup_test.go`. **Anti-footgun:** si `ddl.Sync` no agrega columnas a
  una tabla existente acá, **PARÁ y REPORTÁ** — no escribas `ALTER` a mano.

## 1.6 — Gate: `config/adminaccess.go` (nuevo)

```go
package config

const EnvAdminEmails = "IAM_ADMIN_EMAILS" // ver ARCHITECTURE.md §8.2, DESIGN.md §2

// PanelAdminList: parsea EnvAdminEmails (por coma, sin vacíos ni espacios). nil si no está.
func PanelAdminList() []string

// IsPanelAdmin: email exacto en list (case-sensitive). "" → false.
func IsPanelAdmin(email string, list []string) bool
```

`tinywasm/env` (`env.Get`) + `tinywasm/fmt` (`Split`, `TrimSpace`). Si el
nombre real difiere en `fmt`, usá el que exista — no importes `strings`.

## 1.7 — Auditoría: `config/audit.go` (nuevo)

```go
package config

const (
	AuditProjectCreate     = "project.create"
	AuditProjectRotate     = "project.rotate_secret"
	AuditProjectActivate   = "project.activate"
	AuditProjectDeactivate = "project.deactivate"
	AuditRoleCreate        = "role.create"
	AuditRoleDelete        = "role.delete"
	AuditRoleSetTTL        = "role.set_ttl"
	AuditUserAssign        = "user.assign_role"
	AuditUserRevoke        = "user.revoke_role"
)

// RecordAudit escribe una fila. NO fatal: si falla, el llamador ya ejecutó la
// mutación (puede no ser idempotente) — loguea con fmt.Println y sigue.
func RecordAudit(db *orm.DB, ids model.IDGenerator, actorEmail, action, target, detail string) error

// ListAudit: últimas `limit` filas, más recientes primero.
func ListAudit(db *orm.DB, limit int) (AuditEntryList, error)
```

`time.Now() / 1e9` para epoch-segundos (patrón de `CreateProject`). El método
para IDs nuevos es el que ya usa `unixid` en los backends.

## 1.8 — `config.Backend` guarda `IDs`

`NewProductionBackend`/`NewLocalBackend` ya reciben `ids model.IDGenerator`.
`Backend` gana `IDs model.IDGenerator` (handlers de auditoría y tests lo usan).

## Criterios (Etapa 1)

- `gotest ./config/...` compila; `go vet ./...` limpio.
- `grep -rn "veltylabs/iam/api" .` → vacío; `ls api` → falla.
- `grep -rn "veltylabs/iam/" config/*.go` → **vacío**.
- `grep -rn "os\.\|encoding/json\|\"errors\"\|\"strings\"\|\"strconv\"\|map\[" config/` → vacío (salvo comentarios).
- Existen: `config.GenerateClientSecret`, `RegenerateProjectSecret`,
  `SetProjectActive`, `ListProjects`, `PanelAdminList`, `IsPanelAdmin`,
  `RecordAudit`, `ListAudit`, `MigrateSchema`; los DTOs de 1.4; las constantes
  de 1.3; `Project.Active`; modelo `AuditEntry` + codec.

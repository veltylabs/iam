← [Etapa 4](PLAN_STAGE_4_PANEL.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 6](PLAN_STAGE_6_DOCS.md)

# Etapa 5 — Tests

Bajo `tests/`, paquete `tests`, **Go estándar** (no rige la regla TinyGo:
`testing`, `strings`, `reflect`, `os/exec` permitidos). Calcá
`tests/token_test.go` y `tests/register_test.go`: handlers manejados directo
con `github.com/tinywasm/router/mock` (`mock.Router`, `mock.Context` con
`SetUserID`/`InBody`), backend con `setupBackend(t)`.

## 5.1 — `tests/setup_test.go`

- `migrateTestDB`: `config.MigrateProjects` → `config.MigrateSchema`.
- Helper:
  ```go
  func setupPanel(t *testing.T, adminEmail string) (*config.Backend, *mock.Router) {
  	t.Helper()
  	b := setupBackend(t)
  	r := &mock.Router{}
  	routes.Register(r, b.DB, b.Auth, b.RBAC, b.JWTSecret, []string{adminEmail}, b.IDs)
  	return b, r
  }
  ```
- "Usuario logueado con email X": `u,_ := b.Auth.CreateUser(email, name, "")`;
  en el `mock.Context`: `ctx.SetUserID(u.Id)` (el gate lee `authMod.GetUser(uid).Email`).
- Actualizá llamadas viejas a `routes.Register` / `config.MigrateProjects` /
  `routes.BindingD1` / `routes.PathToken` en el resto de `tests/`.

## 5.2 — `tests/admin_access_test.go`

- `TestPanelGate_NoSession401` · `TestPanelGate_NonAdmin403` ·
  `TestPanelGate_Admin200` (contra el handler de `PathAdminProjects`).
- `TestIsPanelAdmin` (tabla: lista vacía, email vacío, match exacto, case-sensitive).
- `TestPanelAdminList_ParsesEnv`: `t.Setenv(config.EnvAdminEmails, "a@x , b@x")`
  → `["a@x","b@x"]`; sin la variable → `nil`.

## 5.3 — `tests/admin_projects_test.go`

- `TestCreateProject_ReturnsSecretOnceAndVerifies`: `POST PathAdminProjects
  {name}` → 201 con `ClientSecret` (prefijo `iam_sk_`) e `Id`.
  `config.VerifyProjectSecret(db,id,secret)` → true; otro string → false.
  `GET PathAdminProjects` **no** incluye secreto ni hash.
- `TestRotateSecret_OldStopsVerifying` · `TestRotateSecret_UnknownProject404`.
- `TestDeactivateProject_BlocksTokenIssue`: `POST PathAdminProjectActive
  {active:false}` → `VerifyProjectSecret` con el secreto correcto → false;
  reactivar → true.
- `TestExistingRowWithoutActiveColumn_TreatedAsActive`: `db.Create(&config.Project{
  Id:"legacy", Name:"L", ClientSecretHash:<hash de "s">})` sin `Active`, correr
  `MigrateSchema`, verificar `VerifyProjectSecret(db,"legacy","s")` → true.
  Protege a los proyectos ya registrados en producción.

## 5.4 — `tests/admin_roles_test.go`

- `TestCreateRole_ThenListed` · `TestSetRoleTTL` (y que un token para un
  usuario con ese rol lleve `Exp-Iat == ttl` — patrón de
  `TestIssueAuthToken_UsesMostRestrictiveRoleTTL`) · `TestDeleteRole` (404 si
  el código no existe) · `TestRolesIsolatedByProject`.

## 5.5 — `tests/admin_users_test.go`

- `TestAssignRole_CreatesUserByEmail` (email nuevo → 200 con `sub`; segundo
  POST idéntico → 200 idempotente, un solo usuario) ·
  `TestAssignRole_UnknownRole404` · `TestRevokeRole` ·
  `TestAssignRole_SubMatchesLoginSub` (`sub` == `b.Auth.UserByEmail(email).Id`).

## 5.6 — `tests/audit_test.go`

- `TestAudit_RecordsEveryMutation`: una de cada operación mutadora →
  `config.ListAudit(db, 100)` tiene una fila por cada una, `ActorEmail` == el
  admin, `Action` == el `Audit*` esperado, `Target` correcto.
- `TestAudit_ReadOnlyEndpoint`: `GET PathAdminAudit` más recientes primero; no
  hay ruta de borrado/edición.

## 5.7 — `tests/panel_view_test.go`

Vistas de `modules/panel/` sin navegador: instanciá cada `xxxView()` y
`RenderHTML()`, afirmá que contiene los `ID*` de `panel/constants.go` y los
textos clave ("Crear proyecto", "Se muestra una sola vez", "Revocar"). Si
`modules/panel` necesita `//go:build wasm` para `dom`, usá el patrón dual del
skill `testing` (`frontWasm_test.go` / `backStlib_test.go` con runner
compartido).

## 5.8 — `tests/layering_test.go` (nuevo — impone la arquitectura)

`go list` sobre el grafo real (`os/exec` permitido; `t.Skip` si `go` no está
en `PATH`). Afirma:

| Test | Afirma |
|---|---|
| `TestConfigIsALeaf` | ningún `.go` de `config/` importa `github.com/veltylabs/iam/` |
| `TestNoApiPackage` | `os.Stat("../api")` → not-exist |
| `TestRoutesDoesNotImportPanel` | `go list -deps ./routes/` no contiene `veltylabs/iam/modules/panel` |
| `TestEdgeDoesNotImportUIKit` | `GOOS=js GOARCH=wasm go list -deps ./edge/` no contiene `veltylabs/iam/modules/panel`, ni `tinywasm/dom`, `html`, `form`, `layout`, `svg` |
| `TestModulesHaveNoSubdirectories` | `find ../modules -mindepth 2 -type d` vacío |
| `TestClientDoesNotImportRoutes` | `GOOS=js GOARCH=wasm go list -deps ./web/` no contiene `veltylabs/iam/routes` ni `.../modules/admin` |
| `TestRouteTableIsSingleFile` | `ls ../routes` == solo `routes.go` |
| `TestPanelViewsInViewGo` | `grep -rl "func .*[Vv]iew() dom.Component" ../modules/panel/` → solo `view.go` |

`tinywasm/model`, `input`, `orm`, `rbac`, `auth` **sí** aparecen legítimamente
en el grafo del Worker — no los pongas en la lista prohibida.

## Criterios (Etapa 5)

- `gotest ./...` — **todo verde** (con `-race`, `-vet`).
- Cada handler de la Etapa 2 tocado por ≥ 1 test.
- Ningún test depende de red, navegador real, ni `IAM_ADMIN_EMAILS` del
  entorno (siempre `t.Setenv` o lista explícita).
- Los 8 tests de `tests/layering_test.go` pasan.

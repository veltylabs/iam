← [Etapa 4](PLAN_STAGE_4_SERVE.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 6](PLAN_STAGE_6_DOCS.md)

# Etapa 5 — Tests

Todos bajo `tests/`, paquete `tests`, **Go estándar** (no rige la regla
TinyGo aquí — podés usar `testing`, `strings`, `reflect`). Calcá el estilo de
`tests/token_test.go` y `tests/register_test.go`: handlers manejados directo
con `github.com/tinywasm/router/mock` (`mock.Router`, `mock.Context` con
`SetUserID`, `InBody`), backend con `setupBackend(t)`.

## 5.1 — Ajustar `tests/setup_test.go`

- `migrateTestDB`: `config.MigrateProjects` → `config.MigrateSchema`.
- Añadí un helper para montar las rutas del panel con un admin conocido:
  ```go
  // setupPanel arma el backend y registra las rutas con adminEmail como el
  // único admin del panel. Devuelve también ese email para las aserciones.
  func setupPanel(t *testing.T, adminEmail string) (*config.Backend, *mock.Router) {
  	t.Helper()
  	backend := setupBackend(t)
  	r := &mock.Router{}
  	routes.Register(r, backend.DB, backend.Auth, backend.RBAC, backend.JWTSecret,
  		[]string{adminEmail}, backend.IDs) // IDs: agregalo a Backend si no está
  	return backend, r
  }
  ```
  Si `config.Backend` no tiene un campo `IDs`, agregalo en la Etapa 1
  (`NewProductionBackend`/`NewLocalBackend` ya reciben `ids model.IDGenerator`
  — guardalo en el struct) — los handlers de auditoría lo necesitan y los
  tests también.
- Para simular "usuario logueado con email X": creá el usuario con
  `backend.Auth.CreateUser(email, name, "")`, y en el `mock.Context` hacé
  `ctx.SetUserID(u.Id)`. El gate `requirePanelAdmin` llama
  `authMod.GetUser(uid).Email`, así que el email debe coincidir con el
  registrado.

## 5.2 — `tests/admin_access_test.go`

- `TestPanelGate_NoSession401`: `mock.Context` sin `SetUserID` → handler de
  `PathAdminProjects` responde `401`.
- `TestPanelGate_NonAdmin403`: usuario logueado con email que **no** está en
  la lista → `403`.
- `TestPanelGate_Admin200`: usuario logueado con el email admin → `200`.
- `TestIsPanelAdmin`: tabla — lista vacía, email vacío, match exacto, espacios
  alrededor en la variable (`" a@x , b@x "` → `["a@x","b@x"]`), case-sensitive.
- `TestPanelAdminList_ParsesEnv`: `t.Setenv(config.EnvAdminEmails, "a@x.cl, b@x.cl")`
  → `["a@x.cl","b@x.cl"]`; sin la variable → `nil`.

## 5.3 — `tests/admin_projects_test.go`

- `TestCreateProject_ReturnsSecretOnceAndVerifies`: POST `PathAdminProjects`
  `{name}` → `201`, cuerpo trae `client_secret` con prefijo `iam_sk_` y un
  `id`. Luego `config.VerifyProjectSecret(db, id, secret)` → `true`; con otro
  string → `false`. El secreto **no** se puede volver a obtener: `GET
  PathAdminProjects` no incluye ningún `client_secret` ni hash.
- `TestRotateSecret_OldStopsVerifying`: crear proyecto, guardar `secret1`;
  POST `PathAdminProjectRotate` `{project_id}` → `200` con `secret2 != secret1`;
  `VerifyProjectSecret(db, id, secret1)` → `false`; `(..., secret2)` → `true`.
- `TestRotateSecret_UnknownProject404`.
- `TestDeactivateProject_BlocksTokenIssue`: crear proyecto + secreto; POST
  `PathAdminProjectActive` `{project_id, active:false}` → `200`;
  `config.VerifyProjectSecret(db, id, secret)` → `false` aunque el secreto sea
  correcto. Reactivar → `true` de nuevo.
- `TestListProjects_ShowsActiveFlag`.
- `TestExistingRowWithoutActiveColumn_TreatedAsActive`: insertá un `Project`
  con `Active: 0` **simulando fila vieja** (o corré `MigrateSchema` sobre una
  D1 con una fila preexistente creada por `db.Create(&config.Project{...})`
  sin `Active`), y verificá que tras `MigrateSchema` esa fila valida el
  secreto (el `UPDATE ... WHERE active = 0` de 1.3 la marca activa). Este test
  protege a los proyectos de producción ya registrados.

## 5.4 — `tests/admin_roles_test.go`

- `TestCreateRole_ThenListed`: POST `PathAdminRoles` `{project_id, code, name}`
  → `201`; `GET PathAdminRoles?project_id=` incluye el rol con `SessionTTL: 0`
  y `UserCount: 0`.
- `TestSetRoleTTL`: POST `PathAdminRoleTTL` `{project_id, code, session_ttl:300}`
  → `200`; el rol listado ahora tiene `SessionTTL: 300`; y un token emitido
  para un usuario con ese rol lleva `Exp-Iat == 300` (reusá el patrón de
  `TestIssueAuthToken_UsesMostRestrictiveRoleTTL`).
- `TestDeleteRole`: crear, borrar (`PathAdminRoleDelete`), ya no aparece en la
  lista. `404` si el código no existe.
- `TestRolesIsolatedByProject`: crear `admin` en `proj-a` y en `proj-b`;
  `GET ...?project_id=proj-a` no muestra el de `proj-b`.

## 5.5 — `tests/admin_users_test.go`

- `TestAssignRole_CreatesUserByEmail`: POST `PathAdminUserAssign`
  `{project_id, code, email}` con un email nunca visto → `200` con `sub` no
  vacío; `GET PathAdminRoleUsers?project_id=&code=` lo incluye. Segundo POST
  idéntico → `200` (idempotente), sigue habiendo un solo usuario.
- `TestAssignRole_UnknownRole404`.
- `TestRevokeRole`: asignar, revocar (`PathAdminUserRevoke`), ya no aparece.
- `TestAssignRole_SubMatchesLoginSub`: el `sub` devuelto por `assign` es el
  mismo `Id` que `backend.Auth.UserByEmail(email)` — garantiza que asignar a
  alguien antes de su primer login le da el rol correcto cuando entra
  (mismo invariante que `/api/users/resolve`).

## 5.6 — `tests/audit_test.go`

- `TestAudit_RecordsEveryMutation`: ejecutá una de cada operación mutadora
  (crear proyecto, rotar, desactivar, crear rol, set TTL, borrar rol, asignar,
  revocar) y verificá que `config.ListAudit(db, 100)` tiene una fila por cada
  una, con `ActorEmail` == el email admin, `Action` == el `Audit*` esperado,
  y `Target` correcto.
- `TestAudit_ReadOnlyEndpoint`: `GET PathAdminAudit` devuelve las entradas más
  recientes primero; no hay ruta para borrar/editar auditoría.
- `TestAudit_FailureDoesNotFailMutation`: (si es fácil de simular con un
  `*orm.DB` que falle solo en `audit_log`) — la mutación responde `200`
  aunque el `RecordAudit` devuelva error. Si no es fácil de simular, omitilo
  y dejá un comentario `// nota: cubierto por revisión de código, no por test`.

## 5.7 — `tests/panel_modules_test.go`

Vistas de los módulos WASM sin navegador, al estilo de
`veltylabs/misitio` `config/panel` (`UIModuleView`): para cada módulo,
instanciá su vista y `RenderHTML()`, y afirmá que contiene los `id` de
`panelconst` (`IDProjectsList`, `IDRolesNew`, …) y los textos clave
("Crear proyecto", "Se muestra una sola vez", "Revocar"). Estos tests
compilan con Go estándar; si la vista necesita `//go:build wasm` para
`dom`, seguí el patrón dual del skill `testing`
(`frontWasm_test.go` / `backStlib_test.go` con un runner compartido).

## 5.8 — Regresión

- `tests/register_test.go`: actualizá la llamada a `routes.Register` a la
  firma nueva (`+ adminEmails, + ids`). Sus 3 casos siguen pasando.
- `tests/token_test.go`, `consumer_test.go`, etc.: cualquier llamada a
  `config.MigrateProjects` o `routes.Register` con la firma vieja se ajusta.

## Criterios de aceptación (Etapa 5)

- `gotest ./...` — **todo verde**, con `-race` y `-vet` (los aplica `gotest`).
- `gotest -cover ./...` — las rutas nuevas y los helpers de `config` tienen
  cobertura (no exijas un %, pero cada handler de 2.4 debe ser tocado por al
  menos un test).
- Ningún test depende de red, de un navegador real, ni de `IAM_ADMIN_EMAILS`
  del entorno (siempre `t.Setenv` o lista explícita).

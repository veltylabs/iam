← [Etapa 2](PLAN_STAGE_2_MODULE_ADMIN.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 4](PLAN_STAGE_4_PANEL.md)

# Etapa 3 — `routes/routes.go` (una tabla) + puntos de entrada

## 3.1 — `routes.Register`: firma y montaje

`routes/routes.go` es **un solo archivo, una sola tabla**. No metas handlers
acá — viven en `modules/admin/`. Importá `github.com/veltylabs/iam/config` y
`github.com/veltylabs/iam/modules/admin`.

Firma nueva (agrega `adminEmails` e `ids` al final):

```go
func Register(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte, adminEmails []string, ids model.IDGenerator)
```

Tras las rutas actuales (que ya no usan alias `api.` — Etapa 1.1), añadí el
bloque del panel. Todas `.Public()` a nivel de router; el gate real es
`admin.RequirePanelAdmin` dentro (mismo patrón que `PathUsersResolve` hoy — el
`Authn` del servidor/edge ya resuelve la cookie SSO y setea `ctx.UserID()`):

```go
r.Get(config.PathAdminMe, admin.RequirePanelAdmin(authMod, adminEmails, admin.Me(authMod))).Public()
r.Get(config.PathAdminProjects, admin.RequirePanelAdmin(authMod, adminEmails, admin.ListProjects(db))).Public()
r.Post(config.PathAdminProjects, admin.RequirePanelAdmin(authMod, adminEmails, admin.CreateProject(db, ids))).Public()
r.Post(config.PathAdminProjectRotate, admin.RequirePanelAdmin(authMod, adminEmails, admin.RotateSecret(db, ids))).Public()
r.Post(config.PathAdminProjectActive, admin.RequirePanelAdmin(authMod, adminEmails, admin.SetActive(db, ids))).Public()
r.Get(config.PathAdminRoles, admin.RequirePanelAdmin(authMod, adminEmails, admin.ListRoles(db))).Public()
r.Post(config.PathAdminRoles, admin.RequirePanelAdmin(authMod, adminEmails, admin.CreateRole(db, rbacSvc, ids))).Public()
r.Post(config.PathAdminRoleTTL, admin.RequirePanelAdmin(authMod, adminEmails, admin.SetRoleTTL(db, rbacSvc, ids))).Public()
r.Post(config.PathAdminRoleDelete, admin.RequirePanelAdmin(authMod, adminEmails, admin.DeleteRole(db, rbacSvc, ids))).Public()
r.Get(config.PathAdminRoleUsers, admin.RequirePanelAdmin(authMod, adminEmails, admin.ListRoleUsers(db, authMod, rbacSvc))).Public()
r.Post(config.PathAdminUserAssign, admin.RequirePanelAdmin(authMod, adminEmails, admin.AssignUser(db, authMod, rbacSvc, ids))).Public()
r.Post(config.PathAdminUserRevoke, admin.RequirePanelAdmin(authMod, adminEmails, admin.RevokeUser(db, authMod, rbacSvc, ids))).Public()
r.Get(config.PathAdminAudit, admin.RequirePanelAdmin(authMod, adminEmails, admin.ListAudit(db))).Public()
```

Extendé el comentario de cabecera de `Register` (el de "ninguna ruta lleva
CORS: el llamador es siempre el SERVIDOR de un proyecto consumidor"): añadí que
el panel también es server-side (la cookie SSO viaja sola), así que
`/admin/api/*` tampoco lleva CORS.

Actualizá las llamadas a `routes.Register` en `edge/main.go`, `web/server.go`
y `tests/`.

## 3.2 — `edge/main.go`

```go
	r := edge.NewRouter(edge.Config{ Authn: backend.Auth.Authenticate() })
	routes.Register(r, db, backend.Auth, backend.RBAC, backend.JWTSecret,
		config.PanelAdminList(), ids)
	edge.Serve(r)
```

`edge.Serve` sirve `web/public/` cuando la carpeta existe — Etapa 4.

## 3.3 — `web/server.go`

Cambios mínimos:

```go
	if err := config.MigrateSchema(conn, compiler); err != nil {   // era MigrateProjects
		fmt.Println("migrate: schema:", err); os.Exit(1)
	}
	backend, err := config.NewLocalBackend(db, ids)
	// ...

	// En local, la identidad mock (config.LocalScenarios[0]) entra al panel sin
	// configurar IAM_ADMIN_EMAILS. Sobrescribible con -server_admin_emails.
	adminEmails := config.PanelAdminList()
	if len(adminEmails) == 0 {
		if a := env.Arg("server_admin_emails"); a != "" {
			adminEmails = []string{a}
		} else {
			adminEmails = []string{config.LocalScenarios[0].Email}
		}
	}

	srv := httpd.New(httpd.Config{
		Port: port, PublicDir: publicDir,
		Authn: backend.Auth.Authenticate(), NoCache: true, RoutesEndpoint: true,
	})
	routes.Register(srv.Router(), db, backend.Auth, backend.RBAC, backend.JWTSecret,
		adminEmails, ids)
```

## Criterios (Etapa 3)

- `go build ./... && GOOS=js GOARCH=wasm go vet ./edge/...` limpios.
- `grep -rn "r\.\(Get\|Post\|Put\|Delete\)(" --include=*.go . | grep -v tests/`
  → **todas** en `routes/routes.go`.
- `ls routes/` → solo `routes.go`.
- `grep -c "admin.RequirePanelAdmin" routes/routes.go` → 13.
- `grep -rn "veltylabs/iam/modules/panel" routes/` → vacío.

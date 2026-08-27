← [Etapa 3](PLAN_STAGE_3_PANEL_UI.md) | [PLAN.md](PLAN.md) | Siguiente → [Etapa 5](PLAN_STAGE_5_TESTS.md)

# Etapa 4 — Servir el panel

Hoy `iam` es "un Worker API puro, sin `web/public`" (`docs/DEPLOY.md`). Esta
etapa lo convierte en Worker con assets, igual que `veltylabs/misitio`.

## 4.1 — `web/public/` (nuevo)

Calcá la estructura de
`https://github.com/veltylabs/misitio/tree/main/web/public`.

### `web/public/index.html`

HTML mínimo servido en `/`:

- `<div id="app"></div>` — el panel se monta acá (o en `<body>`, según use
  `platformd`).
- Un bloque `id="login"` **visible por defecto** con un enlace
  `<a href="/oauth/google?redirect_uri=<origin>">Entrar con Google</a>`.
  `web/client.go` lo oculta (`display:none` vía `dom`) cuando
  `/admin/api/me` responde `200`. Con `401` el enlace queda visible — mismo
  criterio que `misitio` (el login funciona sin WASM).
- El `<script>` que carga `wasm_exec.js` + `client.wasm` — copialo tal cual
  de `misitio/web/public/index.html`, cambiando textos.
- Sin CSS propio más allá de un reset mínimo; el tema lo trae el WASM.

### `web/public/favicon.svg`

Uno simple (una llave 🔑 en SVG con `fill="currentColor"`), o copiá el de
`misitio` y cambiá el color.

## 4.2 — `web/server.go` (reemplazo completo)

```go
//go:build !wasm

package main

import (
	"os"

	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/server/httpd"
	"github.com/tinywasm/sqlt"
	"github.com/tinywasm/storage/mem"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

const (
	DevPort      = "8080"
	DevPublicDir = "web/public"
)

func main() {
	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		fmt.Println("unixid:", err)
		os.Exit(1)
	}

	conn := db.RawConn()
	compiler := sqlt.NewCompiler()
	if err := authority.Migrate(conn, compiler); err != nil {
		fmt.Println("migrate: authority:", err)
		os.Exit(1)
	}
	if err := rbac.Migrate(conn, compiler); err != nil {
		fmt.Println("migrate: rbac:", err)
		os.Exit(1)
	}
	if err := config.MigrateSchema(conn, compiler); err != nil {
		fmt.Println("migrate: schema:", err)
		os.Exit(1)
	}

	backend, err := config.NewLocalBackend(db, ids)
	if err != nil {
		fmt.Println("backend:", err)
		os.Exit(1)
	}

	// En local, la identidad mock de desarrollo (config.LocalScenarios[0])
	// entra al panel sin configurar IAM_ADMIN_EMAILS. Se puede sobrescribir
	// con -server_admin_emails / env.Arg("server_admin_emails") si hace falta
	// probar el caso "no-admin".
	adminEmails := config.PanelAdminList()
	if len(adminEmails) == 0 {
		if arg := env.Arg("server_admin_emails"); arg != "" {
			adminEmails = []string{arg}
		} else {
			adminEmails = []string{config.LocalScenarios[0].Email}
		}
	}

	port := env.Arg("server_port")
	if port == "" {
		port = DevPort
	}
	publicDir := env.Arg("server_public_dir")
	if publicDir == "" {
		publicDir = DevPublicDir
	}

	srv := httpd.New(httpd.Config{
		Port:           port,
		PublicDir:      publicDir,
		Authn:          backend.Auth.Authenticate(),
		NoCache:        true,
		RoutesEndpoint: true,
	})
	routes.Register(srv.Router(), db, backend.Auth, backend.RBAC, backend.JWTSecret, adminEmails, ids)

	if err := srv.ListenAndServe(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

(Ajustá el orden de parámetros de `routes.Register` al que definiste en la
Etapa 2 — `adminEmails` y `ids` al final.)

## 4.3 — `edge/main.go`

Añadí, después de construir `backend`:

```go
	r := edge.NewRouter(edge.Config{
		Authn: backend.Auth.Authenticate(),
	})
	routes.Register(r, db, backend.Auth, backend.RBAC, backend.JWTSecret, config.PanelAdminList(), ids)
	edge.Serve(r)
```

`edge.Serve` con el runtime de `goflare` sirve `web/public/` automáticamente
cuando la carpeta existe (mismo mecanismo que `misitio`). **Verificá** contra
`misitio/edge/main.go`: si `misitio` necesita un paso o config extra para los
assets, replicalo. Si `edge` NO sirve estáticos solo, **pará y reportá** — no
inventes un file server dentro del Worker.

## 4.4 — `.github/workflows/deploy.yml`

El paso `tinywasm/goflare@v1` ya compila `edge.wasm`, corre `cmd/migrate` y
despliega. Añadí lo que `misitio` tiene y `iam` no, si el panel lo requiere:
compilar `web/client.go` → `web/public/client.wasm`. En `misitio` eso lo hace
`goflare` al detectar `web/client.go`; **verificá** que con `iam` pasa igual
(comparando el `deploy.yml` de ambos). Si `goflare@v1` necesita un input para
el client build, agregalo calcando `misitio`.

No agregues secretos nuevos al `env:` del job: `IAM_ADMIN_EMAILS` es una
variable de **ejecución** del Worker (se carga una vez en el panel de
Cloudflare, ver `docs/DEPLOY.md` §3), no de despliegue.

## 4.5 — `.gitignore`

Asegurate de que `web/public/client.wasm` y `.build/` estén ignorados si el
repo no versiona binarios (mirá el `.gitignore` de `misitio`). `index.html`,
`favicon.svg` y `wasm_exec.js` SÍ se versionan.

## Criterios de aceptación (Etapa 4)

- `go build ./web/...` (`!wasm`) y el build wasm de `web/client.go` compilan.
- Levantando `web/server.go` local: `GET /` sirve `index.html`; `GET /` sin
  sesión muestra el enlace de login; `GET /admin/api/me` sin sesión → 401.
- `curl localhost:8080/api/health` → `{"ok":true}` (rutas viejas intactas).
- `docs/DEPLOY.md` menciona `web/public` y el panel (lo ajustás en la Etapa 6).

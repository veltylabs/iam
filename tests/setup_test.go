package tests

import (
	"testing"

	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/router/mock"
	"github.com/tinywasm/sqlt"
	"github.com/tinywasm/storage/mem"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/routes"
)

// testPanelOrigin es el origen del panel en tests — lo que NewProductionBackend
// exige vía IAM_PANEL_ORIGIN.
const testPanelOrigin = "https://iam.velty.cl"

func migrateTestDB(t *testing.T, db *orm.DB) {
	t.Helper()
	if err := config.MigrateAll(db.RawConn(), sqlt.NewCompiler()); err != nil {
		t.Fatal(err)
	}
}

// setup arma el motor de producción (Google mock vía variables de entorno
// de prueba) contra una base en memoria.
func setup(t *testing.T) (*orm.DB, *authority.Module, *rbac.Service, model.IDGenerator) {
	t.Helper()
	t.Setenv(config.EnvGoogleClientID, "test-client-id")
	t.Setenv(config.EnvGoogleClientSecret, "test-client-secret")
	t.Setenv(config.EnvGoogleRedirectURL, "http://localhost:8080/oauth/callback/google")
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")

	db := orm.New(mem.New())
	migrateTestDB(t, db)
	ids, err := config.NewIDs()
	if err != nil {
		t.Fatalf("ids: %v", err)
	}
	authMod, rbacSvc, err := config.NewProductionAuth(db, ids)
	if err != nil {
		t.Fatalf("NewProductionAuth: %v", err)
	}
	return db, authMod, rbacSvc, ids
}

// setupBackend arma el Backend completo (incluye JWTSecret) para los tests
// de la Etapa 3/4 (routes.Token, CORS) que necesitan firmar/verificar.
func setupBackend(t *testing.T) *config.Backend {
	t.Helper()
	t.Setenv(config.EnvGoogleClientID, "test-client-id")
	t.Setenv(config.EnvGoogleClientSecret, "test-client-secret")
	t.Setenv(config.EnvGoogleRedirectURL, "http://localhost:8080/oauth/callback/google")
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")
	t.Setenv(config.EnvPanelOrigin, testPanelOrigin)

	db := orm.New(mem.New())
	migrateTestDB(t, db)
	ids, err := config.NewIDs()
	if err != nil {
		t.Fatalf("ids: %v", err)
	}
	backend, err := config.NewProductionBackend(db, ids)
	if err != nil {
		t.Fatalf("NewProductionBackend: %v", err)
	}
	return backend
}

func setupPanel(t *testing.T, adminEmail string) (*config.Backend, *mock.Router) {
	t.Helper()
	return setupPanelWithOrigin(t, adminEmail, testPanelOrigin)
}

// setupPanelWithOrigin arma el panel fijando IAM_PANEL_ORIGIN — lo necesitan
// todos los tests de la guarda de origen.
func setupPanelWithOrigin(t *testing.T, adminEmail, origin string) (*config.Backend, *mock.Router) {
	t.Helper()
	b := setupBackend(t)
	b.PanelOrigin = origin
	r := &mock.Router{}
	routes.Register(r, b.DB, b.Auth, b.RBAC, b.JWTSecret, []string{adminEmail}, b.IDs, origin)
	return b, r
}

// adminCtx devuelve un mock.Context con la sesión ya emitida para email y
// con Sec-Fetch-Site: same-origin puesto. Es el "camino feliz" que casi
// todos los tests nuevos necesitan.
func adminCtx(t *testing.T, b *config.Backend, email string) *mock.Context {
	t.Helper()
	u, err := b.Auth.UserByEmail(email)
	if err != nil {
		u, err = b.Auth.CreateUser(email, email, "")
		if err != nil {
			t.Fatalf("adminCtx CreateUser: %v", err)
		}
	}
	ctx := &mock.Context{}
	ctx.SetUserID(u.Id)
	ctx.SetHeader("Sec-Fetch-Site", "same-origin")
	return ctx
}

// crossSiteCtx es el atacante hermano: misma sesión válida, pero la petición
// viene de otro subdominio (Sec-Fetch-Site: cross-site, Origin de misitio).
func crossSiteCtx(t *testing.T, b *config.Backend, email string) *mock.Context {
	t.Helper()
	u, err := b.Auth.UserByEmail(email)
	if err != nil {
		u, err = b.Auth.CreateUser(email, email, "")
		if err != nil {
			t.Fatalf("crossSiteCtx CreateUser: %v", err)
		}
	}
	ctx := &mock.Context{}
	ctx.SetUserID(u.Id)
	ctx.SetHeader("Sec-Fetch-Site", "cross-site")
	ctx.SetHeader("Origin", "https://misitio.velty.cl")
	return ctx
}

// lastAudit devuelve la última fila de auditoría, para afirmar sobre ella
// sin repetir la consulta.
func lastAudit(t *testing.T, db *orm.DB) config.AuditEntry {
	t.Helper()
	entries, err := config.ListAudit(db, 1)
	if err != nil {
		t.Fatalf("lastAudit: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("lastAudit: no audit entries")
	}
	return *entries[0]
}

// setupLocal arma el motor de desarrollo local, sin variables GOOGLE_*.
func setupLocal(t *testing.T) (*orm.DB, *authority.Module, *rbac.Service, model.IDGenerator) {
	t.Helper()
	db := orm.New(mem.New())
	migrateTestDB(t, db)
	ids, err := config.NewIDs()
	if err != nil {
		t.Fatalf("ids: %v", err)
	}
	authMod, rbacSvc, err := config.NewLocalAuth(db, ids)
	if err != nil {
		t.Fatalf("NewLocalAuth: %v", err)
	}
	return db, authMod, rbacSvc, ids
}

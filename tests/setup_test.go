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
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")

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

// setupBackend arma el Backend completo (incluye JWTSecret) para los tests
// de la Etapa 3/4 (routes.Token, CORS) que necesitan firmar/verificar.
func setupBackend(t *testing.T) *config.Backend {
	t.Helper()
	t.Setenv(config.EnvGoogleClientID, "test-client-id")
	t.Setenv(config.EnvGoogleClientSecret, "test-client-secret")
	t.Setenv(config.EnvGoogleRedirectURL, "http://localhost:8080/oauth/callback/google")
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")

	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		t.Fatalf("unixid: %v", err)
	}
	backend, err := config.NewProductionBackend(db, ids)
	if err != nil {
		t.Fatalf("NewProductionBackend: %v", err)
	}
	return backend
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

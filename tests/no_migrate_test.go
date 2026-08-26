package tests

import (
	"testing"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/storage/mem"
	"github.com/tinywasm/unixid"
	"github.com/veltylabs/iam/config"
)

func TestProductionBackend_DoesNotMigrate(t *testing.T) {
	t.Setenv(config.EnvGoogleClientID, "test-client-id")
	t.Setenv(config.EnvGoogleClientSecret, "test-client-secret")
	t.Setenv(config.EnvGoogleRedirectURL, "http://localhost:8080/oauth/callback/google")
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")

	// DB is intentionally empty (no tables migrated).
	db := orm.New(mem.New())
	ids, err := unixid.NewUnixID()
	if err != nil {
		t.Fatalf("unixid: %v", err)
	}

	backend, err := config.NewProductionBackend(db, ids)
	if err != nil {
		t.Fatalf("NewProductionBackend failed against empty DB: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

package tests

import (
	"testing"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/storage/mem"
	"github.com/veltylabs/iam/config"
)

func TestProductionBackend_DoesNotMigrate(t *testing.T) {
	t.Setenv(config.EnvGoogleClientID, "test-client-id")
	t.Setenv(config.EnvGoogleClientSecret, "test-client-secret")
	t.Setenv(config.EnvGoogleRedirectURL, "http://localhost:8080/oauth/callback/google")
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")
	t.Setenv(config.EnvPanelOrigin, "https://iam.velty.cl")

	// DB is intentionally empty (no tables migrated).
	db := orm.New(mem.New())
	ids, err := config.NewIDs()
	if err != nil {
		t.Fatalf("ids: %v", err)
	}

	backend, err := config.NewProductionBackend(db, ids)
	if err != nil {
		t.Fatalf("NewProductionBackend failed against empty DB: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
}

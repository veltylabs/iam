package tests

import (
	"testing"

	"github.com/tinywasm/env/osenv"
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

	if _, _, err := config.NewProductionAuth(db, ids, osenv.Reader()); err == nil {
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

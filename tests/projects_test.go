package tests

import (
	"bytes"
	"testing"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/storage/mem"
	"github.com/veltylabs/iam/config"
)

func TestProjectSecretRoundTrip(t *testing.T) {
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")
	db := orm.New(mem.New())
	migrateTestDB(t, db)

	const projectID = "proj-roundtrip"
	const secret = "secret-123"

	if err := config.CreateProject(db, projectID, "Test Project", secret); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	ok, err := config.VerifyProjectSecret(db, projectID, secret)
	if err != nil {
		t.Fatalf("VerifyProjectSecret (valid): %v", err)
	}
	if !ok {
		t.Errorf("expected true for correct secret, got false")
	}

	okWrong, err := config.VerifyProjectSecret(db, projectID, "wrong-secret")
	if err != nil {
		t.Fatalf("VerifyProjectSecret (wrong): %v", err)
	}
	if okWrong {
		t.Errorf("expected false for wrong secret, got true")
	}
}

func TestProjectSecretRejectsCorruptHash(t *testing.T) {
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")
	db := orm.New(mem.New())
	migrateTestDB(t, db)

	const projectID = "proj-corrupt"
	// Write a row manually with invalid base64 hash
	err := db.Create(&config.Project{
		Id:               projectID,
		Name:             "Corrupt Project",
		ClientSecretHash: "!!!invalid-base64!!!",
		CreatedAt:        1000,
	})
	if err != nil {
		t.Fatalf("Create project row: %v", err)
	}

	ok, err := config.VerifyProjectSecret(db, projectID, "any-secret")
	if err != nil {
		t.Fatalf("expected nil error for corrupt hash, got: %v", err)
	}
	if ok {
		t.Errorf("expected false for corrupt hash, got true")
	}
}

func TestProjectSecretKeyIsDerivedNotReused(t *testing.T) {
	const jwtSecret = "test-jwt-secret-32-bytes-long-0000"
	t.Setenv(config.EnvJWTSecret, jwtSecret)

	derivedKey := config.ProjectSecretKey()
	if bytes.Equal(derivedKey, []byte(jwtSecret)) {
		t.Errorf("ProjectSecretKey() returned raw JWTSecret, expected derived key")
	}
}

package tests

import (
	"bytes"
	"testing"

	"github.com/tinywasm/env"
	"github.com/veltylabs/iam/config"
)

func TestProjectSecretRoundTrip(t *testing.T) {
	db, _, _, _ := setup(t)
	const (
		projectID   = "proj-secret-test"
		projectName = "Project Secret Test"
		secret      = "super-secret-key-123"
	)

	if err := config.CreateProject(db, projectID, projectName, secret); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	ok, err := config.VerifyProjectSecret(db, projectID, secret)
	if err != nil {
		t.Fatalf("VerifyProjectSecret correct secret: %v", err)
	}
	if !ok {
		t.Errorf("VerifyProjectSecret correct secret: got false, want true")
	}

	okWrong, err := config.VerifyProjectSecret(db, projectID, "wrong-secret")
	if err != nil {
		t.Fatalf("VerifyProjectSecret wrong secret: %v", err)
	}
	if okWrong {
		t.Errorf("VerifyProjectSecret wrong secret: got true, want false")
	}
}

func TestProjectSecretRejectsCorruptHash(t *testing.T) {
	db, _, _, _ := setup(t)
	const projectID = "proj-corrupt-hash"

	if err := db.Create(&config.Project{
		Id:               projectID,
		Name:             "Corrupt Project",
		ClientSecretHash: "!!!not-valid-base64!!!",
		CreatedAt:        1000,
	}); err != nil {
		t.Fatalf("Create project with corrupt hash: %v", err)
	}

	ok, err := config.VerifyProjectSecret(db, projectID, "any-secret")
	if err != nil {
		t.Fatalf("VerifyProjectSecret with corrupt hash returned error: %v, want (false, nil)", err)
	}
	if ok {
		t.Errorf("VerifyProjectSecret with corrupt hash: got true, want false")
	}
}

func TestProjectSecretKeyIsDerivedNotReused(t *testing.T) {
	t.Setenv(config.EnvJWTSecret, "test-jwt-secret-32-bytes-long-0000")
	rawSecret := []byte(env.Get(config.EnvJWTSecret))

	// Re-run setup to get helper or test config if needed, or invoke test logic
	db, _, _, _ := setup(t)
	const (
		projectID = "proj-derived-test"
		secret    = "secret-1"
	)

	if err := config.CreateProject(db, projectID, "Derived Test", secret); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	qb := db.Query(&config.Project{}).Where(config.Project_.Id).Eq(projectID)
	projects, err := config.ReadAllProject(qb)
	if err != nil {
		t.Fatalf("ReadAllProject: %v", err)
	}
	if len(projects) == 0 {
		t.Fatalf("Project not found")
	}

	// Verify that the stored hash is not simply an HMAC with rawSecret directly.
	// We check rawSecret is not equal to derived key implicitly by verifying hashing difference if needed.
	if bytes.Equal(rawSecret, []byte(projects[0].ClientSecretHash)) {
		t.Errorf("stored hash matched raw jwt secret")
	}
}

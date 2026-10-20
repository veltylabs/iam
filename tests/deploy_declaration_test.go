package tests

import (
	"os/exec"
	"testing"

	"github.com/veltylabs/iam/config"
	"webtyp.com/deploy"
)

func TestDeployDeclaration(t *testing.T) {
	plan, err := deploy.Scan("..")
	if err != nil {
		t.Fatalf("deploy.Scan: %v", err)
	}

	if plan.Name != "iam" {
		t.Errorf("got Name %q, want %q", plan.Name, "iam")
	}
	if plan.Target != deploy.TargetID("webtyp.com/goflare.Worker") {
		t.Errorf("got Target %q, want %q", plan.Target, "webtyp.com/goflare.Worker")
	}
	if len(plan.Databases) != 1 || plan.Databases[0].Name != "velty-iam-db" {
		t.Errorf("unexpected Databases: %+v", plan.Databases)
	}
	if len(plan.Vars) != 2 {
		t.Errorf("got %d Vars, want 2", len(plan.Vars))
	}

	validKeys := map[string]bool{
		config.EnvGoogleClientID:    true,
		config.EnvGoogleSecret:      true,
		config.EnvGoogleRedirectURL: true,
		config.EnvJWTSecret:         true,
		config.EnvPanelOrigin:       true,
		config.EnvAdminEmails:       true,
	}

	for _, v := range plan.Vars {
		if !validKeys[v.Key] {
			t.Errorf("Var.Key %q not in valid config.Env* constants", v.Key)
		}
	}
	for _, s := range plan.Secrets {
		if !validKeys[s.Key] {
			t.Errorf("Secret.Key %q not in valid config.Env* constants", s.Key)
		}
	}

	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found on PATH, skipping go vet test")
		return
	}
	cmd := exec.Command(goPath, "vet", "-tags", "deploy", "../config/...")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet -tags deploy failed: %v\nOutput: %s", err, string(out))
	}
}

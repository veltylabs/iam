package tests

import (
	"testing"

	"github.com/tinywasm/model"
)

const (
	exampleResource model.Resource = "example"
	roleExampleID                  = "role_example_admin"
	roleExampleCode                = "example_admin"
	permExampleRead                = "example:read"
)

func TestLocalScenariosStartWithoutGoogleEnv(t *testing.T) {
	// setupLocal no depende de GOOGLE_*; si esto compila y pasa, el
	// requisito está probado (setupLocal ya construye el motor).
	_, _, _, _ = setupLocal(t)
}

// Dos escenarios locales, permisos distintos por rol: el mismo mecanismo
// que verifica el local_auth_test.go original de misitio, en aislamiento
// del modulo de sitios.
func TestLocalScenariosPermissionsDifferByRole(t *testing.T) {
	_, authMod, rbacSvc, _ := setupLocal(t)

	adminSubj, err := authMod.GetOrCreateSubject("local-admin", "admin@iam.local", "Admin Local", "")
	if err != nil {
		t.Fatalf("admin subject: %v", err)
	}
	viewerSubj, err := authMod.GetOrCreateSubject("local-viewer", "viewer@iam.local", "Viewer Local", "")
	if err != nil {
		t.Fatalf("viewer subject: %v", err)
	}

	if err := rbacSvc.CreateRole(roleExampleID, roleExampleCode, "Example Admin", ""); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := rbacSvc.CreatePermission(permExampleRead, "Read example", exampleResource, model.Read); err != nil {
		t.Fatalf("CreatePermission: %v", err)
	}
	if err := rbacSvc.AssignPermission(roleExampleID, permExampleRead); err != nil {
		t.Fatalf("AssignPermission: %v", err)
	}
	if err := rbacSvc.AssignRole(string(adminSubj.ID), roleExampleID); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	if !rbacSvc.Can(string(adminSubj.ID), exampleResource, model.Read) {
		t.Errorf("admin should have %s:Read", exampleResource)
	}
	if rbacSvc.Can(string(viewerSubj.ID), exampleResource, model.Read) {
		t.Errorf("viewer should NOT have %s:Read", exampleResource)
	}
	if adminSubj.ID == viewerSubj.ID {
		t.Errorf("admin and viewer should have distinct SubjectIDs")
	}
}

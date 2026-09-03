package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/veltylabs/iam/config"
)

func runCmd(t *testing.T, cmd string, args ...string) string {
	t.Helper()
	c := exec.Command(cmd, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %v: %v\nOutput: %s", cmd, args, err, string(out))
	}
	return string(out)
}

func TestConfigIsALeaf(t *testing.T) {
	out := runCmd(t, "go", "list", "-deps", "../config")
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "github.com/veltylabs/iam/") && l != "github.com/veltylabs/iam/config" {
			t.Fatalf("config/ imports forbidden repo package: %s", l)
		}
	}
}

func TestNoApiPackage(t *testing.T) {
	if _, err := os.Stat("../api"); !os.IsNotExist(err) {
		t.Fatalf("api/ package directory still exists")
	}
}

func TestRoutesDoesNotImportPanel(t *testing.T) {
	out := runCmd(t, "go", "list", "-deps", "../routes")
	if strings.Contains(out, "github.com/veltylabs/iam/modules/panel") {
		t.Fatalf("routes/ transitively imports modules/panel")
	}
}

func TestEdgeDoesNotImportUIKit(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "../edge")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./edge/ failed: %v\nOutput: %s", err, string(out))
	}
	sOut := string(out)
	forbidden := []string{
		"github.com/veltylabs/iam/modules/panel",
		"github.com/tinywasm/dom",
		"github.com/tinywasm/html",
		"github.com/tinywasm/form",
		"github.com/tinywasm/layout",
		"github.com/tinywasm/svg",
	}
	for _, f := range forbidden {
		if strings.Contains(sOut, f) {
			t.Fatalf("edge/ transitively imports forbidden package: %s", f)
		}
	}
}

func TestModulesHaveNoSubdirectories(t *testing.T) {
	entries, err := os.ReadDir("../modules")
	if err != nil {
		t.Fatalf("ReadDir modules: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			subEntries, err := os.ReadDir("../modules/" + e.Name())
			if err != nil {
				t.Fatalf("ReadDir module subDir: %v", err)
			}
			for _, se := range subEntries {
				if se.IsDir() {
					t.Fatalf("module %s has subdirectory %s", e.Name(), se.Name())
				}
			}
		}
	}
}

func TestClientDoesNotImportRoutes(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "../web")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./web failed: %v\nOutput: %s", err, string(out))
	}
	sOut := string(out)
	if strings.Contains(sOut, "github.com/veltylabs/iam/routes") || strings.Contains(sOut, "github.com/veltylabs/iam/modules/admin") {
		t.Fatalf("web/ (client.wasm) transitively imports routes or modules/admin")
	}
}

func TestRouteTableIsSingleFile(t *testing.T) {
	entries, err := os.ReadDir("../routes")
	if err != nil {
		t.Fatalf("ReadDir routes: %v", err)
	}
	// La tabla de rutas sigue siendo un solo archivo (routes.go); headers.go
	// es el middleware de cabeceras de seguridad, no una tabla de rutas.
	allowed := map[string]bool{"routes.go": true, "headers.go": true}
	for _, e := range entries {
		if !allowed[e.Name()] {
			t.Fatalf("routes/ directory contains unexpected file %s (want only routes.go and headers.go)", e.Name())
		}
	}
	if len(entries) != len(allowed) {
		t.Fatalf("routes/ directory is missing an expected file: got %d entries, want %d", len(entries), len(allowed))
	}
}

func TestPanelViewsInViewGo(t *testing.T) {
	entries, err := os.ReadDir("../modules/panel")
	if err != nil {
		t.Fatalf("ReadDir modules/panel: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && e.Name() != "view.go" {
			content, err := os.ReadFile("../modules/panel/" + e.Name())
			if err != nil {
				t.Fatalf("ReadFile %s: %v", e.Name(), err)
			}
			if strings.Contains(string(content), "dom.Component") && strings.Contains(string(content), "func ") && strings.Contains(string(content), "View(") {
				t.Fatalf("view builder component declared outside view.go in file %s", e.Name())
			}
		}
	}
}

// TestAdminPathsAreUnderAPIPrefix: toda constante PathAdmin* empieza con
// /api/ — la convención worker-first del ecosistema (I-8).
func TestAdminPathsAreUnderAPIPrefix(t *testing.T) {
	paths := map[string]string{
		"PathAdminMe":            config.PathAdminMe,
		"PathAdminProjects":      config.PathAdminProjects,
		"PathAdminProjectRotate": config.PathAdminProjectRotate,
		"PathAdminProjectActive": config.PathAdminProjectActive,
		"PathAdminRoles":         config.PathAdminRoles,
		"PathAdminRoleTTL":       config.PathAdminRoleTTL,
		"PathAdminRoleDelete":    config.PathAdminRoleDelete,
		"PathAdminRoleUsers":     config.PathAdminRoleUsers,
		"PathAdminUserAssign":    config.PathAdminUserAssign,
		"PathAdminUserRevoke":    config.PathAdminUserRevoke,
		"PathAdminAudit":         config.PathAdminAudit,
	}
	for name, path := range paths {
		if !strings.HasPrefix(path, "/api/") {
			t.Errorf("%s = %q, want prefix /api/", name, path)
		}
	}
}

// TestNoLocalQueryParser: modules/admin/ no tiene su propio parser de query
// string — usa router.QueryParam (Restricción #2).
func TestNoLocalQueryParser(t *testing.T) {
	entries, err := os.ReadDir("../modules/admin")
	if err != nil {
		t.Fatalf("ReadDir modules/admin: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		content, err := os.ReadFile("../modules/admin/" + e.Name())
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e.Name(), err)
		}
		if strings.Contains(string(content), "func getQueryParam") {
			t.Errorf("modules/admin/%s still defines a local getQueryParam", e.Name())
		}
	}
}

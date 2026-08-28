package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"
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
	if len(entries) != 1 || entries[0].Name() != "routes.go" {
		t.Fatalf("routes/ directory contains files other than routes.go")
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

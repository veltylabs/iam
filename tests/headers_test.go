package tests

import (
	"strings"
	"testing"

	"github.com/veltylabs/iam/routes"
)

// TestSecurityHeaders_OnEveryRoute recorre TODAS las rutas registradas e
// invoca cada una con handler, afirmando las seis cabeceras. Un test que las
// chequea en una sola ruta no fija nada: una ruta agregada sin pasar por el
// middleware quedaría desprotegida en silencio.
func TestSecurityHeaders_OnEveryRoute(t *testing.T) {
	b, r := setupPanel(t, "admin@example.com")

	want := map[string]string{
		routes.HeaderCSP:            "",
		routes.HeaderFrameOptions:   "",
		routes.HeaderContentType:    "",
		routes.HeaderReferrerPolicy: "",
		routes.HeaderHSTS:           "",
		routes.HeaderPermissions:    "",
	}

	infos := r.Routes()
	if len(infos) == 0 {
		t.Fatal("no routes registered")
	}
	for _, info := range infos {
		ctx := adminCtx(t, b, "admin@example.com")
		ctx.InMethod = info.Method
		ctx.InPath = info.Path
		r.Invoke(info.Method, info.Path, ctx)
		for header := range want {
			if got := ctx.GetHeader(header); got == "" {
				t.Errorf("%s %s: missing security header %s", info.Method, info.Path, header)
			}
		}
	}
}

// TestCSPForbidsUnsafeInline: el valor de CSP no contiene unsafe-inline ni
// unsafe-eval como directivas propias (sí puede contener wasm-unsafe-eval,
// que es lo que exige instanciar un módulo WASM y no habilita eval()).
func TestCSPForbidsUnsafeInline(t *testing.T) {
	b, r := setupPanel(t, "admin@example.com")

	var csp string
	for _, info := range r.Routes() {
		ctx := adminCtx(t, b, "admin@example.com")
		ctx.InMethod = info.Method
		ctx.InPath = info.Path
		r.Invoke(info.Method, info.Path, ctx)
		if got := ctx.GetHeader(routes.HeaderCSP); got != "" {
			csp = got
			break
		}
	}
	if csp == "" {
		t.Fatal("no CSP header emitted on any route")
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP allows unsafe-inline: %q", csp)
	}
	rest := strings.ReplaceAll(csp, "wasm-unsafe-eval", "")
	if strings.Contains(rest, "unsafe-eval") {
		t.Errorf("CSP allows unsafe-eval: %q", csp)
	}
	if !strings.Contains(csp, "wasm-unsafe-eval") {
		t.Errorf("CSP must allow wasm-unsafe-eval for the WASM panel: %q", csp)
	}
}

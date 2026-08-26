package tests

import (
	"testing"

	"github.com/tinywasm/router"
	"github.com/veltylabs/iam/routes"
)

type mockContext struct {
	router.Context
	status int
	body   []byte
}

func (m *mockContext) SetHeader(key, value string) {}
func (m *mockContext) WriteStatus(status int) {
	m.status = status
}
func (m *mockContext) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func TestHealthDoesNotTouchDB(t *testing.T) {
	handler := routes.Health()
	ctx := &mockContext{}
	handler(ctx)

	if ctx.status != 200 {
		t.Fatalf("expected status 200, got %d", ctx.status)
	}
	if string(ctx.body) != `{"ok":true}` {
		t.Fatalf("expected body {\"ok\":true}, got %q", string(ctx.body))
	}
}

func TestHealthDBReportsFailure(t *testing.T) {
	handler := routes.HealthDB(nil)
	ctx := &mockContext{}
	handler(ctx)

	if ctx.status != 503 {
		t.Fatalf("expected status 503, got %d", ctx.status)
	}
	if string(ctx.body) != `{"ok":false}` {
		t.Fatalf("expected body {\"ok\":false}, got %q", string(ctx.body))
	}
}

func TestHealthDBReportsSuccess(t *testing.T) {
	db, _, _, _ := setup(t)
	handler := routes.HealthDB(db)
	ctx := &mockContext{}
	handler(ctx)

	if ctx.status != 200 {
		t.Fatalf("expected status 200, got %d", ctx.status)
	}
	if string(ctx.body) != `{"ok":true}` {
		t.Fatalf("expected body {\"ok\":true}, got %q", string(ctx.body))
	}
}

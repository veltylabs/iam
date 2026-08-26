package tests

import (
	"testing"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/router/mock"
	"github.com/tinywasm/storage/mem"
	"github.com/veltylabs/iam/routes"
)

func TestHealthDoesNotTouchDB(t *testing.T) {
	handler := routes.Health()
	ctx := &mock.Context{}
	handler(ctx)

	if ctx.Status != 200 {
		t.Errorf("status: got %d, want 200", ctx.Status)
	}
	if got := string(ctx.ResponseBody()); got != `{"ok":true}` {
		t.Errorf("body: got %q, want %q", got, `{"ok":true}`)
	}
}

func TestHealthDBReportsFailure(t *testing.T) {
	handler := routes.HealthDB(nil)
	ctx := &mock.Context{}
	handler(ctx)

	if ctx.Status != 503 {
		t.Errorf("status: got %d, want 503", ctx.Status)
	}
	if got := string(ctx.ResponseBody()); got != `{"ok":false}` {
		t.Errorf("body: got %q, want %q", got, `{"ok":false}`)
	}
}

func TestHealthDBReportsSuccess(t *testing.T) {
	db := orm.New(mem.New())
	handler := routes.HealthDB(db)
	ctx := &mock.Context{}
	handler(ctx)

	if ctx.Status != 200 {
		t.Errorf("status: got %d, want 200", ctx.Status)
	}
	if got := string(ctx.ResponseBody()); got != `{"ok":true}` {
		t.Errorf("body: got %q, want %q", got, `{"ok":true}`)
	}
}

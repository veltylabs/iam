//go:build wasm

package tests

import (
	"testing"

	"github.com/tinywasm/dom"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/panel"
)

func TestPanelViewBuilderFunctions(t *testing.T) {
	me := config.AdminMeResponse{Email: "admin@example.com", Name: "Admin"}
	panel.Boot(me)

	ref, ok := dom.Get(panel.IDProjectsList)
	if !ok || ref == nil {
		t.Fatalf("panel.Boot failed to mount projects list container")
	}
}

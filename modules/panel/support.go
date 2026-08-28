//go:build wasm

package panel

import (
	"github.com/tinywasm/dom"
	"github.com/tinywasm/html"
)

func ShowStatus(msg string) {
	ref, ok := dom.Get("iam-status-message")
	if !ok {
		el := html.Div().ID("iam-status-message").Class("iam-status-banner").Text(msg)
		_ = dom.Append("body", el)
	} else {
		ref.SetText(msg)
	}
}

func HideStatus() {
	ref, ok := dom.Get("iam-status-message")
	if ok {
		ref.SetText("")
	}
}

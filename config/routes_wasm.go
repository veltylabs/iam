//go:build wasm

package config

import "webtyp.com/router"

// registerLocal es un stub no-op para entornos WebAssembly/edge.
func registerLocal(r router.Router) {}


// Package api holds the wire-facing constants shared by config/, routes/
// and any future client, so none of them need to import each other just to
// agree on a path or binding name (breaks the config↔routes cycle).
package api

const (
	PathHealth = "/api/health"
	PathToken  = "/api/token"
)

// BindingD1 es el nombre del binding de D1 declarado en Cloudflare para la
// base propia de iam — no comparte la de ningún consumidor (ver AGENTS.md
// Restricción #5).
const BindingD1 = "DB"

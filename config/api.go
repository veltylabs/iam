package config

// BindingD1 es el nombre del binding de D1 declarado en Cloudflare para la
// base propia de iam — no comparte la de ningún consumidor (ver AGENTS.md
// Restricción #5).
const BindingD1 = "DB"

// EnvPanelOrigin es el origen exacto desde el que se sirve el panel
// ("https://iam.velty.cl"). Se utiliza para:
// 1. Origen del panel y verificación de Same-Origin CSRF.
// 2. Origen del Relying Party de WebAuthn/Passkey.
// 3. Audience para la verificación de aserciones de clientes máquina (RFC 7523).
const EnvPanelOrigin = "IAM_PANEL_ORIGIN"

// LocalPanelOrigin es el origen del panel sólo para desarrollo local
// (config.NewLocalBackend). Nunca se usa en producción: NewProductionBackend
// exige EnvPanelOrigin y falla rápido si falta.
const LocalPanelOrigin = "http://localhost:8080"

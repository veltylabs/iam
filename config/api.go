package config

const (
	PathHealth       = "/api/health"
	PathHealthDB     = "/api/health/db"
	PathToken        = "/api/token"
	PathUsersResolve = "/api/users/resolve"
	PathRolesAssign  = "/api/roles/assign"

	PathAdminMe            = "/api/admin/me"
	PathAdminProjects      = "/api/admin/projects"        // GET lista · POST crea
	PathAdminProjectRotate = "/api/admin/projects/rotate" // POST
	PathAdminProjectActive = "/api/admin/projects/active" // POST
	PathAdminRoles         = "/api/admin/roles"           // GET ?project_id= · POST crea
	PathAdminRoleTTL       = "/api/admin/roles/ttl"       // POST
	PathAdminRoleDelete    = "/api/admin/roles/delete"    // POST
	PathAdminRoleUsers     = "/api/admin/roles/users"     // GET ?project_id=&code=
	PathAdminUserAssign    = "/api/admin/users/assign"    // POST
	PathAdminUserRevoke    = "/api/admin/users/revoke"    // POST
	PathAdminAudit         = "/api/admin/audit"           // GET
)

// BindingD1 es el nombre del binding de D1 declarado en Cloudflare para la
// base propia de iam — no comparte la de ningún consumidor (ver AGENTS.md
// Restricción #5).
const BindingD1 = "DB"

// EnvPanelOrigin es el origen exacto desde el que se sirve el panel
// ("https://iam.velty.cl"). Sin ella, iam NO arranca: adivinarla y errarle
// deja el panel inutilizable, y aceptarla ausente deja la puerta abierta.
const EnvPanelOrigin = "IAM_PANEL_ORIGIN"

// LocalPanelOrigin es el origen del panel sólo para desarrollo local
// (config.NewLocalBackend). Nunca se usa en producción: NewProductionBackend
// exige EnvPanelOrigin y falla rápido si falta.
const LocalPanelOrigin = "http://localhost:8080"

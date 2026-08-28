package config

const (
	PathHealth       = "/api/health"
	PathHealthDB     = "/api/health/db"
	PathToken        = "/api/token"
	PathUsersResolve = "/api/users/resolve"
	PathRolesAssign  = "/api/roles/assign"

	PathAdminMe            = "/admin/api/me"
	PathAdminProjects      = "/admin/api/projects"        // GET lista · POST crea
	PathAdminProjectRotate = "/admin/api/projects/rotate" // POST
	PathAdminProjectActive = "/admin/api/projects/active" // POST
	PathAdminRoles         = "/admin/api/roles"           // GET ?project_id= · POST crea
	PathAdminRoleTTL       = "/admin/api/roles/ttl"       // POST
	PathAdminRoleDelete    = "/admin/api/roles/delete"    // POST
	PathAdminRoleUsers     = "/admin/api/roles/users"     // GET ?project_id=&code=
	PathAdminUserAssign    = "/admin/api/users/assign"    // POST
	PathAdminUserRevoke    = "/admin/api/users/revoke"    // POST
	PathAdminAudit         = "/admin/api/audit"           // GET
)

// BindingD1 es el nombre del binding de D1 declarado en Cloudflare para la
// base propia de iam — no comparte la de ningún consumidor (ver AGENTS.md
// Restricción #5).
const BindingD1 = "DB"

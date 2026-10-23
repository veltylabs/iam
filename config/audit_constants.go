package config

const (
	AuditProjectCreate     = "project.create"
	AuditProjectRotate     = "project.rotate_secret"
	AuditProjectActivate   = "project.activate"
	AuditProjectDeactivate = "project.deactivate"
	AuditRoleCreate        = "role.create"
	AuditRoleDelete        = "role.delete"
	AuditRoleSetTTL        = "role.set_ttl"
	AuditUserAssign        = "user.assign_role"
	AuditUserRevoke        = "user.revoke_role"
	AuditRoleGrantedByClient     = "client.assign_role"
	AuditIdentityCreatedByClient = "client.create_identity"

	AuditPanelDenied  = "panel.access_denied"  // sesión válida, email fuera de IAM_ADMIN_EMAILS
	AuditOriginDenied = "panel.origin_denied"  // mutación desde un origen ajeno
	AuditTokenDenied  = "token.secret_invalid" // client_secret inválido en /api/token
)

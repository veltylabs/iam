package panel

import (
	"github.com/tinywasm/svg"
)

const (
	ModuleProjects = "projects"
	ModuleRoles    = "roles"
	ModuleUsers    = "users"
	ModuleAudit    = "audit"
)

const (
	IconProjects = svg.Icon("iam-projects")
	IconRoles    = svg.Icon("iam-roles")
	IconUsers    = svg.Icon("iam-users")
	IconAudit    = svg.Icon("iam-audit")
)

const (
	IDProjectsList   = "iam-projects-list"
	IDProjectsNew    = "iam-projects-new"
	IDProjectsSecret = "iam-projects-secret" // bloque destacado del secreto
	IDRolesProject   = "iam-roles-project"
	IDRolesList      = "iam-roles-list"
	IDRolesNew       = "iam-roles-new"
	IDUsersForm      = "iam-users-form"
	IDUsersList      = "iam-users-list"
	IDAuditList      = "iam-audit-list"
)

package admin

import (
	"github.com/veltylabs/iam/config"
)

var RequirePanelAdmin = config.RequirePanelAdmin
var Me = config.Me
var ListProjectsHandler = config.ListProjectsHandler
var CreateProjectHandler = config.CreateProjectHandler
var RotateSecretHandler = config.RotateSecretHandler
var SetActiveHandler = config.SetActiveHandler
var ListRolesHandler = config.ListRolesHandler
var CreateRoleHandler = config.CreateRoleHandler
var SetRoleTTLHandler = config.SetRoleTTLHandler
var DeleteRoleHandler = config.DeleteRoleHandler
var ListRoleUsersHandler = config.ListRoleUsersHandler
var AssignUserHandler = config.AssignUserHandler
var RevokeUserHandler = config.RevokeUserHandler
var ListAuditHandler = config.ListAuditHandler

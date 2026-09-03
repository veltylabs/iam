package admin

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/veltylabs/iam/config"
)

// ListRoles obtiene todos los roles de projectID formateados como config.RoleView, incluyendo UserCount.
func ListRoles(db *orm.DB, rbacSvc *rbac.Service, projectID string) ([]config.RoleView, error) {
	qb := db.Query(&rbac.Role{}).Where(rbac.Role_.ProjectId).Eq(projectID)
	roles, err := rbac.ReadAllRole(qb)
	if err != nil {
		return nil, err
	}
	views := make([]config.RoleView, 0, len(roles))
	for _, r := range roles {
		count, err := rbacSvc.RoleUserCount(projectID, model.RoleCode(r.Code))
		if err != nil {
			return nil, err
		}
		views = append(views, config.RoleView{
			Code:        string(r.Code),
			Name:        r.Name,
			Description: r.Description,
			SessionTtl:  r.SessionTtl,
			UserCount:   count,
		})
	}
	return views, nil
}

// ListRoleUsers devuelve la lista de usuarios asignados al roleCode en projectID.
func ListRoleUsers(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, projectID, roleCode string) ([]config.RoleUser, error) {
	userIDs, err := rbacSvc.UsersInRole(projectID, model.RoleCode(roleCode))
	if err != nil {
		return nil, err
	}
	users := make([]config.RoleUser, 0, len(userIDs))
	for _, userID := range userIDs {
		u, err := authMod.GetUser(userID)
		if err != nil {
			continue
		}
		users = append(users, config.RoleUser{
			Email: u.Email,
			Name:  u.Name,
			Sub:   u.Id,
		})
	}
	return users, nil
}

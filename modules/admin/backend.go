package admin

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/veltylabs/iam/config"
)

// ListRoles obtiene todos los roles de projectID formateados como config.RoleView, incluyendo UserCount.
func ListRoles(db *orm.DB, projectID string) ([]config.RoleView, error) {
	qb := db.Query(&rbac.Role{}).Where(rbac.Role_.ProjectId).Eq(projectID)
	roles, err := rbac.ReadAllRole(qb)
	if err != nil {
		return nil, err
	}
	views := make([]config.RoleView, 0, len(roles))
	for _, r := range roles {
		uCountQB := db.Query(&rbac.UserRole{}).
			Where(rbac.UserRole_.ProjectId).Eq(projectID).
			Where(rbac.UserRole_.RoleId).Eq(r.Id)
		userRoles, err := rbac.ReadAllUserRole(uCountQB)
		if err != nil {
			return nil, err
		}
		views = append(views, config.RoleView{
			Code:        string(r.Code),
			Name:        r.Name,
			Description: r.Description,
			SessionTtl:  r.SessionTtl,
			UserCount:   int64(len(userRoles)),
		})
	}
	return views, nil
}

// ListRoleUsers devuelve la lista de usuarios asignados al roleCode en projectID.
func ListRoleUsers(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, projectID, roleCode string) ([]config.RoleUser, error) {
	role, err := rbacSvc.GetRoleByCode(projectID, model.RoleCode(roleCode))
	if err != nil {
		return nil, err
	}
	uQB := db.Query(&rbac.UserRole{}).
		Where(rbac.UserRole_.ProjectId).Eq(projectID).
		Where(rbac.UserRole_.RoleId).Eq(role.Id)
	userRoles, err := rbac.ReadAllUserRole(uQB)
	if err != nil {
		return nil, err
	}
	users := make([]config.RoleUser, 0, len(userRoles))
	for _, ur := range userRoles {
		u, err := authMod.GetUser(ur.UserId)
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

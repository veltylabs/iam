package admin

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/router"
	"github.com/tinywasm/storage"
	"github.com/veltylabs/iam/config"
)

// RequirePanelAdmin envuelve un handler: 401 sin sesión, 403 si el correo de
// la sesión no está en adminEmails. En el camino feliz pasa el correo del
// admin al handler (para auditoría).
func RequirePanelAdmin(authMod *authority.Module, adminEmails []string, h func(ctx router.Context, adminEmail string)) router.HandlerFunc {
	return func(ctx router.Context) {
		uid := ctx.UserID()
		if uid == "" {
			ctx.WriteStatus(401)
			return
		}
		u, err := authMod.GetUser(uid)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !config.IsPanelAdmin(u.Email, adminEmails) {
			ctx.WriteStatus(403)
			return
		}
		h(ctx, u.Email)
	}
}

// helper para parsear query params de Path()
func getQueryParam(path, key string) string {
	idx := fmt.Index(path, "?")
	if idx == -1 {
		return ""
	}
	query := path[idx+1:]
	parts := fmt.Split(query, "&")
	prefix := key + "="
	for _, p := range parts {
		if fmt.HasPrefix(p, prefix) {
			return p[len(prefix):]
		}
	}
	return ""
}

// Me responde con datos del administrador actual.
func Me(authMod *authority.Module) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		u, err := authMod.GetUser(ctx.UserID())
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminMeResponse{
			Email: adminEmail,
			Name:  u.Name,
		})
	}
}

// ListProjectsHandler lista todos los proyectos.
func ListProjectsHandler(db *orm.DB) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		projects, err := config.ListProjects(db)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		views := make([]config.AdminProjectView, 0, len(projects))
		for _, p := range projects {
			views = append(views, config.AdminProjectView{
				Id:        p.Id,
				Name:      p.Name,
				Active:    p.Active != 0,
				CreatedAt: p.CreatedAt,
			})
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminProjectsResponse{Projects: views})
	}
}

// CreateProjectHandler crea un nuevo proyecto y devuelve su client_secret en claro.
func CreateProjectHandler(db *orm.DB, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminCreateProjectRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.Name) == "" {
			ctx.WriteStatus(400)
			return
		}
		id := ids.NewID()
		secret, err := config.GenerateClientSecret()
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if err := config.CreateProject(db, id, req.Name, secret); err != nil {
			ctx.WriteStatus(500)
			return
		}
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditProjectCreate, id, req.Name); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(201)
		_ = ctx.Encode(&config.AdminSecretResponse{
			Id:           id,
			ClientSecret: secret,
		})
	}
}

// RotateSecretHandler regenera el client_secret de un proyecto.
func RotateSecretHandler(db *orm.DB, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminProjectIDRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" {
			ctx.WriteStatus(400)
			return
		}
		secret, err := config.GenerateClientSecret()
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if err := config.RegenerateProjectSecret(db, req.ProjectId, secret); err != nil {
			if err == config.ErrProjectNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditProjectRotate, req.ProjectId, ""); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminSecretResponse{
			Id:           req.ProjectId,
			ClientSecret: secret,
		})
	}
}

// SetActiveHandler activa o desactiva un proyecto.
func SetActiveHandler(db *orm.DB, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminSetActiveRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" {
			ctx.WriteStatus(400)
			return
		}
		if err := config.SetProjectActive(db, req.ProjectId, req.Active); err != nil {
			if err == config.ErrProjectNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		action := config.AuditProjectActivate
		if !req.Active {
			action = config.AuditProjectDeactivate
		}
		if err := config.RecordAudit(db, ids, adminEmail, action, req.ProjectId, ""); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminProjectIDRequest{ProjectId: req.ProjectId})
	}
}

// ListRolesHandler lista los roles de un proyecto dado por query param ?project_id=.
func ListRolesHandler(db *orm.DB) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		projectID := getQueryParam(ctx.Path(), "project_id")
		if fmt.TrimSpace(projectID) == "" {
			ctx.WriteStatus(400)
			return
		}
		roles, err := ListRoles(db, projectID)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminRolesResponse{Roles: roles})
	}
}

// CreateRoleHandler crea un rol dentro de un proyecto.
func CreateRoleHandler(db *orm.DB, rbacSvc *rbac.Service, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminCreateRoleRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" || fmt.TrimSpace(req.Code) == "" {
			ctx.WriteStatus(400)
			return
		}
		roleID := ids.NewID()
		if err := rbacSvc.CreateRole(req.ProjectId, roleID, model.RoleCode(req.Code), req.Name, req.Description); err != nil {
			ctx.WriteStatus(500)
			return
		}
		target := req.ProjectId + "/" + req.Code
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditRoleCreate, target, req.Name); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(201)
		_ = ctx.Encode(&config.AdminRoleRefRequest{ProjectId: req.ProjectId, Code: req.Code})
	}
}

// SetRoleTTLHandler actualiza el SessionTTL de un rol.
func SetRoleTTLHandler(db *orm.DB, rbacSvc *rbac.Service, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminRoleTTLRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" || fmt.TrimSpace(req.Code) == "" {
			ctx.WriteStatus(400)
			return
		}
		role, err := rbacSvc.GetRoleByCode(req.ProjectId, model.RoleCode(req.Code))
		if err != nil {
			if err == orm.ErrNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		if err := rbacSvc.SetRoleSessionTTL(req.ProjectId, role.Id, req.SessionTtl); err != nil {
			ctx.WriteStatus(500)
			return
		}
		target := req.ProjectId + "/" + req.Code
		detail := fmt.Sprintf("%d", req.SessionTtl)
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditRoleSetTTL, target, detail); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminRoleRefRequest{ProjectId: req.ProjectId, Code: req.Code})
	}
}

// DeleteRoleHandler elimina un rol de un proyecto.
func DeleteRoleHandler(db *orm.DB, rbacSvc *rbac.Service, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminRoleRefRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" || fmt.TrimSpace(req.Code) == "" {
			ctx.WriteStatus(400)
			return
		}
		role, err := rbacSvc.GetRoleByCode(req.ProjectId, model.RoleCode(req.Code))
		if err != nil {
			if err == orm.ErrNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		if err := rbacSvc.DeleteRole(req.ProjectId, role.Id); err != nil {
			ctx.WriteStatus(500)
			return
		}
		target := req.ProjectId + "/" + req.Code
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditRoleDelete, target, ""); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminRoleRefRequest{ProjectId: req.ProjectId, Code: req.Code})
	}
}

// ListRoleUsersHandler lista usuarios asignados a un rol (?project_id=&code=).
func ListRoleUsersHandler(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		projectID := getQueryParam(ctx.Path(), "project_id")
		code := getQueryParam(ctx.Path(), "code")
		if fmt.TrimSpace(projectID) == "" || fmt.TrimSpace(code) == "" {
			ctx.WriteStatus(400)
			return
		}
		users, err := ListRoleUsers(db, authMod, rbacSvc, projectID, code)
		if err != nil {
			if err == orm.ErrNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminRoleUsersResponse{Users: users})
	}
}

// AssignUserHandler asigna un rol a un usuario por email (lo crea si no existe).
func AssignUserHandler(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminUserRoleRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" || fmt.TrimSpace(req.Code) == "" || fmt.TrimSpace(req.Email) == "" {
			ctx.WriteStatus(400)
			return
		}
		u, err := authMod.UserByEmail(req.Email)
		if err != nil {
			u, err = authMod.CreateUser(req.Email, req.Email, "")
			if err != nil {
				ctx.WriteStatus(500)
				return
			}
		}
		role, err := rbacSvc.GetRoleByCode(req.ProjectId, model.RoleCode(req.Code))
		if err != nil {
			if err == orm.ErrNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		if err := rbacSvc.AssignRole(req.ProjectId, u.Id, role.Id); err != nil {
			ctx.WriteStatus(500)
			return
		}
		target := req.ProjectId + "/" + req.Code
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditUserAssign, target, req.Email); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminAssignResponse{Sub: u.Id})
	}
}

// RevokeUserHandler revoca un rol a un usuario.
func RevokeUserHandler(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, ids model.IDGenerator) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		var req config.AdminUserRoleRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" || fmt.TrimSpace(req.Code) == "" || fmt.TrimSpace(req.Email) == "" {
			ctx.WriteStatus(400)
			return
		}
		u, err := authMod.UserByEmail(req.Email)
		if err != nil {
			ctx.WriteStatus(404)
			return
		}
		role, err := rbacSvc.GetRoleByCode(req.ProjectId, model.RoleCode(req.Code))
		if err != nil {
			if err == orm.ErrNotFound {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		if err := db.Delete(&rbac.UserRole{}, storage.Eq(rbac.UserRole_.ProjectId, req.ProjectId), storage.Eq(rbac.UserRole_.UserId, u.Id), storage.Eq(rbac.UserRole_.RoleId, role.Id)); err != nil {
			ctx.WriteStatus(500)
			return
		}
		target := req.ProjectId + "/" + req.Code
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditUserRevoke, target, req.Email); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminAssignResponse{Sub: u.Id})
	}
}

// ListAuditHandler lista las entradas del registro de auditoría.
func ListAuditHandler(db *orm.DB) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		entries, err := config.ListAudit(db, 200)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		views := make([]config.AdminAuditEntry, 0, len(entries))
		for _, e := range entries {
			views = append(views, config.AdminAuditEntry{
				ActorEmail: e.ActorEmail,
				Action:     e.Action,
				Target:     e.Target,
				Detail:     e.Detail,
				CreatedAt:  e.CreatedAt,
			})
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&config.AdminAuditResponse{Entries: views})
	}
}

package config

import (
	"webtyp.com/access"
	"webtyp.com/auth/authority"
	"webtyp.com/auth/passkey"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/rbac"
	"webtyp.com/router"
)

func adminEmailOf(authMod *authority.Module, ctx router.Context) string {
	if uid := ctx.UserID(); uid != "" {
		if u, err := authMod.GetUser(uid); err == nil {
			return u.Email
		}
	}
	return ""
}

// Me responde con datos del administrador actual.
func Me(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service) router.HandlerFunc {
	return func(ctx router.Context) {
		uid := ctx.UserID()
		if uid == "" {
			ctx.WriteStatus(403)
			return
		}
		u, err := authMod.GetUser(uid)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}

		isAdmin := rbacSvc.Can(IAMProjectID, uid, ResourceProject, model.Read)

		qb := db.Query(&passkey.PasskeyCredential{}).Where(passkey.PasskeyCredential_.UserId).Eq(uid)
		creds, err := passkey.ReadAllPasskeyCredential(qb)
		hasPasskey := err == nil && len(creds) > 0

		multiFactor := ctx.Assurance() == model.AssuranceMultiFactor

		ctx.WriteStatus(200)
		_ = ctx.Encode(&AdminMeResponse{
			Email:       u.Email,
			Name:        u.Name,
			IsAdmin:     isAdmin,
			HasPasskey:  hasPasskey,
			MultiFactor: multiFactor,
		})
	}
}

// ListProjectsHandler lista todos los proyectos.
func ListProjectsHandler(db *orm.DB) router.HandlerFunc {
	return func(ctx router.Context) {
		projects, err := ListProjects(db)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		views := make([]AdminProjectView, 0, len(projects))
		for _, p := range projects {
			views = append(views, AdminProjectView{
				Id:        p.Id,
				Name:      p.Name,
				Active:    p.Active != 0,
				CreatedAt: int64(p.CreatedAt),
			})
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&AdminProjectsResponse{Projects: views})
	}
}

// CreateProjectHandler crea un nuevo proyecto.
func CreateProjectHandler(db *orm.DB, accessMod *access.Module, ids model.IDGenerator, authMod *authority.Module, rbacSvc *rbac.Service) router.HandlerFunc {
	return func(ctx router.Context) {
		var req AdminCreateProjectRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.Name) == "" {
			ctx.WriteStatus(400)
			return
		}
		id := ids.NewID()
		if err := CreateProject(db, id, req.Name, req.PublicKey); err != nil {
			ctx.WriteStatus(400)
			return
		}

		_ = rbacSvc.AssignRoleByCode(IAMProjectID, id, RoleIAMClient)

		adminEmail := adminEmailOf(authMod, ctx)
		if err := accessMod.Record(adminEmail, AuditProjectCreate, id, req.Name); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(201)
		_ = ctx.Encode(&AdminProjectIDRequest{
			ProjectId: id,
		})
	}
}

// SetProjectKeyHandler replaces a project's public key.
func SetProjectKeyHandler(db *orm.DB, accessMod *access.Module, ids model.IDGenerator, authMod *authority.Module) router.HandlerFunc {
	return func(ctx router.Context) {
		var req AdminProjectKeyRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" {
			ctx.WriteStatus(400)
			return
		}
		if err := SetProjectKey(db, req.ProjectId, req.PublicKey); err != nil {
			if IsProjectNotFound(err) {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(400)
			return
		}
		adminEmail := adminEmailOf(authMod, ctx)
		if err := accessMod.Record(adminEmail, AuditProjectRotate, req.ProjectId, ""); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&AdminProjectIDRequest{
			ProjectId: req.ProjectId,
		})
	}
}

// SetActiveHandler activa o desactiva un proyecto.
func SetActiveHandler(db *orm.DB, accessMod *access.Module, ids model.IDGenerator, authMod *authority.Module) router.HandlerFunc {
	return func(ctx router.Context) {
		var req AdminSetActiveRequest
		if err := ctx.Decode(&req); err != nil || fmt.TrimSpace(req.ProjectId) == "" {
			ctx.WriteStatus(400)
			return
		}
		if err := SetProjectActive(db, req.ProjectId, req.Active); err != nil {
			if IsProjectNotFound(err) {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		action := AuditProjectActivate
		if !req.Active {
			action = AuditProjectDeactivate
		}
		adminEmail := adminEmailOf(authMod, ctx)
		if err := accessMod.Record(adminEmail, action, req.ProjectId, ""); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&AdminProjectIDRequest{ProjectId: req.ProjectId})
	}
}

// ListRolesHandler lista los roles de un proyecto dado por query param ?project_id=.

// CreateRoleHandler crea un rol dentro de un proyecto.

// SetRoleTTLHandler actualiza el SessionTTL de un rol.

// DeleteRoleHandler elimina un rol de un proyecto.

// ListRoleUsersHandler lista usuarios asignados a un rol (?project_id=&code=).

// AssignUserHandler asigna un rol a un usuario por email (lo crea si no existe).

// RevokeUserHandler revoca un rol a un usuario.

// ListAuditHandler lista las entradas del registro de auditoría.

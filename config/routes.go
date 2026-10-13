package config

import (
	"webtyp.com/auth/authority"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/rbac"
	"webtyp.com/router"
	"webtyp.com/router/security"
)

const (
	bodyHealthOK   = `{"ok":true}`
	bodyHealthFail = `{"ok":false}`
	healthProbeSQL = "SELECT 1"
)

// Register monta todas las rutas del servicio iam sobre el router provisto para el servidor local.
func Register(r router.Router) {
	registerLocal(r)
}

// RegisterRoutes monta todas las rutas del servicio iam con sus dependencias explícitas
// (usado por edge/main.go, Register local y el arnés de tests).
func RegisterRoutes(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte, adminEmails []string, ids model.IDGenerator, panelOrigin string) {
	policy := security.Policy{}.AllowImages("https://lh3.googleusercontent.com")
	r.Use(policy.Middleware())

	r.Use(authMod.Authenticate())

	modules := []router.APIModule{authMod}
	for _, m := range modules {
		m.MountAPI(r)
	}

	r.Get(PathHealth, Health()).Public()
	r.Get(PathHealthDB, HealthDB(db)).Public()
	r.Post(PathToken, Token(db, authMod, rbacSvc, secret, ids)).Authenticated()
	r.Post(PathUsersResolve, ResolveUser(db, authMod)).Public()
	r.Post(PathRolesAssign, AssignRole(db, rbacSvc)).Public()

	r.Get(PathAdminMe, RequirePanelAdmin(db, ids, authMod, adminEmails, Me(authMod))).Public()
	r.Get(PathAdminProjects, RequirePanelAdmin(db, ids, authMod, adminEmails, ListProjectsHandler(db))).Public()
	adminPost(r, PathAdminProjects, db, authMod, adminEmails, ids, panelOrigin, CreateProjectHandler(db, ids))
	adminPost(r, PathAdminProjectRotate, db, authMod, adminEmails, ids, panelOrigin, RotateSecretHandler(db, ids))
	adminPost(r, PathAdminProjectActive, db, authMod, adminEmails, ids, panelOrigin, SetActiveHandler(db, ids))
	r.Get(PathAdminRoles, RequirePanelAdmin(db, ids, authMod, adminEmails, ListRolesHandler(db, rbacSvc))).Public()
	adminPost(r, PathAdminRoles, db, authMod, adminEmails, ids, panelOrigin, CreateRoleHandler(db, rbacSvc, ids))
	adminPost(r, PathAdminRoleTTL, db, authMod, adminEmails, ids, panelOrigin, SetRoleTTLHandler(db, rbacSvc, ids))
	adminPost(r, PathAdminRoleDelete, db, authMod, adminEmails, ids, panelOrigin, DeleteRoleHandler(db, rbacSvc, ids))
	r.Get(PathAdminRoleUsers, RequirePanelAdmin(db, ids, authMod, adminEmails, ListRoleUsersHandler(db, authMod, rbacSvc))).Public()
	adminPost(r, PathAdminUserAssign, db, authMod, adminEmails, ids, panelOrigin, AssignUserHandler(db, authMod, rbacSvc, ids))
	adminPost(r, PathAdminUserRevoke, db, authMod, adminEmails, ids, panelOrigin, RevokeUserHandler(db, authMod, rbacSvc, ids))
	r.Get(PathAdminAudit, RequirePanelAdmin(db, ids, authMod, adminEmails, ListAuditHandler(db))).Public()

	router.MountIntrospection(r, router.IntrospectionPath, nil).Public()
}

func adminPost(r router.Router, path string, db *orm.DB, authMod *authority.Module, adminEmails []string, ids model.IDGenerator, origin string, h func(router.Context, string)) {
	r.Post(path,
		RequirePanelAdmin(db, ids, authMod, adminEmails,
			RequireSameOrigin(db, ids, origin, h))).Public()
}

// Health responde si el servidor está vivo.
func Health() router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		ctx.WriteStatus(200)
		_, _ = ctx.Write([]byte(bodyHealthOK))
	}
}

// HealthDB comprueba que la base responde.
func HealthDB(db *orm.DB) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")

		if db == nil || db.RawConn() == nil {
			ctx.WriteStatus(503)
			_, _ = ctx.Write([]byte(bodyHealthFail))
			return
		}
		if err := db.RawConn().Exec(healthProbeSQL); err != nil {
			ctx.WriteStatus(503)
			_, _ = ctx.Write([]byte(bodyHealthFail))
			return
		}
		ctx.WriteStatus(200)
		_, _ = ctx.Write([]byte(bodyHealthOK))
	}
}

// TokenRequest es el cuerpo de POST /api/token.
type TokenRequest struct {
	ProjectID    string
	ClientSecret string
}

func (t *TokenRequest) IsNil() bool { return t == nil }
func (t *TokenRequest) EncodeFields(w model.FieldWriter) {
	w.String("project_id", t.ProjectID)
	w.String("client_secret", t.ClientSecret)
}
func (t *TokenRequest) DecodeFields(r model.FieldReader) {
	t.ProjectID, _ = r.String("project_id")
	t.ClientSecret, _ = r.String("client_secret")
}

// TokenResponse es la respuesta de POST /api/token.
type TokenResponse struct {
	Token  string
	Email  string
	Name   string
	Avatar string
}

func (t *TokenResponse) IsNil() bool { return t == nil }
func (t *TokenResponse) EncodeFields(w model.FieldWriter) {
	w.String("token", t.Token)
	w.String("email", t.Email)
	w.String("name", t.Name)
	w.String("avatar", t.Avatar)
}
func (t *TokenResponse) DecodeFields(r model.FieldReader) {
	t.Token, _ = r.String("token")
	t.Email, _ = r.String("email")
	t.Name, _ = r.String("name")
	t.Avatar, _ = r.String("avatar")
}

// Token emite un token de autorización project-scoped para el usuario de la sesión activa.
func Token(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte, ids model.IDGenerator) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		userID := ctx.UserID()
		if userID == "" {
			ctx.WriteStatus(401)
			return
		}
		var body TokenRequest
		if err := ctx.Decode(&body); err != nil {
			ctx.WriteStatus(400)
			return
		}
		ok, err := VerifyProjectSecret(db, body.ProjectID, body.ClientSecret)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !ok {
			if err := RecordAudit(db, ids, "", AuditTokenDenied, body.ProjectID, ""); err != nil {
				fmt.Println("audit:", err)
			}
			ctx.WriteStatus(403)
			return
		}
		token, err := IssueAuthToken(rbacSvc, secret, body.ProjectID, userID)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		u, err := authMod.GetUser(userID)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&TokenResponse{Token: token, Email: u.Email, Name: u.Name, Avatar: u.Avatar})
	}
}

// ResolveUserRequest es el cuerpo de POST /api/users/resolve.
type ResolveUserRequest struct {
	ProjectID    string
	ClientSecret string
	Email        string
	Name         string
}

func (r *ResolveUserRequest) IsNil() bool { return r == nil }
func (r *ResolveUserRequest) EncodeFields(w model.FieldWriter) {
	w.String("project_id", r.ProjectID)
	w.String("client_secret", r.ClientSecret)
	w.String("email", r.Email)
	w.String("name", r.Name)
}
func (r *ResolveUserRequest) DecodeFields(fr model.FieldReader) {
	r.ProjectID, _ = fr.String("project_id")
	r.ClientSecret, _ = fr.String("client_secret")
	r.Email, _ = fr.String("email")
	r.Name, _ = fr.String("name")
}

// ResolveUserResponse es la respuesta de POST /api/users/resolve.
type ResolveUserResponse struct {
	Sub string
}

func (r *ResolveUserResponse) IsNil() bool { return r == nil }
func (r *ResolveUserResponse) EncodeFields(w model.FieldWriter) {
	w.String("sub", r.Sub)
}
func (r *ResolveUserResponse) DecodeFields(fr model.FieldReader) {
	r.Sub, _ = fr.String("sub")
}

// AssignRoleRequest es el cuerpo de POST /api/roles/assign.
type AssignRoleRequest struct {
	ProjectID    string
	ClientSecret string
	UserID       string
	RoleCode     string
}

func (r *AssignRoleRequest) IsNil() bool { return r == nil }
func (r *AssignRoleRequest) EncodeFields(w model.FieldWriter) {
	w.String("project_id", r.ProjectID)
	w.String("client_secret", r.ClientSecret)
	w.String("user_id", r.UserID)
	w.String("role_code", r.RoleCode)
}
func (r *AssignRoleRequest) DecodeFields(fr model.FieldReader) {
	r.ProjectID, _ = fr.String("project_id")
	r.ClientSecret, _ = fr.String("client_secret")
	r.UserID, _ = fr.String("user_id")
	r.RoleCode, _ = fr.String("role_code")
}

type AssignRoleResponse struct{}

func (r *AssignRoleResponse) IsNil() bool                       { return r == nil }
func (r *AssignRoleResponse) EncodeFields(w model.FieldWriter)  {}
func (r *AssignRoleResponse) DecodeFields(fr model.FieldReader) {}

// AssignRole concede roleCode al usuario en el proyecto del llamante.
func AssignRole(db *orm.DB, rbacSvc *rbac.Service) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		var body AssignRoleRequest
		if err := ctx.Decode(&body); err != nil {
			ctx.WriteStatus(400)
			return
		}
		if body.UserID == "" || body.RoleCode == "" {
			ctx.WriteStatus(400)
			return
		}
		ok, err := VerifyProjectSecret(db, body.ProjectID, body.ClientSecret)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !ok {
			ctx.WriteStatus(403)
			return
		}
		if err := rbacSvc.AssignRoleByCode(body.ProjectID, body.UserID, model.RoleCode(body.RoleCode)); err != nil {
			if rbac.IsRoleNotFound(err) {
				ctx.WriteStatus(404)
				return
			}
			ctx.WriteStatus(500)
			return
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&AssignRoleResponse{})
	}
}

// ResolveUser busca un usuario por email dentro de la identidad global de iam y lo crea si no existe.
func ResolveUser(db *orm.DB, authMod *authority.Module) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		var body ResolveUserRequest
		if err := ctx.Decode(&body); err != nil {
			ctx.WriteStatus(400)
			return
		}
		if body.Email == "" {
			ctx.WriteStatus(400)
			return
		}
		ok, err := VerifyProjectSecret(db, body.ProjectID, body.ClientSecret)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !ok {
			ctx.WriteStatus(403)
			return
		}
		u, err := authMod.UserByEmail(body.Email)
		if err != nil {
			u, err = authMod.CreateUser(body.Email, body.Name, "")
			if err != nil {
				ctx.WriteStatus(500)
				return
			}
		}
		ctx.WriteStatus(200)
		_ = ctx.Encode(&ResolveUserResponse{Sub: u.Id})
	}
}

package routes

import (
	"github.com/tinywasm/auth/authority"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/router"
	"github.com/veltylabs/iam/api"
	"github.com/veltylabs/iam/config"
)

const PathHealth = api.PathHealth
const PathHealthDB = api.PathHealthDB
const PathToken = api.PathToken
const PathUsersResolve = api.PathUsersResolve
const BindingD1 = api.BindingD1

const (
	bodyHealthOK   = `{"ok":true}`
	bodyHealthFail = `{"ok":false}`
)

const healthProbeSQL = "SELECT 1"

// Register monta todas las rutas de iam en el router.
//
// modules is the single, exhaustive list of everything this server mounts
// through router.APIModule. It is intentionally the ONLY place iam calls
// MountAPI: a module with routes to serve belongs in this slice, never
// invoked ad hoc elsewhere, so adding a module here is the only way its
// routes reach a caller — and leaving it out is the only way they don't.
// That is what closes the hole this plan exists for: authMod's OAuth routes
// (/oauth/google, /oauth/callback/google) and /logout were built and
// enabled (config/auth.go, config/auth_local.go) but never mounted, so
// every login attempt 404'd, silently, since Stage 4 — MountAPI existed and
// was correct, nothing ever called it.
//
// Ninguna ruta lleva CORS: el llamador es siempre el SERVIDOR de un
// proyecto consumidor (server-to-server), nunca el navegador del usuario
// directamente — el client_secret no puede vivir en un bundle WASM/JS
// (ver ARCHITECTURE.md §6.4/§7). Sin llamador cross-origin, no hay
// cabeceras CORS que emitir.
func Register(r router.Router, db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte) {
	modules := []router.APIModule{authMod}
	for _, m := range modules {
		m.MountAPI(r)
	}

	r.Get(PathHealth, Health()).Public()
	r.Get(PathHealthDB, HealthDB(db)).Public()
	r.Post(PathToken, Token(db, authMod, rbacSvc, secret)).Authenticated()
	// PathUsersResolve is Public() at the router's access-gate level
	// (no session cookie is involved — see its own doc), but it is NOT
	// reachable without a valid client_secret, checked inside the handler.
	r.Post(PathUsersResolve, ResolveUser(db, authMod)).Public()
}

// Health responde si este Worker esta vivo y sirviendo. A proposito NO toca la
// base: quien consulta esta ruta pregunta por el Worker, y meter una consulta a
// D1 aqui costaba ~160 ms de viaje a la region de la base ademas de convertir
// un hipo de red en una falsa alarma de caida. La alcanzabilidad de la base
// tiene su propia ruta, PathHealthDB.
func Health() router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		ctx.WriteStatus(200)
		_, _ = ctx.Write([]byte(bodyHealthOK))
	}
}

// HealthDB comprueba que la base responde. Es deliberadamente una ruta aparte:
// cuesta un viaje completo a la region donde vive D1, asi que se consulta
// cuando se quiere esa respuesta, no en cada latido de un monitor.
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

// TokenRequest es el cuerpo de POST /api/token. userID NUNCA viaja aquí —
// sale de ctx.UserID() (la sesión activa), nunca del body (ver
// ARCHITECTURE.md §6, "el userID sale de la sesión, nunca del cuerpo").
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

// TokenResponse es la respuesta de POST /api/token. Además del token de
// autorización, lleva el perfil básico del usuario de la sesión —
// Email/Name/Avatar, no Sub/Aud/Scope (eso vive en Token, ver
// ARCHITECTURE.md §6.2) — para que un consumidor que ya está haciendo esta
// llamada no necesite una segunda solo para mostrar "Hola, <nombre>": la
// respuesta ya requiere sesión activa + client_secret válido, así que no
// hay superficie nueva que exponer.
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

// Token emite un token de autorización project-scoped para el usuario de
// la sesión activa, si project_id/client_secret validan contra el
// proyecto registrado.
func Token(db *orm.DB, authMod *authority.Module, rbacSvc *rbac.Service, secret []byte) router.HandlerFunc {
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
		ok, err := config.VerifyProjectSecret(db, body.ProjectID, body.ClientSecret)
		if err != nil {
			ctx.WriteStatus(500)
			return
		}
		if !ok {
			ctx.WriteStatus(403)
			return
		}
		token, err := config.IssueAuthToken(rbacSvc, secret, body.ProjectID, userID)
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

// ResolveUserRequest es el cuerpo de POST /api/users/resolve. No hay
// sesión de por medio — a diferencia de /api/token, el email de destino no
// tiene por qué haber iniciado sesión nunca (un admin de un consumidor
// registra un cliente nuevo por su email). client_secret es la única
// prueba de identidad: sin él, cualquiera podría crear usuarios en
// cualquier proyecto.
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

// ResolveUser busca un usuario por email dentro de la identidad global de
// iam (no hay tabla de usuarios por proyecto: la identidad es una sola,
// ver ARCHITECTURE.md §1) y lo crea si no existe. Devuelve su Sub — el
// mismo id que llevará el Sub de su token cuando inicie sesión, así que un
// consumidor puede asignarle recursos (ej. dueño de un sitio) ANTES de que
// esa persona haga login por primera vez.
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
		ok, err := config.VerifyProjectSecret(db, body.ProjectID, body.ClientSecret)
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

package routes

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/rbac"
	"github.com/tinywasm/router"
	"github.com/veltylabs/iam/api"
	"github.com/veltylabs/iam/config"
)

const PathHealth = api.PathHealth
const PathToken = api.PathToken
const BindingD1 = api.BindingD1

// corsRootDomain/corsSubdomainSuffix scope which Origin values may call
// /api/token with credentials — see ARCHITECTURE.md §7 (CORS with
// credentials cannot use a wildcard origin, so this reflects the exact
// Origin back only when it matches). Compared against the Origin header's
// HOST, never a raw substring of the header: "https://evilvelty.cl" must
// never match "velty.cl".
const (
	corsRootDomain      = "velty.cl"
	corsSubdomainSuffix = "." + corsRootDomain
)

// Register monta todas las rutas de iam en el router.
func Register(r router.Router, db *orm.DB, rbacSvc *rbac.Service, secret []byte) {
	r.Get(PathHealth, Health(db)).Public()
	r.Post(PathToken, TokenHandler(db, rbacSvc, secret)).Authenticated()
}

// TokenHandler es el handler que Register monta en PathToken: Token
// envuelto en withCORS. Exportado para que un test pueda dirigir una
// request contra él sin levantar un router real (ver tests/cors_test.go).
func TokenHandler(db *orm.DB, rbacSvc *rbac.Service, secret []byte) router.HandlerFunc {
	return withCORS(Token(db, rbacSvc, secret))
}

// Health devuelve el manejador HTTP para la verificación de estado.
func Health(db *orm.DB) router.HandlerFunc {
	return func(ctx router.Context) {
		ctx.SetHeader("Content-Type", "application/json")

		if db == nil || db.RawConn() == nil {
			ctx.WriteStatus(503)
			_, _ = ctx.Write([]byte("{\"ok\":false}"))
			return
		}
		if err := db.RawConn().Exec("SELECT 1"); err != nil {
			ctx.WriteStatus(503)
			_, _ = ctx.Write([]byte("{\"ok\":false}"))
			return
		}
		ctx.WriteStatus(200)
		_, _ = ctx.Write([]byte("{\"ok\":true}"))
	}
}

// withCORS refleja Access-Control-Allow-Origin cuando el Origin de la
// request termina en corsAllowedDomain (o lo es exactamente) — nunca "*":
// CORS con credentials no lo admite (regla del propio estándar, ver
// ARCHITECTURE.md §7). Un Origin fuera de ese dominio no recibe la
// cabecera y el navegador bloquea la respuesta.
func withCORS(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx router.Context) {
		origin := ctx.GetHeader("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			ctx.SetHeader("Access-Control-Allow-Origin", origin)
			ctx.SetHeader("Access-Control-Allow-Credentials", "true")
		}
		next(ctx)
	}
}

// isAllowedOrigin extracts the HOST from an Origin header value
// ("<scheme>://<host>[:port]", never a path) and compares it against the
// allowed domain — never a raw substring match of the whole header.
func isAllowedOrigin(origin string) bool {
	idx := fmt.Index(origin, "://")
	if idx < 0 {
		return false
	}
	host := origin[idx+3:]
	return host == corsRootDomain || fmt.HasSuffix(host, corsSubdomainSuffix)
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

// TokenResponse es la respuesta de POST /api/token.
type TokenResponse struct {
	Token string
}

func (t *TokenResponse) IsNil() bool { return t == nil }
func (t *TokenResponse) EncodeFields(w model.FieldWriter) {
	w.String("token", t.Token)
}
func (t *TokenResponse) DecodeFields(r model.FieldReader) {
	t.Token, _ = r.String("token")
}

// Token emite un token de autorización project-scoped para el usuario de
// la sesión activa, si project_id/client_secret validan contra el
// proyecto registrado.
func Token(db *orm.DB, rbacSvc *rbac.Service, secret []byte) router.HandlerFunc {
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
		ctx.WriteStatus(200)
		_ = ctx.Encode(&TokenResponse{Token: token})
	}
}

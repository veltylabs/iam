package routes

import (
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

// Register monta todas las rutas de iam en el router.
//
// PathToken no lleva CORS: el llamador es siempre el SERVIDOR de un
// proyecto consumidor (server-to-server), nunca el navegador del usuario
// directamente — el client_secret no puede vivir en un bundle WASM/JS
// (ver ARCHITECTURE.md §6.4/§7). Sin llamador cross-origin, no hay
// cabeceras CORS que emitir.
func Register(r router.Router, db *orm.DB, rbacSvc *rbac.Service, secret []byte) {
	r.Get(PathHealth, Health(db)).Public()
	r.Post(PathToken, Token(db, rbacSvc, secret)).Authenticated()
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

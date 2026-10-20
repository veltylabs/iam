package client

import (
	"webtyp.com/base64"
	"webtyp.com/env"
	"webtyp.com/fetch"
	"webtyp.com/fmt"
	"webtyp.com/json"
	tinyjwt "webtyp.com/jwt"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/time"
)

// Nombres convencionales de las variables de entorno. Están aquí para que
// todos los consumidores usen las mismas y nadie invente un tercer nombre.
const (
	EnvBaseURL          = "IAM_BASE_URL"
	EnvClientPrivateKey = "IAM_CLIENT_PRIVATE_KEY"
)

// Config es lo que un proyecto necesita para hablarle a iam.
type Config struct {
	BaseURL    string // https://iam.velty.cl, o el servidor de desarrollo
	ProjectID  string // identifica al proyecto ante iam
	PrivateKey []byte // base64url SEC1 P-256 private key; NUNCA llega al navegador
}

// Consumer es un proyecto que consume iam. Autentica peticiones contra iam y
// recuerda el scope que iam devolvió, para que el autorizador del proyecto lo
// lea sin un segundo viaje.
//
// Un proyecto crea UNO al arrancar y lo comparte entre su servidor local y su
// Worker: son el mismo objeto con la misma configuración, y ahí está la razón
// de que exista.
type Consumer struct {
	cfg   Config
	cache *scopeCache
}

const (
	ErrMsgMissingBaseURL    = "iam client: Config.BaseURL is required"
	ErrMsgMissingProjectID  = "iam client: Config.ProjectID is required"
	ErrMsgMissingPrivateKey = "iam client: Config.PrivateKey is required"
)

const (
	ErrMsgAssignRoleUserRequired = "iam client: AssignRole: userID is required"
	ErrMsgAssignRoleCodeRequired = "iam client: AssignRole: roleCode is required"
	ErrMsgAssignRoleUnknownCode  = "iam client: AssignRole: role code not found"
)

// New valida la configuración y falla rápido si falta algo.
func New(cfg Config) (*Consumer, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Err(ErrMsgMissingBaseURL)
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Err(ErrMsgMissingProjectID)
	}
	if len(cfg.PrivateKey) == 0 {
		return nil, fmt.Err(ErrMsgMissingPrivateKey)
	}
	return &Consumer{cfg: cfg, cache: &scopeCache{}}, nil
}

// ConfigFromEnv arma una Config leyendo EnvBaseURL y EnvClientPrivateKey.
// projectID es del proyecto, no del entorno: es una constante suya, no algo
// que se despliega distinto por ambiente.
func ConfigFromEnv(projectID string) Config {
	pkB64 := env.Get(EnvClientPrivateKey)
	pk, _ := base64.URLDecode(pkB64)
	return Config{
		BaseURL:    env.Get(EnvBaseURL),
		ProjectID:  projectID,
		PrivateKey: pk,
	}
}

func (c *Consumer) post(path string, body []byte) (*fetch.Response, error) {
	method := "POST"
	fullURL := c.cfg.BaseURL + path
	assertion, err := tinyjwt.NewClientAssertion(c.cfg.ProjectID, c.cfg.BaseURL, method, fullURL)
	if err != nil {
		return nil, err
	}
	signed, err := tinyjwt.SignClientAssertion(c.cfg.PrivateKey, assertion)
	if err != nil {
		return nil, err
	}
	req := fetch.Post(fullURL).
		ContentTypeJSON().
		Header("Authorization", "Bearer "+signed)
	if len(body) > 0 {
		req.Body(body)
	}
	return doSync(req)
}

// Authn identifica al llamante. Reemplaza a authority.Authenticate() en un
// proyecto que delega su identidad en iam.
//
// Una cookie SSO ausente o rechazada es lo NORMAL (sesión vencida, nadie ha
// entrado todavía): deja al llamante anónimo y sigue. Nunca es un 500 ni un
// error que se le muestre a la petición.
func (c *Consumer) Authn() router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx router.Context) {
			cookie, ok := ctx.Cookie(SSOCookieName)
			if !ok {
				next(ctx)
				return
			}
			id, err := c.fetchAuthzToken(cookie.Value)
			if err != nil {
				next(ctx)
				return
			}
			ctx.SetUserID(id.Claims.Sub)
			SetIdentity(ctx, id)
			c.cache.Set(id.Claims.Sub, id.Claims.Scope, time.Unix(id.Claims.Exp))
			next(ctx)
		}
	}
}

// Scope devuelve los códigos de rol vigentes que iam entregó para userID.
// ok es false cuando no hay entrada vigente — y eso DENIEGA: la ausencia de
// una respuesta no es un permiso.
func (c *Consumer) Scope(userID string) ([]string, bool) {
	return c.cache.Scope(userID)
}

// ResolveUser resuelve un usuario por email ante iam.
func (c *Consumer) ResolveUser(email, name string) (string, error) {
	return c.resolveUser(email, name)
}

type assignRoleRequest struct {
	UserID   string
	RoleCode string
}

func (r *assignRoleRequest) IsNil() bool { return r == nil }
func (r *assignRoleRequest) EncodeFields(w model.FieldWriter) {
	w.String("user_id", r.UserID)
	w.String("role_code", r.RoleCode)
}
func (r *assignRoleRequest) DecodeFields(fr model.FieldReader) {
	r.UserID, _ = fr.String("user_id")
	r.RoleCode, _ = fr.String("role_code")
}

// AssignRole concede roleCode al usuario en ESTE proyecto. Idempotente: si ya
// lo tiene, no es un error. Si el roleCode no existe en el proyecto devuelve
// un error (ErrMsgAssignRoleUnknownCode).
func (c *Consumer) AssignRole(userID string, roleCode model.RoleCode) error {
	if userID == "" {
		return fmt.Err(ErrMsgAssignRoleUserRequired)
	}
	if string(roleCode) == "" {
		return fmt.Err(ErrMsgAssignRoleCodeRequired)
	}
	var body []byte
	if err := json.Encode(&assignRoleRequest{
		UserID:   userID,
		RoleCode: string(roleCode),
	}, &body); err != nil {
		return err
	}
	resp, err := c.post("/api/roles/assign", body)
	if err != nil {
		return err
	}
	if resp.Status == 404 {
		return fmt.Err(ErrMsgAssignRoleUnknownCode)
	}
	if resp.Status != 200 {
		return fmt.Errf("iam: POST /api/roles/assign returned status %d", resp.Status)
	}
	return nil
}

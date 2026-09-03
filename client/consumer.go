package client

import (
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/json"
	"github.com/tinywasm/fetch"
	"github.com/tinywasm/model"
	"github.com/tinywasm/router"
)

// Nombres convencionales de las variables de entorno. Están aquí para que
// todos los consumidores usen las mismas y nadie invente un tercer nombre.
const (
	EnvBaseURL      = "IAM_BASE_URL"
	EnvClientSecret = "IAM_CLIENT_SECRET"
)

// Config es lo que un proyecto necesita para hablarle a iam.
type Config struct {
	BaseURL      string // https://iam.velty.cl, o el servidor de desarrollo
	ProjectID    string // identifica al proyecto ante iam
	ClientSecret string // NUNCA llega al navegador
}

// Consumer es un proyecto que consume iam. Autentica peticiones contra iam y
// recuerda el scope que iam devolvió, para que el autorizador del proyecto lo
// lea sin un segundo viaje.
//
// Un proyecto crea UNO al arrancar y lo comparte entre su servidor local y su
// Worker: son el mismo objeto con la misma configuración, y ahí está la razón
// de que exista — antes cada punto de entrada armaba el middleware por su
// cuenta, con cuatro argumentos que había que mantener sincronizados a mano.
type Consumer struct {
	cfg   Config
	cache *scopeCache
}

const (
	ErrMsgMissingBaseURL      = "iam client: Config.BaseURL is required"
	ErrMsgMissingProjectID    = "iam client: Config.ProjectID is required"
	ErrMsgMissingClientSecret = "iam client: Config.ClientSecret is required"
)

const (
	ErrMsgAssignRoleUserRequired = "iam client: AssignRole: userID is required"
	ErrMsgAssignRoleCodeRequired = "iam client: AssignRole: roleCode is required"
	ErrMsgAssignRoleUnknownCode  = "iam client: AssignRole: role code not found"
)

// New valida la configuración y falla rápido si falta algo. Arrancar sin poder
// hablarle a iam es peor que no arrancar: cada ruta protegida respondería 403
// para siempre y nadie sabría por qué.
func New(cfg Config) (*Consumer, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Err(ErrMsgMissingBaseURL)
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Err(ErrMsgMissingProjectID)
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Err(ErrMsgMissingClientSecret)
	}
	return &Consumer{cfg: cfg, cache: &scopeCache{}}, nil
}

// ConfigFromEnv arma una Config leyendo EnvBaseURL y EnvClientSecret.
// projectID es del proyecto, no del entorno: es una constante suya, no algo
// que se despliega distinto por ambiente.
func ConfigFromEnv(projectID string) Config {
	return Config{
		BaseURL:      env.Get(EnvBaseURL),
		ProjectID:    projectID,
		ClientSecret: env.Get(EnvClientSecret),
	}
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
			id, err := FetchAuthzToken(c.cfg.BaseURL, c.cfg.ProjectID, c.cfg.ClientSecret, cookie.Value)
			if err != nil {
				next(ctx)
				return
			}
			ctx.SetUserID(id.Claims.Sub)
			SetIdentity(ctx, id)
			c.cache.Set(id.Claims.Sub, id.Claims.Scope, id.Claims.Exp)
			next(ctx)
		}
	}
}

// Scope devuelve los códigos de rol vigentes que iam entregó para userID.
// ok es false cuando no hay entrada vigente — y eso DENIEGA: la ausencia de
// una respuesta no es un permiso.
//
// Devuelve códigos, no permisos. Qué autoriza cada código es política del
// proyecto, no de iam (ver docs/ARCHITECTURE.md §3.4).
func (c *Consumer) Scope(userID string) ([]string, bool) {
	return c.cache.Scope(userID)
}

type assignRoleRequest struct {
	ProjectID    string
	ClientSecret string
	UserID       string
	RoleCode     string
}

func (r *assignRoleRequest) IsNil() bool { return r == nil }
func (r *assignRoleRequest) EncodeFields(w model.FieldWriter) {
	w.String("project_id", r.ProjectID)
	w.String("client_secret", r.ClientSecret)
	w.String("user_id", r.UserID)
	w.String("role_code", r.RoleCode)
}
func (r *assignRoleRequest) DecodeFields(fr model.FieldReader) {}

type assignRoleResponse struct{}

func (r *assignRoleResponse) IsNil() bool                      { return r == nil }
func (r *assignRoleResponse) EncodeFields(w model.FieldWriter) {}
func (r *assignRoleResponse) DecodeFields(fr model.FieldReader) {}

// AssignRole concede roleCode al usuario en ESTE proyecto. Idempotente: si ya
// lo tiene, no es un error. Si el roleCode no existe en el proyecto devuelve
// un error (ErrMsgAssignRoleUnknownCode): un consumidor no define roles, los
// usa — definirlos es del panel.
func (c *Consumer) AssignRole(userID string, roleCode model.RoleCode) error {
	if userID == "" {
		return fmt.Err(ErrMsgAssignRoleUserRequired)
	}
	if string(roleCode) == "" {
		return fmt.Err(ErrMsgAssignRoleCodeRequired)
	}
	var body []byte
	if err := json.Encode(&assignRoleRequest{
		ProjectID:    c.cfg.ProjectID,
		ClientSecret: c.cfg.ClientSecret,
		UserID:       userID,
		RoleCode:     string(roleCode),
	}, &body); err != nil {
		return err
	}
	resp, err := doSync(fetch.Post(c.cfg.BaseURL + "/api/roles/assign").
		ContentTypeJSON().
		Body(body))
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

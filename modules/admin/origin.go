package admin

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/router"
	"github.com/veltylabs/iam/config"
)

const (
	HeaderOrigin       = "Origin"
	HeaderSecFetchSite = "Sec-Fetch-Site"

	// secFetchSameOrigin es el único valor aceptable: "same-site" NO alcanza,
	// y ésa es exactamente la razón por la que este archivo existe — la
	// cookie SSO es Domain=.velty.cl, así que misitio.velty.cl es same-site
	// respecto de iam.velty.cl y podría disparar estas rutas con la sesión
	// del administrador adjunta.
	secFetchSameOrigin = "same-origin"
)

// auditDetailMax acota lo que se guarda de la cabecera Origin recibida: es
// entrada del atacante y no puede inflar una fila de la base sin techo.
const auditDetailMax = 200

// RequireSameOrigin envuelve un handler de mutación: 403 si la petición no
// vino del propio origen de iam.
//
// Acepta si Sec-Fetch-Site es "same-origin" (lo mandan todos los navegadores
// vigentes y no es falsificable desde JavaScript), o si Origin coincide
// exactamente con expectedOrigin. Una petición SIN ninguna de las dos
// cabeceras se RECHAZA: el panel siempre las manda, y lo único que llega sin
// ellas es un cliente que no es el panel.
func RequireSameOrigin(db *orm.DB, ids model.IDGenerator, expectedOrigin string, h func(ctx router.Context, adminEmail string)) func(ctx router.Context, adminEmail string) {
	return func(ctx router.Context, adminEmail string) {
		fetchSite := ctx.GetHeader(HeaderSecFetchSite)
		origin := ctx.GetHeader(HeaderOrigin)
		if fetchSite == secFetchSameOrigin || (origin != "" && origin == expectedOrigin) {
			h(ctx, adminEmail)
			return
		}
		detail := origin
		if len(detail) > auditDetailMax {
			detail = detail[:auditDetailMax]
		}
		if err := config.RecordAudit(db, ids, adminEmail, config.AuditOriginDenied, adminEmail, detail); err != nil {
			fmt.Println("audit:", err)
		}
		ctx.WriteStatus(403)
	}
}

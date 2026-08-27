package client

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/router"
	tinyjwt "github.com/tinywasm/jwt"
)

// La Identity viaja como ocho claves escalares y no como un blob porque
// router.Context tiene un almacén solo-strings, y Scope (el único campo de
// slice) va y vuelve por un join/split con coma, seguro porque un código de
// rol nunca contiene una coma.

const (
	ctxKeySub    = "iam_identity_sub"
	ctxKeyExp    = "iam_identity_exp"
	ctxKeyIat    = "iam_identity_iat"
	ctxKeyAud    = "iam_identity_aud"
	ctxKeyScope  = "iam_identity_scope"
	ctxKeyEmail  = "iam_identity_email"
	ctxKeyName   = "iam_identity_name"
	ctxKeyAvatar = "iam_identity_avatar"

	scopeSep = ","
)

// SetIdentity guarda en el contexto lo que iam resolvió para esta petición.
func SetIdentity(ctx router.Context, id Identity) {
	ctx.SetValue(ctxKeySub, id.Claims.Sub)
	ctx.SetValue(ctxKeyExp, fmt.Convert(id.Claims.Exp).String())
	ctx.SetValue(ctxKeyIat, fmt.Convert(id.Claims.Iat).String())
	ctx.SetValue(ctxKeyAud, id.Claims.Aud)
	ctx.SetValue(ctxKeyScope, fmt.Convert(id.Claims.Scope).Join(scopeSep).String())
	ctx.SetValue(ctxKeyEmail, id.Email)
	ctx.SetValue(ctxKeyName, id.Name)
	ctx.SetValue(ctxKeyAvatar, id.Avatar)
}

// FromContext lee lo que SetIdentity guardó. ok es false cuando nunca se
// llamó a SetIdentity en esta petición.
func FromContext(ctx router.Context) (Identity, bool) {
	sub := ctx.Value(ctxKeySub)
	if sub == "" {
		return Identity{}, false
	}

	exp, _ := fmt.Convert(ctx.Value(ctxKeyExp)).Int64()
	iat, _ := fmt.Convert(ctx.Value(ctxKeyIat)).Int64()
	var scope []string
	if s := ctx.Value(ctxKeyScope); s != "" {
		scope = fmt.Split(s, scopeSep)
	}

	return Identity{
		Claims: tinyjwt.Claims{
			Sub:   sub,
			Exp:   exp,
			Iat:   iat,
			Aud:   ctx.Value(ctxKeyAud),
			Scope: scope,
		},
		Email:  ctx.Value(ctxKeyEmail),
		Name:   ctx.Value(ctxKeyName),
		Avatar: ctx.Value(ctxKeyAvatar),
	}, true
}

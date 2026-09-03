package routes

import (
	"github.com/tinywasm/router"
)

// Cabeceras de seguridad emitidas en TODAS las respuestas de iam. Los valores
// son constantes y no configurables: un servicio de identidad no tiene un
// modo "menos seguro" que valga la pena poder activar.
const (
	HeaderCSP            = "Content-Security-Policy"
	HeaderFrameOptions   = "X-Frame-Options"
	HeaderContentType    = "X-Content-Type-Options"
	HeaderReferrerPolicy = "Referrer-Policy"
	HeaderHSTS           = "Strict-Transport-Security"
	HeaderPermissions    = "Permissions-Policy"
)

const (
	// cspValue: el panel es Go compilado a WASM servido desde el mismo
	// origen. 'wasm-unsafe-eval' es lo que exige instanciar un módulo WASM;
	// NO es 'unsafe-eval' y no habilita eval() de JavaScript.
	// frame-ancestors 'none' es la versión moderna de X-Frame-Options y la
	// que realmente aplican los navegadores actuales.
	// img-src incluye https://lh3.googleusercontent.com porque los avatares
	// de Google (OAuthUserInfo.Avatar) vienen de ahí.
	cspValue = "default-src 'self'; " +
		"script-src 'self' 'wasm-unsafe-eval'; " +
		"style-src 'self'; " +
		"img-src 'self' data: https://lh3.googleusercontent.com; " +
		"connect-src 'self'; " +
		"font-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'"

	frameOptionsValue   = "DENY"
	contentTypeValue    = "nosniff"
	referrerPolicyValue = "strict-origin-when-cross-origin"
	// hstsValue: dos años, con subdominios. iam sólo se sirve por HTTPS
	// detrás de Cloudflare; no hay escenario en que una respuesta suya deba
	// aceptarse por HTTP.
	hstsValue        = "max-age=63072000; includeSubDomains"
	permissionsValue = "camera=(), microphone=(), geolocation=(), payment=()"
)

// SecurityHeaders es el middleware que las emite. Se instala con r.Use() en
// Register, antes de cualquier ruta.
func SecurityHeaders() router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx router.Context) {
			ctx.SetHeader(HeaderCSP, cspValue)
			ctx.SetHeader(HeaderFrameOptions, frameOptionsValue)
			ctx.SetHeader(HeaderContentType, contentTypeValue)
			ctx.SetHeader(HeaderReferrerPolicy, referrerPolicyValue)
			ctx.SetHeader(HeaderHSTS, hstsValue)
			ctx.SetHeader(HeaderPermissions, permissionsValue)
			next(ctx)
		}
	}
}

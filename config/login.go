package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/oauth2/provider/google"
	"github.com/tinywasm/html"
	"github.com/tinywasm/layout/login"
)

// PathLogo apunta al isotipo de Velty. Placeholder vacío: no hay todavía un
// mecanismo para que iam sirva sus propios assets estáticos (llega con la
// Etapa 3, API HTTP).
const PathLogo = ""

// LoginScreen es la pantalla compartida de todos los proyectos Velty: sin
// marca de ningún proyecto en particular, porque el mismo login sirve a
// todos bajo SSO (Etapa 4).
func LoginScreen() *login.Login {
	return &login.Login{
		Title:    "Velty",
		Subtitle: "Iniciar sesión",
		LogoMark: PathLogo,
		Form:     html.A(auth.PathOAuthStart(google.ProviderName)).Attr("class", "btn btn-primary").Text("Iniciar sesión con Google"),
	}
}

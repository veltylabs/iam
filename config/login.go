package config

import (
	"github.com/tinywasm/auth"
	"github.com/tinywasm/auth/oauth2/provider/google"
	"github.com/tinywasm/components/actionbutton"
	"github.com/tinywasm/layout/login"
)

// PathLogo apunta al isotipo de Velty. Placeholder vacío: no hay todavía un
// mecanismo para que iam sirva sus propios assets estáticos (llega con la
// Etapa 3, API HTTP).
const PathLogo = ""

// LoginScreen es la pantalla compartida de todos los proyectos Velty: sin
// marca de ningún proyecto en particular, porque el mismo login sirve a
// todos bajo SSO (Etapa 4). El botón es actionbutton.ActionButton con Href
// (no OnClick): un <a>, no un <button>, estilizado por
// tinywasm/widget/style igual que cualquier otro botón del ecosistema —
// nunca una clase CSS suelta (ver AGENTS.md Restricción #2).
func LoginScreen() *login.Login {
	return &login.Login{
		Title:    "Velty",
		Subtitle: "Iniciar sesión",
		LogoMark: PathLogo,
		Form: &actionbutton.ActionButton{
			Text:    "Iniciar sesión con Google",
			Variant: "primary",
			Href:    auth.PathOAuthStart(google.ProviderName),
		},
	}
}

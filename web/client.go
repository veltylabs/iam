//go:build wasm

package main

import (
	_ "github.com/tinywasm/components/fieldset"
	"github.com/tinywasm/dom"
	"github.com/tinywasm/fetch"
	"github.com/tinywasm/json"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/panel"
)

func main() {
	fetch.Get(config.PathAdminMe).Send(func(resp *fetch.Response, err error) {
		if err != nil {
			panel.ShowStatus("No se pudo contactar al servidor.")
			return
		}
		switch resp.Status {
		case 401:
			return // el HTML servido ya muestra el enlace de login
		case 403:
			panel.ShowStatus("Tu cuenta no tiene acceso al panel de administración.")
		case 200:
			if ref, ok := dom.Get("login"); ok {
				ref.SetAttr("style", "display: none;")
			}
			var me config.AdminMeResponse
			if err := json.Decode(resp.Body(), &me); err != nil {
				panel.ShowStatus("Respuesta ilegible del servidor.")
				return
			}
			panel.Boot(me)
		default:
			panel.ShowStatus("El servidor respondió un estado inesperado.")
		}
	})
	select {}
}



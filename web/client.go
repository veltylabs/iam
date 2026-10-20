//go:build wasm

package main

import (
	"webtyp.com/dom"
	"webtyp.com/fetch"
	"webtyp.com/json"
	"github.com/veltylabs/iam/config"
	"github.com/veltylabs/iam/modules/panel"
)

func main() {
	panel.SetOnMFARequired(func() {
		panel.ShowStepUp(loadMe)
	})
	loadMe()
	select {}
}

func loadMe() {
	fetch.Get(config.PathAdminMe).Send(func(resp *fetch.Response, err error) {
		if err != nil {
			panel.ShowStatus("No se pudo contactar al servidor.")
			return
		}
		switch resp.Status {
		case 401, 403:
			if ref, ok := dom.Get("login"); ok {
				ref.SetAttr("style", "")
			}
			return
		case 200:
			if ref, ok := dom.Get("login"); ok {
				ref.SetAttr("style", "display: none;")
			}
			var me config.AdminMeResponse
			if err := json.Decode(resp.Body(), &me); err != nil {
				panel.ShowStatus("Respuesta ilegible del servidor.")
				return
			}
			if !me.IsAdmin {
				panel.ShowStatus("Tu cuenta no tiene acceso al panel de administración.")
				return
			}
			if !me.HasPasskey {
				panel.ShowEnrollment(loadMe)
				return
			}
			if !me.MultiFactor {
				panel.ShowStepUp(loadMe)
				return
			}
			panel.Boot(me)
		default:
			panel.ShowStatus("El servidor respondió un estado inesperado.")
		}
	})
}

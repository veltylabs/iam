//go:build wasm

package panel

import (
	"github.com/tinywasm/dom"
	"github.com/tinywasm/fetch"
	"github.com/tinywasm/html"
	"github.com/tinywasm/json"
	"github.com/veltylabs/iam/config"
)

func wireAudit() {
	loadAuditList()
}

func loadAuditList() {
	fetch.Get(config.PathAdminAudit).Send(func(resp *fetch.Response, err error) {
		if err != nil || resp.Status != 200 {
			setContainerText(IDAuditList, "Error al cargar la auditoría.")
			return
		}
		var auditResp config.AdminAuditResponse
		if err := json.Decode(resp.Body(), &auditResp); err != nil {
			setContainerText(IDAuditList, "Error decodificando la auditoría.")
			return
		}

		table := html.Table().Class("iam-table").Child(
			html.Thead().Child(
				html.Tr().Child(
					html.Th().Text("Fecha"),
					html.Th().Text("Actor"),
					html.Th().Text("Acción"),
					html.Th().Text("Objetivo"),
					html.Th().Text("Detalle"),
				),
			),
		)
		tbody := html.Tbody()
		for _, e := range auditResp.Entries {
			row := html.Tr().Child(
				html.Td().Text(formatEpoch(e.CreatedAt)),
				html.Td().Text(e.ActorEmail),
				html.Td().Text(e.Action),
				html.Td().Text(e.Target),
				html.Td().Text(e.Detail),
			)
			tbody.Child(row)
		}
		table.Child(tbody)
		_ = dom.Render(IDAuditList, table)
	})
}

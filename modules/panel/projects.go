//go:build wasm

package panel

import (
	"github.com/tinywasm/dom"
	"github.com/tinywasm/fetch"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/form"
	"github.com/tinywasm/html"
	"github.com/tinywasm/json"
	"github.com/tinywasm/model"
	"github.com/tinywasm/time"
	"github.com/veltylabs/iam/config"
)

func wireProjects() {
	loadProjectsList()

	idGen, _ := config.NewIDs()
	f, err := form.New(IDProjectsNew, &config.AdminCreateProjectRequest{}, idGen)
	if err == nil && f != nil {
		f.SubmitLabel("Crear proyecto")
		f.OnSubmit(func(data model.Fielder, done func(error)) {
			req, ok := data.(*config.AdminCreateProjectRequest)
			if !ok || fmt.TrimSpace(req.Name) == "" {
				done(fmt.Err("Nombre inválido"))
				return
			}
			var buf []byte
			if err := json.Encode(req, &buf); err != nil {
				done(err)
				return
			}
			fetch.Post(config.PathAdminProjects).ContentTypeJSON().Body(buf).Send(func(resp *fetch.Response, err error) {
				if err != nil || resp.Status != 201 {
					done(fmt.Err("Fallo al crear proyecto"))
					return
				}
				var secretResp config.AdminSecretResponse
				if err := json.Decode(resp.Body(), &secretResp); err == nil {
					renderSecret(secretResp.ClientSecret)
				}
				done(nil)
				loadProjectsList()
			})
		})
		_ = dom.Render(IDProjectsNew, f)
	}
}

func renderSecret(secret string) {
	el := html.Div().Class("iam-secret-box").Child(
		html.P().Text("Guardá este client_secret. Se muestra una sola vez. Copialo ahora:"),
		html.Code().Text(secret),
	)
	_ = dom.Render(IDProjectsSecret, el)
}

func setContainerText(id, text string) {
	_ = dom.Render(id, html.Div().Text(text))
}

func loadProjectsList() {
	fetch.Get(config.PathAdminProjects).Send(func(resp *fetch.Response, err error) {
		if err != nil || resp.Status != 200 {
			setContainerText(IDProjectsList, "Error al cargar la lista de proyectos.")
			return
		}
		var projResp config.AdminProjectsResponse
		if err := json.Decode(resp.Body(), &projResp); err != nil {
			setContainerText(IDProjectsList, "Error decodificando la lista.")
			return
		}

		table := html.Table().Class("iam-table").Child(
			html.Thead().Child(
				html.Tr().Child(
					html.Th().Text("ID"),
					html.Th().Text("Nombre"),
					html.Th().Text("Estado"),
					html.Th().Text("Creado"),
					html.Th().Text("Acciones"),
				),
			),
		)
		tbody := html.Tbody()
		for _, p := range projResp.Projects {
			pID := p.Id
			pActive := p.Active

			statusText := "Inactivo"
			if pActive {
				statusText = "Activo"
			}

			btnRotate := html.Button().Text("Regenerar secreto")
			btnRotate.On("click", func(event dom.Event) {
				req := &config.AdminProjectIDRequest{ProjectId: pID}
				var buf []byte
				_ = json.Encode(req, &buf)
				fetch.Post(config.PathAdminProjectRotate).ContentTypeJSON().Body(buf).Send(func(r *fetch.Response, err error) {
					if err == nil && r.Status == 200 {
						var secretResp config.AdminSecretResponse
						if err := json.Decode(r.Body(), &secretResp); err == nil {
							renderSecret(secretResp.ClientSecret)
						}
					}
				})
			})

			toggleText := "Activar"
			if pActive {
				toggleText = "Desactivar"
			}
			btnToggle := html.Button().Text(toggleText)
			btnToggle.On("click", func(event dom.Event) {
				req := &config.AdminSetActiveRequest{ProjectId: pID, Active: !pActive}
				var buf []byte
				_ = json.Encode(req, &buf)
				fetch.Post(config.PathAdminProjectActive).ContentTypeJSON().Body(buf).Send(func(r *fetch.Response, err error) {
					if err == nil && r.Status == 200 {
						loadProjectsList()
					}
				})
			})

			row := html.Tr().Child(
				html.Td().Text(p.Id),
				html.Td().Text(p.Name),
				html.Td().Text(statusText),
				html.Td().Text(formatEpoch(p.CreatedAt)),
				html.Td().Child(btnRotate, btnToggle),
			)
			tbody.Child(row)
		}
		table.Child(tbody)
		_ = dom.Render(IDProjectsList, table)
	})
}

func formatEpoch(sec int64) string {
	if sec == 0 {
		return "-"
	}
	return time.FormatDateTime(sec * 1e9)
}

//go:build wasm

package panel

import (
	"github.com/tinywasm/dom"
	"github.com/tinywasm/html"
	"github.com/tinywasm/layout/rightpanel"
)

type moduleID string

func (m moduleID) ModelName() string { return string(m) }

func newMountedView(comp dom.Component, onMount func()) dom.Component {
	if onMount != nil {
		onMount()
	}
	return comp
}

func projectsView() dom.Component {
	rp := &rightpanel.RightPanel{
		Module: moduleID(ModuleProjects),
		Title:  "Proyectos",
		Article: html.Div().Child(
			html.Div().ID(IDProjectsSecret),
			html.Div().ID(IDProjectsList),
		),
		Aside: html.Div().ID(IDProjectsNew),
	}
	return newMountedView(rp, wireProjects)
}

func rolesView() dom.Component {
	rp := &rightpanel.RightPanel{
		Module: moduleID(ModuleRoles),
		Title:  "Roles",
		HeadControls: html.Div().Child(
			html.Label().Text("Proyecto: "),
			html.Input("select").ID(IDRolesProject),
		),
		Article: html.Div().ID(IDRolesList),
		Aside:   html.Div().ID(IDRolesNew),
	}
	return newMountedView(rp, wireRoles)
}

func usersView() dom.Component {
	rp := &rightpanel.RightPanel{
		Module:  moduleID(ModuleUsers),
		Title:   "Usuarios y Asignación de Roles",
		Article: html.Div().ID(IDUsersList),
		Aside:   html.Div().ID(IDUsersForm),
	}
	return newMountedView(rp, wireUsers)
}

func auditView() dom.Component {
	rp := &rightpanel.RightPanel{
		Module:  moduleID(ModuleAudit),
		Title:   "Registro de Auditoría",
		Article: html.Div().ID(IDAuditList),
	}
	return newMountedView(rp, wireAudit)
}

//go:build wasm

package panel

import (
	"github.com/tinywasm/dom"
	"github.com/tinywasm/layout/platformd"
	"github.com/veltylabs/iam/config"
)

type adminIdentity struct {
	name string
}

func (a adminIdentity) UserName() string     { return a.name }
func (a adminIdentity) UserAvatar() string   { return "" }
func (a adminIdentity) UserRoles() []string { return nil }

func Boot(me config.AdminMeResponse) {
	p := &platformd.Platform{
		AppName:   "iam · administración",
		DefaultID: ModuleProjects,
		User:      adminIdentity{name: me.Name},
		CanView:   func(id string) bool { return true },
	}
	p.Modules = []platformd.UIModule{
		platformd.NewUIModule(ModuleProjects, "Proyectos", IconProjects, projectsView()),
		platformd.NewUIModule(ModuleRoles, "Roles", IconRoles, rolesView()),
		platformd.NewUIModule(ModuleUsers, "Usuarios", IconUsers, usersView()),
		platformd.NewUIModule(ModuleAudit, "Auditoría", IconAudit, auditView()),
	}
	_ = dom.Append("body", p)
}

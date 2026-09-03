package config

import (
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
)

const EnvAdminEmails = "IAM_ADMIN_EMAILS"

// PanelAdminList parsea EnvAdminEmails (por coma, sin vacíos ni espacios). nil si no está.
func PanelAdminList() []string {
	val := env.Get(EnvAdminEmails)
	if val == "" {
		return nil
	}
	raw := fmt.Split(val, ",")
	var list []string
	for _, item := range raw {
		trimmed := fmt.TrimSpace(item)
		if trimmed != "" {
			list = append(list, trimmed)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

// IsPanelAdmin evalúa si el email exacto (case-sensitive) está en la lista de administradores del panel.
func IsPanelAdmin(email string, list []string) bool {
	if email == "" || len(list) == 0 {
		return false
	}
	for _, admin := range list {
		if admin == email {
			return true
		}
	}
	return false
}

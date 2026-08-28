package config

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/time"
)

const (
	AuditProjectCreate     = "project.create"
	AuditProjectRotate     = "project.rotate_secret"
	AuditProjectActivate   = "project.activate"
	AuditProjectDeactivate = "project.deactivate"
	AuditRoleCreate        = "role.create"
	AuditRoleDelete        = "role.delete"
	AuditRoleSetTTL        = "role.set_ttl"
	AuditUserAssign        = "user.assign_role"
	AuditUserRevoke        = "user.revoke_role"
)

// RecordAudit escribe una fila de auditoría. Si falla, loguea con fmt.Println y sigue.
func RecordAudit(db *orm.DB, ids model.IDGenerator, actorEmail, action, target, detail string) error {
	entry := &AuditEntry{
		Id:         ids.NewID(),
		ActorEmail: actorEmail,
		Action:     action,
		Target:     target,
		Detail:     detail,
		CreatedAt:  time.Now() / 1e9,
	}
	if err := db.Create(entry); err != nil {
		fmt.Println("audit error:", err)
		return err
	}
	return nil
}

// ListAudit devuelve las últimas `limit` filas, más recientes primero.
func ListAudit(db *orm.DB, limit int) (AuditEntryList, error) {
	qb := db.Query(&AuditEntry{}).OrderBy(AuditEntry_.CreatedAt).Desc()
	if limit > 0 {
		qb = qb.Limit(limit)
	}
	return ReadAllAuditEntry(qb)
}

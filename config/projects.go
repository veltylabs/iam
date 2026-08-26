package config

import (
	"github.com/tinywasm/crypto/bcrypt"
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/time"
)

// ErrProjectNotFound reports that a project id has no matching row.
var ErrProjectNotFound = fmt.Err("project", "not", "found")

// MigrateProjects reconciles the schema this service owns (Project).
//
// Deliberately not called from NewProductionBackend: schema reconciliation
// is deploy-time work. Doing it per process start cost ~10 D1 round trips
// on every isolate cold start (8.5–10.4 s measured). cmd/migrate calls this
// once, from CI.
func MigrateProjects(conn ddl.Execer, ddlCompiler ddl.Compiler) error {
	return ddl.New(conn, ddlCompiler).Sync(&Project{})
}

// CreateProject registra un proyecto nuevo y devuelve el client_secret EN
// CLARO una sola vez — no se puede recuperar después, solo regenerar.
func CreateProject(db *orm.DB, id, name, plainSecret string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainSecret), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.Create(&Project{
		Id: id, Name: name, ClientSecretHash: string(hash),
		CreatedAt: time.Now() / 1e9,
	})
}

// VerifyProjectSecret compara en tiempo constante vía bcrypt — nunca ==.
func VerifyProjectSecret(db *orm.DB, projectID, plainSecret string) (bool, error) {
	qb := db.Query(&Project{}).Where(Project_.Id).Eq(projectID)
	rows, err := ReadAllProject(qb)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(rows[0].ClientSecretHash), []byte(plainSecret)) == nil, nil
}

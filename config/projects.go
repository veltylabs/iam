package config

import (
	"webtyp.com/base64"
	"webtyp.com/crypto/asym"
	"webtyp.com/ddl"
	"webtyp.com/fmt"
	"webtyp.com/orm"
	"webtyp.com/storage"
	"webtyp.com/time"
)

// projectNotFound is the concrete type of ErrProjectNotFound. IsProjectNotFound
// recognises it with a type assertion: under TinyGo, == between two error
// values goes through runtime.interfaceEqual and pulls reflection into wasm.
type projectNotFound struct{}

func (projectNotFound) Error() string { return "project not found" }

// ErrProjectNotFound reports that a project id has no matching row. Detect it
// with IsProjectNotFound, never with ==.
var ErrProjectNotFound error = projectNotFound{}

// IsProjectNotFound reports whether err is ErrProjectNotFound.
func IsProjectNotFound(err error) bool {
	_, ok := err.(projectNotFound)
	return ok
}

type projectInactive struct{}

func (projectInactive) Error() string { return "project inactive or key not set" }

var ErrProjectInactive error = projectInactive{}

func IsProjectInactive(err error) bool {
	_, ok := err.(projectInactive)
	return ok
}

// MigrateSchema reconciles the schema this service owns (Project and AuditEntry).
//
// Deliberately not called from NewProductionBackend: schema reconciliation
// is deploy-time work. Doing it per process start cost ~10 D1 round trips
// on every isolate cold start (8.5–10.4 s measured). cmd/migrate calls this
// once, from CI.
func MigrateSchema(conn ddl.Execer, ddlCompiler ddl.Compiler) error {
	d := ddl.New(conn, ddlCompiler)
	if err := d.Sync(&Project{}); err != nil {
		return err
	}
	if err := d.Sync(&AuditEntry{}); err != nil {
		return err
	}
	return conn.Exec("UPDATE project SET active = 1 WHERE active IS NULL OR active = 0")
}

func validatePublicKey(publicKeyB64 string) ([]byte, error) {
	if publicKeyB64 == "" {
		return nil, fmt.Err("project key cannot be empty")
	}
	keyBytes, err := base64.URLDecode(publicKeyB64)
	if err != nil {
		return nil, err
	}
	if _, err := asym.VerifyCompact([]byte("probe"), make([]byte, 64), keyBytes); err != nil {
		return nil, err
	}
	return keyBytes, nil
}

// CreateProject registra un proyecto nuevo. Si publicKeyB64 no está vacío,
// valida que sea un SPKI P-256 codificado en base64url.
func CreateProject(db *orm.DB, id, name, publicKeyB64 string) error {
	if publicKeyB64 != "" {
		if _, err := validatePublicKey(publicKeyB64); err != nil {
			return err
		}
	}
	return db.Create(&Project{
		Id:              id,
		Name:            name,
		ClientPublicKey: publicKeyB64,
		CreatedAt:       time.NowUnix(),
		Active:          1,
	})
}

// SetProjectKey registers (or replaces) a project's public key. It refuses a
// key that is not base64url of a P-256 SPKI.
func SetProjectKey(db *orm.DB, projectID, publicKeyB64 string) error {
	if _, err := validatePublicKey(publicKeyB64); err != nil {
		return err
	}
	qb := db.Query(&Project{}).Where(Project_.Id).Eq(projectID)
	rows, err := ReadAllProject(qb)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrProjectNotFound
	}
	proj := rows[0]
	proj.ClientPublicKey = publicKeyB64
	return db.Update(proj, storage.Eq(Project_.Id, projectID))
}

// SetProjectActive activa o desactiva un proyecto. ErrProjectNotFound si no existe.
func SetProjectActive(db *orm.DB, projectID string, active bool) error {
	qb := db.Query(&Project{}).Where(Project_.Id).Eq(projectID)
	rows, err := ReadAllProject(qb)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrProjectNotFound
	}
	proj := rows[0]
	if active {
		proj.Active = 1
	} else {
		proj.Active = 0
	}
	return db.Update(proj, storage.Eq(Project_.Id, projectID))
}

// ListProjects devuelve todos los proyectos ordenados por created_at asc.
func ListProjects(db *orm.DB) (ProjectList, error) {
	qb := db.Query(&Project{}).OrderBy(Project_.CreatedAt).Asc()
	return ReadAllProject(qb)
}

// ProjectKeys implements clientassertion.ClientKeys for machine client assertions.
type ProjectKeys struct {
	DB *orm.DB
}

// ClientPublicKey implements clientassertion.ClientKeys: active project with a
// non-empty key → decoded SPKI; otherwise an error (IsProjectNotFound for
// unknown ids, a typed inactive error otherwise).
func (k ProjectKeys) ClientPublicKey(clientID string) ([]byte, error) {
	if k.DB == nil {
		return nil, ErrProjectNotFound
	}
	qb := k.DB.Query(&Project{}).Where(Project_.Id).Eq(clientID)
	rows, err := ReadAllProject(qb)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrProjectNotFound
	}
	proj := rows[0]
	if proj.Active == 0 || proj.ClientPublicKey == "" {
		return nil, ErrProjectInactive
	}
	keyBytes, err := base64.URLDecode(proj.ClientPublicKey)
	if err != nil {
		return nil, ErrProjectInactive
	}
	return keyBytes, nil
}

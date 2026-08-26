package config

import (
	"github.com/tinywasm/base64"
	"github.com/tinywasm/crypto/hmac"
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/env"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/time"
)

// ErrProjectNotFound reports that a project id has no matching row.
var ErrProjectNotFound = fmt.Err("project", "not", "found")

// projectSecretKeyLabel separa la clave que hashea client_secret de la que
// firma JWT. Reutilizar una sola clave para dos propositos hace que un fallo en
// uno comprometa el otro; derivar cuesta un HMAC y elimina el acoplamiento.
// El sufijo de version permite rotar el esquema sin ambiguedad.
const projectSecretKeyLabel = "iam:project-secret:v1"

func projectSecretKey() []byte {
	return hmac.HMACSHA256([]byte(env.Get(EnvJWTSecret)), []byte(projectSecretKeyLabel))
}

// hashProjectSecret deriva el hash almacenable de un client_secret. Es HMAC y
// no bcrypt a proposito: un client_secret es aleatorio y de alta entropia, asi
// que no hay nada que frenar con un factor de costo — solo CPU que quemar, y en
// un Worker eso se paga en cada peticion contra un limite de 10 ms.
func hashProjectSecret(plainSecret string) string {
	return base64.URLEncode(hmac.HMACSHA256(projectSecretKey(), []byte(plainSecret)))
}

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
	return db.Create(&Project{
		Id: id, Name: name, ClientSecretHash: hashProjectSecret(plainSecret),
		CreatedAt: time.Now() / 1e9,
	})
}

// VerifyProjectSecret compara en tiempo constante vía HMAC SHA256 — nunca ==.
func VerifyProjectSecret(db *orm.DB, projectID, plainSecret string) (bool, error) {
	qb := db.Query(&Project{}).Where(Project_.Id).Eq(projectID)
	rows, err := ReadAllProject(qb)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	stored, err := base64.URLDecode(rows[0].ClientSecretHash)
	if err != nil {
		return false, nil
	}
	return hmac.HMACEqual(stored, hmac.HMACSHA256(projectSecretKey(), []byte(plainSecret))), nil
}

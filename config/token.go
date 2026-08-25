package config

import (
	"github.com/tinywasm/jwt"
	"github.com/tinywasm/rbac"
)

const DefaultAuthTokenTTL = 30 * 60 // 30 minutos — ver ARCHITECTURE.md §6.3

// IssueAuthToken firma un token de autorizacion project-scoped para
// userID, usando el SessionTTL mas restrictivo entre sus roles en
// projectID (0 => DefaultAuthTokenTTL). secret es el mismo secreto HS256
// que usa el resto de la sesion de iam. Aud lleva projectID, Scope lleva
// los codigos de rol — vocabulario JWT estandar, ver ARCHITECTURE.md §6.2.
func IssueAuthToken(rbacSvc *rbac.Service, secret []byte, projectID, userID string) (string, error) {
	roles, err := rbacSvc.GetUserRoles(projectID, userID)
	if err != nil {
		return "", err
	}
	ttl := DefaultAuthTokenTTL
	roleCodes := make([]string, len(roles))
	for i, r := range roles {
		roleCodes[i] = r.Code
		if r.SessionTtl > 0 && (ttl == DefaultAuthTokenTTL || int(r.SessionTtl) < ttl) {
			ttl = int(r.SessionTtl)
		}
	}
	claims := jwt.NewScopedClaims(userID, projectID, roleCodes, ttl)
	return jwt.Sign(secret, claims)
}

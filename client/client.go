package client

import (
	"webtyp.com/fmt"
	"webtyp.com/json"
	tinyjwt "webtyp.com/jwt"
	"webtyp.com/model"
)

// SSOCookieName is the cookie iam sets after a successful login, shared
// across every *.velty.cl subdomain (Domain=".velty.cl", see
// ARCHITECTURE.md §7). A consumer reads it off its OWN incoming request
// (net/http-style cookie jar, or ctx.Cookie(SSOCookieName) in this
// ecosystem's router.Context) and passes its value to fetchAuthzToken.
const SSOCookieName = "iam_session"

// ErrNoSSOSession means either no SSO cookie was forwarded, or iam
// rejected it (expired/forged) — both collapse to "treat this caller as
// anonymous", never as an error worth alarming on: an expired or absent
// session is routine, not an attack (same principle as tinywasm/jwt's
// Outcome — see its docs).
var ErrNoSSOSession = fmt.Err("sso", "session-required")

type tokenRequest struct {
	SubjectToken string
}

func (t *tokenRequest) IsNil() bool { return t == nil }
func (t *tokenRequest) EncodeFields(w model.FieldWriter) {
	w.String("subject_token", t.SubjectToken)
}
func (t *tokenRequest) DecodeFields(r model.FieldReader) {
	t.SubjectToken, _ = r.String("subject_token")
}

type tokenResponse struct {
	Token  string
	Email  string
	Name   string
	Avatar string
}

func (t *tokenResponse) IsNil() bool                      { return t == nil }
func (t *tokenResponse) EncodeFields(w model.FieldWriter) {}
func (t *tokenResponse) DecodeFields(r model.FieldReader) {
	t.Token, _ = r.String("token")
	t.Email, _ = r.String("email")
	t.Name, _ = r.String("name")
	t.Avatar, _ = r.String("avatar")
}

// Identity is what fetchAuthzToken resolves for the caller's session: the
// authorization claims (Sub/Aud/Scope) plus the profile fields iam's
// /api/token response carries alongside them — a consumer showing "Hola,
// <Name>" needs no second call.
type Identity struct {
	Claims tinyjwt.Claims
	Email  string
	Name   string
	Avatar string
}

func (c *Consumer) fetchAuthzToken(ssoCookieValue string) (Identity, error) {
	if ssoCookieValue == "" {
		return Identity{}, ErrNoSSOSession
	}

	var body []byte
	if err := json.Encode(&tokenRequest{SubjectToken: ssoCookieValue}, &body); err != nil {
		return Identity{}, err
	}

	resp, err := c.post("/api/token", body)
	if err != nil {
		return Identity{}, err
	}
	if resp.Status == 401 || resp.Status == 403 {
		return Identity{}, ErrNoSSOSession
	}
	if resp.Status != 200 {
		return Identity{}, fmt.Errf("iam: POST /api/token returned status %d", resp.Status)
	}

	var tr tokenResponse
	if err := json.Decode(resp.Body(), &tr); err != nil {
		return Identity{}, err
	}
	claims, err := tinyjwt.DecodeUnverified(tr.Token)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Claims: claims, Email: tr.Email, Name: tr.Name, Avatar: tr.Avatar}, nil
}

type resolveUserRequest struct {
	Email string
	Name  string
}

func (r *resolveUserRequest) IsNil() bool { return r == nil }
func (r *resolveUserRequest) EncodeFields(w model.FieldWriter) {
	w.String("email", r.Email)
	w.String("name", r.Name)
}
func (r *resolveUserRequest) DecodeFields(fr model.FieldReader) {
	r.Email, _ = fr.String("email")
	r.Name, _ = fr.String("name")
}

type resolveUserResponse struct {
	Sub string
}

func (r *resolveUserResponse) IsNil() bool                      { return r == nil }
func (r *resolveUserResponse) EncodeFields(w model.FieldWriter) {}
func (r *resolveUserResponse) DecodeFields(fr model.FieldReader) {
	r.Sub, _ = fr.String("sub")
}

func (c *Consumer) resolveUser(email, name string) (string, error) {
	var body []byte
	if err := json.Encode(&resolveUserRequest{Email: email, Name: name}, &body); err != nil {
		return "", err
	}
	resp, err := c.post("/api/users/resolve", body)
	if err != nil {
		return "", err
	}
	if resp.Status != 200 {
		return "", fmt.Errf("iam: POST /api/users/resolve returned status %d", resp.Status)
	}
	var rr resolveUserResponse
	if err := json.Decode(resp.Body(), &rr); err != nil {
		return "", err
	}
	return rr.Sub, nil
}

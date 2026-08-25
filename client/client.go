// Package client is what a project consuming iam remotely imports — a
// thin server-to-server HTTP client for POST /api/token. It carries none
// of iam's own ORM/auth machinery: a consumer's binary (including its WASM
// edge build) never pulls in tinywasm/orm or tinywasm/rbac just to ask
// "who is this user and what may they do in MY project".
//
// This client is called from the CONSUMER'S OWN SERVER, never from the
// browser: client_secret must never reach a WASM/JS bundle a user can
// inspect (see ARCHITECTURE.md §6.4/§7). The consumer's server reads the
// SSO cookie off the incoming request and forwards its value here — it
// never verifies or decodes that cookie itself, only iam can.
package client

import (
	"github.com/tinywasm/fetch"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/json"
	tinyjwt "github.com/tinywasm/jwt"
	"github.com/tinywasm/model"
)

// SSOCookieName is the cookie iam sets after a successful login, shared
// across every *.velty.cl subdomain (Domain=".velty.cl", see
// ARCHITECTURE.md §7). A consumer reads it off its OWN incoming request
// (net/http-style cookie jar, or ctx.Cookie(SSOCookieName) in this
// ecosystem's router.Context) and passes its value to FetchAuthzToken.
const SSOCookieName = "iam_session"

// ErrNoSSOSession means either no SSO cookie was forwarded, or iam
// rejected it (expired/forged) — both collapse to "treat this caller as
// anonymous", never as an error worth alarming on: an expired or absent
// session is routine, not an attack (same principle as tinywasm/jwt's
// Outcome — see its docs).
var ErrNoSSOSession = fmt.Err("iam", "sso", "session-required")

type tokenRequest struct {
	ProjectID    string
	ClientSecret string
}

func (t *tokenRequest) IsNil() bool { return t == nil }
func (t *tokenRequest) EncodeFields(w model.FieldWriter) {
	w.String("project_id", t.ProjectID)
	w.String("client_secret", t.ClientSecret)
}
func (t *tokenRequest) DecodeFields(r model.FieldReader) {}

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

// Identity is what FetchAuthzToken resolves for the caller's session: the
// authorization claims (Sub/Aud/Scope) plus the profile fields iam's
// /api/token response carries alongside them — a consumer showing "Hola,
// <Name>" needs no second call.
type Identity struct {
	Claims tinyjwt.Claims
	Email  string
	Name   string
	Avatar string
}

// FetchAuthzToken calls POST <iamBaseURL>/api/token server-to-server,
// forwarding ssoCookieValue (the SSO cookie's value read off the caller's
// own incoming request) so iam can identify the user, and clientSecret so
// iam knows which project is asking. Returns the project-scoped
// authorization claims — Aud is projectID, Scope is the user's role codes
// in that project (see ARCHITECTURE.md §6.1/§6.2) — plus the user's basic
// profile.
//
// The claims are decoded WITHOUT verifying the HMAC signature
// (tinyjwt.DecodeUnverified, not Verify): this response came directly from
// iam over THIS SAME HTTPS call, not from an untrusted third party
// presenting a token later — there is nothing to gain from re-checking a
// signature whose secret this consumer does not, and must never, have
// (ARCHITECTURE.md §6.2: the HS256 secret is internal to iam). Do not
// change this to Verify — it would require sharing iam's secret, which is
// exactly the acoplamiento this design avoids.
func FetchAuthzToken(iamBaseURL, projectID, clientSecret, ssoCookieValue string) (Identity, error) {
	if ssoCookieValue == "" {
		return Identity{}, ErrNoSSOSession
	}

	var body []byte
	if err := json.Encode(&tokenRequest{ProjectID: projectID, ClientSecret: clientSecret}, &body); err != nil {
		return Identity{}, err
	}

	resp, err := doSync(fetch.Post(iamBaseURL+"/api/token").
		ContentTypeJSON().
		Header("Cookie", SSOCookieName+"="+ssoCookieValue).
		Body(body))
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
	ProjectID    string
	ClientSecret string
	Email        string
	Name         string
}

func (r *resolveUserRequest) IsNil() bool { return r == nil }
func (r *resolveUserRequest) EncodeFields(w model.FieldWriter) {
	w.String("project_id", r.ProjectID)
	w.String("client_secret", r.ClientSecret)
	w.String("email", r.Email)
	w.String("name", r.Name)
}
func (r *resolveUserRequest) DecodeFields(fr model.FieldReader) {}

type resolveUserResponse struct {
	Sub string
}

func (r *resolveUserResponse) IsNil() bool                      { return r == nil }
func (r *resolveUserResponse) EncodeFields(w model.FieldWriter) {}
func (r *resolveUserResponse) DecodeFields(fr model.FieldReader) {
	r.Sub, _ = fr.String("sub")
}

// ResolveUser calls POST <iamBaseURL>/api/users/resolve server-to-server:
// finds (or creates) a user by email within iam's identity, and returns
// their Sub — the same id their token's Sub will carry once they log in.
// For a consumer that needs to grant a resource (e.g. site ownership) to
// someone who may never have logged in yet. Requires only clientSecret —
// there is no session involved (see ARCHITECTURE.md/PathUsersResolve doc
// in iam's routes package for why that's safe).
func ResolveUser(iamBaseURL, projectID, clientSecret, email, name string) (string, error) {
	var body []byte
	if err := json.Encode(&resolveUserRequest{ProjectID: projectID, ClientSecret: clientSecret, Email: email, Name: name}, &body); err != nil {
		return "", err
	}
	resp, err := doSync(fetch.Post(iamBaseURL + "/api/users/resolve").
		ContentTypeJSON().
		Body(body))
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

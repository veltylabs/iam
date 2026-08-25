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
	Token string
}

func (t *tokenResponse) IsNil() bool                      { return t == nil }
func (t *tokenResponse) EncodeFields(w model.FieldWriter) {}
func (t *tokenResponse) DecodeFields(r model.FieldReader) {
	t.Token, _ = r.String("token")
}

// FetchAuthzToken calls POST <iamBaseURL>/api/token server-to-server,
// forwarding ssoCookieValue (the SSO cookie's value read off the caller's
// own incoming request) so iam can identify the user, and clientSecret so
// iam knows which project is asking. Returns the project-scoped
// authorization token's claims — Aud is projectID, Scope is the user's
// role codes in that project (see ARCHITECTURE.md §6.1/§6.2).
//
// The returned Claims are decoded WITHOUT verifying the HMAC signature
// (tinyjwt.DecodeUnverified, not Verify): this response came directly from
// iam over THIS SAME HTTPS call, not from an untrusted third party
// presenting a token later — there is nothing to gain from re-checking a
// signature whose secret this consumer does not, and must never, have
// (ARCHITECTURE.md §6.2: the HS256 secret is internal to iam). Do not
// change this to Verify — it would require sharing iam's secret, which is
// exactly the acoplamiento this design avoids.
func FetchAuthzToken(iamBaseURL, projectID, clientSecret, ssoCookieValue string) (tinyjwt.Claims, error) {
	if ssoCookieValue == "" {
		return tinyjwt.Claims{}, ErrNoSSOSession
	}

	var body []byte
	if err := json.Encode(&tokenRequest{ProjectID: projectID, ClientSecret: clientSecret}, &body); err != nil {
		return tinyjwt.Claims{}, err
	}

	resp, err := doSync(fetch.Post(iamBaseURL+"/api/token").
		ContentTypeJSON().
		Header("Cookie", SSOCookieName+"="+ssoCookieValue).
		Body(body))
	if err != nil {
		return tinyjwt.Claims{}, err
	}
	if resp.Status == 401 || resp.Status == 403 {
		return tinyjwt.Claims{}, ErrNoSSOSession
	}
	if resp.Status != 200 {
		return tinyjwt.Claims{}, fmt.Errf("iam: POST /api/token returned status %d", resp.Status)
	}

	var tr tokenResponse
	if err := json.Decode(resp.Body(), &tr); err != nil {
		return tinyjwt.Claims{}, err
	}
	return tinyjwt.DecodeUnverified(tr.Token)
}

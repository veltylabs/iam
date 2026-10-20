//go:build deploy

package config

import (
	"webtyp.com/deploy"
	"webtyp.com/github"
	"webtyp.com/goflare"
	"webtyp.com/lang"
)

// Deploy declares how iam is deployed. webtyp reads it without running it.
func Deploy() deploy.Plan {
	return deploy.Plan{
		Name:      "iam",
		Domain:    "iam.velty.cl",
		Target:    goflare.Worker,
		CI:        github.Actions,
		Databases: []deploy.Database{{Binding: "DB", Name: "velty-iam-db"}},
		Vars: []deploy.Var{
			{Key: "GOOGLE_REDIRECT_URL", Value: "https://iam.velty.cl/oauth/callback/google"},
			{Key: "IAM_PANEL_ORIGIN", Value: "https://iam.velty.cl"},
		},
		Secrets: []deploy.Secret{
			{
				Key:    "GOOGLE_CLIENT_ID",
				What:   "Identifier of iam's OAuth client at Google",
				Source: deploy.Typed,
				Guide: []lang.Text{
					"Open https://console.cloud.google.com/apis/credentials in the project that owns iam's login",
					"Create credentials, OAuth client ID, type Web application (or open the existing one)",
					"Authorized redirect URI: https://iam.velty.cl/oauth/callback/google",
					"Copy the value shown here",
				},
				To: []deploy.Place{deploy.ToRuntime},
			},
			{
				Key:    "GOOGLE_CLIENT_SECRET",
				What:   "Password of iam's OAuth client at Google",
				Source: deploy.Typed,
				Guide: []lang.Text{
					"Open https://console.cloud.google.com/apis/credentials in the project that owns iam's login",
					"Create credentials, OAuth client ID, type Web application (or open the existing one)",
					"Authorized redirect URI: https://iam.velty.cl/oauth/callback/google",
					"Copy the value shown here",
				},
				To: []deploy.Place{deploy.ToRuntime},
			},
			{
				Key:    "JWT_SECRET",
				What:   "Key that signs the authorization tokens",
				Source: deploy.Random,
				To:     []deploy.Place{deploy.ToRuntime, deploy.ToLocal},
			},
			{
				Key:    "IAM_ADMIN_EMAILS",
				What:   "Comma-separated emails seeded as iam_admin",
				Source: deploy.Typed,
				Guide:  []lang.Text{"Type the admin emails separated by commas"},
				To:     []deploy.Place{deploy.ToRuntime},
			},
		},
	}
}

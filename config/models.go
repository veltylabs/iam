package config

import "github.com/tinywasm/model"

// Project is iam's own concept, not rbac's: rbac knows roles/permissions
// scoped by project_id, but has no notion of an application authenticating
// itself to ask for a token — that credential belongs here.
var ProjectModel = model.Definition{
	Name: "project",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text()},
		{Name: "client_secret_hash", Type: model.Text()},
		{Name: "created_at", Type: model.Int()},
	},
}

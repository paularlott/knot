package msg

import "github.com/paularlott/knot/database/model"

type SyncTemplates struct {
	Existing []string
}

type UpdateTemplate struct {
	Template     model.Template
	UpdateFields []string
}

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

// HandleExportTemplate returns a portable YAML representation of a template
// suitable for version control and cross-instance import.
func HandleExportTemplate(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	templateId := r.PathValue("template_id")
	db := database.GetInstance()

	var template *model.Template
	var err error
	if validate.UUID(templateId) {
		template, err = db.GetTemplate(templateId)
	} else {
		template, err = db.GetTemplateByName(templateId)
	}
	if err != nil || template == nil || template.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "template not found"})
		return
	}

	// Same visibility as the read path: an export is a full read of the
	// template (job, volumes — including any registry auth in the job), so
	// users without template-manage permission may only export templates
	// they are allowed to see.
	if err := api_utils.CheckTemplateAccess(template.Id, user); err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "template not found"})
		return
	}

	// Fetch full details (includes job, volumes, schedule, etc.).
	details := buildTemplateExportDetails(template, db)

	yaml, err := details.String()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, yaml)
}

// HandleImportTemplate accepts a portable YAML template export, resolves
// script names to IDs, and creates the template via the existing create path.
func HandleImportTemplate(w http.ResponseWriter, r *http.Request) {
	var yamlText string
	if err := rest.DecodeRequestBody(w, r, &yamlText); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	exp, err := apiclient.ParseTemplateExport([]byte(yamlText))
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if exp.Name == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "template name is required"})
		return
	}

	// Resolve script names to IDs.
	db := database.GetInstance()
	if exp.StartupScript != "" {
		if id, ok := resolveScriptName(db, exp.StartupScript); ok {
			exp.StartupScript = id
		} else {
			exp.StartupScript = ""
		}
	}
	if exp.ShutdownScript != "" {
		if id, ok := resolveScriptName(db, exp.ShutdownScript); ok {
			exp.ShutdownScript = id
		} else {
			exp.ShutdownScript = ""
		}
	}

	// Convert to create request and delegate to the existing handler.
	createReq := exp.ToCreateRequest()
	jsonBody, err := json.Marshal(createReq)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Check if a template with this name already exists (upsert).
	if existing, err := db.GetTemplateByName(exp.Name); err == nil && existing != nil {
		// Update existing template — use a recorder so we can return a
		// consistent response with the template ID (HandleUpdateTemplate
		// returns 200 with no body).
		r2 := r.Clone(r.Context())
		r2.SetPathValue("template_id", existing.Id)
		r2.Body = io.NopCloser(bytes.NewReader(jsonBody))
		r2.ContentLength = int64(len(jsonBody))
		r2.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		HandleUpdateTemplate(rec, r2)
		if rec.Code == http.StatusOK {
			rest.WriteResponse(http.StatusOK, w, r, apiclient.TemplateCreateResponse{Status: true, Id: existing.Id})
		} else {
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
		}
		return
	}

	// Create new template.
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(jsonBody))
	r2.ContentLength = int64(len(jsonBody))
	r2.Header.Set("Content-Type", "application/json")
	HandleCreateTemplate(w, r2)
}

// buildTemplateExportDetails converts a template model into the portable export
// format, resolving script IDs to names.
func buildTemplateExportDetails(template *model.Template, db database.DbDriver) *apiclient.TemplateExport {
	details := &apiclient.TemplateDetails{
		Name:                     template.Name,
		Description:              template.Description,
		Platform:                 template.Platform,
		IconURL:                  template.IconURL,
		Active:                   template.Active,
		Job:                      template.Job,
		Volumes:                  template.Volumes,
		Groups:                   template.Groups,
		Zones:                    template.Zones,
		ComputeUnits:             template.ComputeUnits,
		StorageUnits:             template.StorageUnits,
		MaxUptime:                template.MaxUptime,
		MaxUptimeUnit:            template.MaxUptimeUnit,
		ScheduleEnabled:          template.ScheduleEnabled,
		AutoStart:                template.AutoStart,
		WithTerminal:             template.WithTerminal,
		WithVSCodeTunnel:         template.WithVSCodeTunnel,
		WithCodeServer:           template.WithCodeServer,
		WithSSH:                  template.WithSSH,
		WithRunCommand:           template.WithRunCommand,
		AllowNodeMigration:       template.AllowNodeMigration,
		HealthCheckType:          template.HealthCheckType,
		HealthCheckConfig:        template.HealthCheckConfig,
		HealthCheckSkipSSLVerify: template.HealthCheckSkipSSLVerify,
		HealthCheckTimeout:       template.HealthCheckTimeout,
		HealthCheckInterval:      template.HealthCheckInterval,
		HealthCheckMaxFailures:   template.HealthCheckMaxFailures,
		HealthCheckAutoRestart:   template.HealthCheckAutoRestart,
		DisableUserActivity:      template.DisableUserActivity,
		Ports:                    template.Ports,
	}
	if len(template.CustomFields) > 0 {
		details.CustomFields = make([]apiclient.CustomFieldDef, len(template.CustomFields))
		for i, cf := range template.CustomFields {
			details.CustomFields[i] = apiclient.CustomFieldDef{Name: cf.Name, Description: cf.Description}
		}
	}
	if len(template.Schedule) > 0 {
		details.Schedule = make([]apiclient.TemplateDetailsDay, len(template.Schedule))
		for i, s := range template.Schedule {
			details.Schedule[i] = apiclient.TemplateDetailsDay{Enabled: s.Enabled, From: s.From, To: s.To}
		}
	}

	exp := apiclient.ExportFromDetails(details)

	// Resolve script IDs to names.
	if template.StartupScriptId != "" {
		if script, err := db.GetScript(template.StartupScriptId); err == nil && script != nil {
			exp.StartupScript = script.Name
		}
	}
	if template.ShutdownScriptId != "" {
		if script, err := db.GetScript(template.ShutdownScriptId); err == nil && script != nil {
			exp.ShutdownScript = script.Name
		}
	}

	return exp
}

// resolveScriptName looks up a script by name (global scripts only) and
// returns its ID. Returns false if not found.
func resolveScriptName(db database.DbDriver, name string) (string, bool) {
	script, err := db.GetScriptByName(name)
	if err != nil || script == nil {
		return "", false
	}
	return script.Id, true
}

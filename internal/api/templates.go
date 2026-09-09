package api

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/spacejobs"
	"github.com/paularlott/knot/internal/specvalidate"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
	"strings"
)

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Get the query parameter user_id if present load the user
	// Supports both UUID and username
	userId := r.URL.Query().Get("user_id")
	if userId != "" {
		if !user.HasPermission(model.PermissionManageSpaces) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Permission denied"})
			return
		}

		db := database.GetInstance()
		var err error
		var lookupUser *model.User
		// Support lookup by both ID and username
		if validate.UUID(userId) {
			lookupUser, err = db.GetUser(userId)
		} else {
			lookupUser, err = db.GetUserByUsername(userId)
		}
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if lookupUser == nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "User not found"})
			return
		}
		user = lookupUser
	}

	templateService := service.GetTemplateService()
	templates, err := templateService.ListTemplates(service.TemplateListOptions{
		User:                 user,
		IncludeInactive:      true,
		IncludeDeleted:       false,
		CheckPermissions:     !user.HasPermission(model.PermissionManageTemplates),
		CheckZoneRestriction: false,
	})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of data to return to the client
	templateResponse := apiclient.TemplateList{
		Count:     0,
		Templates: []apiclient.TemplateInfo{},
	}

	for _, template := range templates {
		templateData := apiclient.TemplateInfo{}

		templateData.Id = template.Id
		templateData.Name = template.Name
		templateData.Description = template.Description
		templateData.Groups = template.Groups
		templateData.Platform = template.Platform
		templateData.IsManaged = template.IsManaged
		templateData.AllowNodeMigration = template.AllowNodeMigration
		templateData.ComputeUnits = template.ComputeUnits
		templateData.StorageUnits = template.StorageUnits
		templateData.ScheduleEnabled = template.ScheduleEnabled
		templateData.AutoStart = template.AutoStart
		templateData.Zones = template.Zones
		templateData.Active = template.Active
		templateData.MaxUptime = template.MaxUptime
		templateData.MaxUptimeUnit = template.MaxUptimeUnit
		templateData.IconURL = template.IconURL
		templateData.Ports = template.Ports
		templateData.Jobs = template.Jobs

		templateData.CustomFields = make([]apiclient.CustomFieldDef, len(template.CustomFields))
		for i, field := range template.CustomFields {
			templateData.CustomFields[i] = apiclient.CustomFieldDef{
				Name:        field.Name,
				Description: field.Description,
				Type:        field.Type,
				Handler:     field.Handler,
				Language:    field.Language,
				Default:     field.Default,
				Required:    field.Required,
				Options:     field.Options,
			}
		}

		// If schedule is enabled then return the schedule
		if template.ScheduleEnabled {
			templateData.Schedule = make([]apiclient.TemplateDetailsDay, 7)
			for i, day := range template.Schedule {
				templateData.Schedule[i] = apiclient.TemplateDetailsDay{
					Enabled: day.Enabled,
					From:    day.From,
					To:      day.To,
				}
			}
		}

		// Get template usage
		total, deployed, err := templateService.GetTemplateUsage(template.Id)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		templateData.Usage = total
		templateData.Deployed = deployed

		templateResponse.Templates = append(templateResponse.Templates, templateData)
		templateResponse.Count++
	}

	rest.WriteResponse(http.StatusOK, w, r, templateResponse)
}

func HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")

	request := apiclient.TemplateUpdateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if errs := spacejobs.ValidateJobs(request.Jobs); len(errs) > 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, JobsErrorResponse{
			Error:     "invalid job definitions: " + summarizeJobErrors(errs),
			JobErrors: errs,
		})
		return
	}

	if request.Platform == model.PlatformManual {
		request.Job = ""
		request.Volumes = ""
		request.ScheduleEnabled = false
		request.MaxUptimeUnit = "disabled"
		request.AllowNodeMigration = false
	}
	if request.Platform == model.PlatformNomad {
		request.AllowNodeMigration = false
	}

	// Support lookup by both ID and name
	db := database.GetInstance()
	var template *model.Template
	if validate.UUID(templateId) {
		template, err = db.GetTemplate(templateId)
	} else {
		template, err = db.GetTemplateByName(templateId)
	}
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	templateId = template.Id // Use the resolved ID for subsequent operations

	user := r.Context().Value("user").(*model.User)

	templateService := service.GetTemplateService()

	// Update template with request data
	template.Name = request.Name
	template.Description = request.Description
	template.Job = request.Job
	template.Volumes = request.Volumes
	template.Platform = request.Platform
	template.Groups = request.Groups
	template.WithTerminal = request.WithTerminal
	template.WithVSCodeTunnel = request.WithVSCodeTunnel
	template.WithCodeServer = request.WithCodeServer
	template.WithSSH = request.WithSSH
	template.WithRunCommand = request.WithRunCommand
	template.AllowNodeMigration = request.AllowNodeMigration
	template.StartupScriptId = request.StartupScriptId
	template.ShutdownScriptId = request.ShutdownScriptId
	template.ComputeUnits = request.ComputeUnits
	template.StorageUnits = request.StorageUnits
	template.ScheduleEnabled = request.ScheduleEnabled
	template.AutoStart = request.AutoStart
	template.Active = request.Active
	template.MaxUptime = request.MaxUptime
	template.MaxUptimeUnit = request.MaxUptimeUnit
	template.IconURL = request.IconURL
	template.Zones = request.Zones
	template.HealthCheckType = request.HealthCheckType
	template.HealthCheckConfig = request.HealthCheckConfig
	template.HealthCheckSkipSSLVerify = request.HealthCheckSkipSSLVerify
	template.HealthCheckTimeout = request.HealthCheckTimeout
	template.HealthCheckInterval = request.HealthCheckInterval
	template.HealthCheckMaxFailures = request.HealthCheckMaxFailures
	template.HealthCheckAutoRestart = request.HealthCheckAutoRestart
	template.DisableUserActivity = request.DisableUserActivity
	template.Ports = request.Ports
	template.Jobs = request.Jobs

	// Convert schedule
	template.Schedule = make([]model.TemplateScheduleDays, 7)
	for i, day := range request.Schedule {
		template.Schedule[i] = model.TemplateScheduleDays{
			Enabled: day.Enabled,
			From:    day.From,
			To:      day.To,
		}
	}

	customFields, fieldErr := normalizeCustomFields(request.CustomFields)
	if fieldErr != "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fieldErr})
		return
	}
	template.CustomFields = customFields

	err = templateService.UpdateTemplate(template, user)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Audit log
	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateUpdate,
		fmt.Sprintf("Updated template %s", template.Name),
		&map[string]interface{}{
			"template_id":   template.Id,
			"template_name": template.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	request := apiclient.TemplateCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if errs := spacejobs.ValidateJobs(request.Jobs); len(errs) > 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, JobsErrorResponse{
			Error:     "invalid job definitions: " + summarizeJobErrors(errs),
			JobErrors: errs,
		})
		return
	}

	if request.Platform == model.PlatformManual {
		request.Job = ""
		request.Volumes = ""
		request.ScheduleEnabled = false
		request.MaxUptimeUnit = "disabled"
		request.AllowNodeMigration = false
	}
	if request.Platform == model.PlatformNomad {
		request.AllowNodeMigration = false
	}

	user := r.Context().Value("user").(*model.User)

	// Convert schedule
	var scheduleDays []model.TemplateScheduleDays
	for _, day := range request.Schedule {
		scheduleDays = append(scheduleDays, model.TemplateScheduleDays{
			Enabled: day.Enabled,
			From:    day.From,
			To:      day.To,
		})
	}

	customFields, fieldErr := normalizeCustomFields(request.CustomFields)
	if fieldErr != "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fieldErr})
		return
	}

	var schedule *[]model.TemplateScheduleDays
	if request.ScheduleEnabled {
		schedule = &scheduleDays
	}

	template := model.NewTemplate(
		request.Name,
		request.Description,
		request.Job,
		request.Volumes,
		user.Id,
		request.Groups,
		request.Platform,
		request.WithTerminal,
		request.WithVSCodeTunnel,
		request.WithCodeServer,
		request.WithSSH,
		request.WithRunCommand,
		request.AllowNodeMigration,
		request.StartupScriptId,
		request.ShutdownScriptId,
		request.ComputeUnits,
		request.StorageUnits,
		request.ScheduleEnabled,
		schedule,
		request.Zones,
		request.AutoStart,
		request.Active,
		request.MaxUptime,
		request.MaxUptimeUnit,
		request.IconURL,
		customFields,
	)
	template.HealthCheckType = request.HealthCheckType
	template.HealthCheckConfig = request.HealthCheckConfig
	template.HealthCheckSkipSSLVerify = request.HealthCheckSkipSSLVerify
	template.HealthCheckTimeout = request.HealthCheckTimeout
	template.HealthCheckInterval = request.HealthCheckInterval
	template.HealthCheckMaxFailures = request.HealthCheckMaxFailures
	template.HealthCheckAutoRestart = request.HealthCheckAutoRestart
	template.DisableUserActivity = request.DisableUserActivity
	template.Ports = request.Ports
	template.Jobs = request.Jobs

	templateService := service.GetTemplateService()
	err = templateService.CreateTemplate(template, user)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Audit log
	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateCreate,
		fmt.Sprintf("Created template %s", template.Name),
		&map[string]interface{}{
			"template_id":   template.Id,
			"template_name": template.Name,
		},
	)

	// Return the ID
	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.TemplateCreateResponse{
		Status: true,
		Id:     template.Id,
	})
}

func HandleValidateTemplate(w http.ResponseWriter, r *http.Request) {
	request := apiclient.TemplateValidateRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	issues := specvalidate.ValidateTemplateSpec(request.Platform, request.Job, request.Volumes)
	response := apiclient.ValidationResponse{
		Valid:  len(issues) == 0,
		Errors: make([]apiclient.ValidationError, 0, len(issues)),
	}

	for _, issue := range issues {
		response.Errors = append(response.Errors, apiclient.ValidationError{
			Field:   issue.Field,
			Message: issue.Message,
			Line:    issue.Line,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")

	user := r.Context().Value("user").(*model.User)
	templateService := service.GetTemplateService()

	// Support lookup by both ID and name
	db := database.GetInstance()
	var template *model.Template
	var err error
	if validate.UUID(templateId) {
		template, err = db.GetTemplate(templateId)
	} else {
		template, err = db.GetTemplateByName(templateId)
	}
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Template not found"})
		} else {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}
	templateName := template.Name
	templateId = template.Id // Use the resolved ID for subsequent operations

	err = templateService.DeleteTemplate(templateId, user)
	if err != nil {
		if err.Error() == "template not found: sql: no rows in result set" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Template not found"})
		} else if err.Error() == "template is in use by spaces" || err.Error() == "template is in use" {
			rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
		} else {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}

	// Audit log
	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateDelete,
		fmt.Sprintf("Deleted template %s", templateName),
		&map[string]interface{}{
			"template_id":   templateId,
			"template_name": templateName,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	templateId := r.PathValue("template_id")

	// Support lookup by both ID and name
	var template *model.Template
	var err error
	db := database.GetInstance()

	if validate.UUID(templateId) {
		// Lookup by ID
		template, err = db.GetTemplate(templateId)
	} else {
		// Lookup by name
		template, err = db.GetTemplateByName(templateId)
	}

	if err != nil || template == nil || template.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "template not found"})
		return
	}

	// Now use GetTemplateDetails with the resolved ID
	data, err := api_utils.GetTemplateDetails(template.Id, user)
	if err != nil {
		if err.Error() == "Template not found: sql: no rows in result set" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Template not found"})
		} else if err.Error() == "No permission to access this template" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Template not found"})
		} else {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, data)
}

// templateCustomFieldTypes and templateFieldLanguages are the exact sets
// the template editor offers. Anything else is a load error, not a silent
// downgrade: an unknown type would fall through to the plain-text input on
// the space form. "masked" is the masking input (a browser type="password"
// control) - presentation only, values are stored as plain strings. "bool"
// renders the styled toggle; its value is the string "true" or "false",
// stored as a string like every other type.
var templateCustomFieldTypes = map[string]bool{
	"text": true, "masked": true, "number": true, "bool": true, "select": true, "autocomplete": true, "textarea": true,
}

var templateFieldLanguages = map[string]bool{
	"": true, "scriptling": true, "yaml": true, "toml": true, "json": true, "markdown": true, "shell": true,
}

// pluginHandlerIdRe is the qualified field-handler form
// plugin.<name>.<function> — the same trust posture as roles' plugin
// grants: shape-checked, existence not. A referenced plugin may be absent
// until it is reinstalled (cluster-order independent storage), and the
// options endpoint re-resolves the handler on every fetch.
var pluginHandlerIdRe = regexp.MustCompile(`^plugin\.[a-z0-9_-]+\.[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// normalizeCustomFields validates custom field declarations on template
// create/update: type must be one the space form renders, an autocomplete
// field must carry a well-formed plugin handler id, and a textarea's
// language must be one the editor offers. Handler and language are cleared
// on the types they do not apply to, so stale config never ships.
func normalizeCustomFields(fields []apiclient.CustomFieldDef) ([]model.TemplateCustomField, string) {
	out := make([]model.TemplateCustomField, 0, len(fields))
	for i, field := range fields {
		fieldType := field.Type
		if fieldType == "" {
			fieldType = "text"
		}
		if !templateCustomFieldTypes[fieldType] {
			return nil, fmt.Sprintf("custom_fields[%d].type must be one of text, masked, number, bool, select, autocomplete or textarea", i)
		}
		handler := field.Handler
		language := field.Language
		// select and autocomplete take their options from exactly one
		// source: a plugin field handler, or a manual option list (select
		// renders a dropdown, autocomplete a pick-or-create combobox).
		// Other types take neither.
		options := field.Options
		if fieldType == "select" || fieldType == "autocomplete" {
			if len(options) > 0 {
				if handler != "" {
					return nil, fmt.Sprintf("custom_fields[%d]: %s takes either a handler or a manual option list, not both", i, fieldType)
				}
				trimmed := make([]string, 0, len(options))
				for _, option := range options {
					if option = strings.TrimSpace(option); option != "" {
						trimmed = append(trimmed, option)
					}
				}
				if len(trimmed) == 0 {
					return nil, fmt.Sprintf("custom_fields[%d].options must have at least one entry", i)
				}
				options = trimmed
			} else if !pluginHandlerIdRe.MatchString(handler) {
				return nil, fmt.Sprintf("custom_fields[%d]: %s needs a plugin field handler or a list of options", i, fieldType)
			}
		} else {
			handler = ""
			options = nil
		}
		if fieldType == "textarea" {
			if !templateFieldLanguages[language] {
				return nil, fmt.Sprintf("custom_fields[%d].language must be one of scriptling, yaml, toml, json, markdown, shell or empty", i)
			}
		} else {
			language = ""
		}
		out = append(out, model.TemplateCustomField{
			Name:        field.Name,
			Description: field.Description,
			Type:        fieldType,
			Handler:     handler,
			Language:    language,
			Default:     field.Default,
			Required:    field.Required,
			Options:     options,
		})
	}
	return out, ""
}

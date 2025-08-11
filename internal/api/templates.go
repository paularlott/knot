package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Get the query parameter user_id if present load the user
	userId := r.URL.Query().Get("user_id")
	if userId != "" {
		if !user.HasPermission(model.PermissionManageSpaces) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Permission denied"})
			return
		}

		db := database.GetInstance()
		var err error
		user, err = db.GetUser(userId)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if user == nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "User not found"})
			return
		}
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
		templateData.ComputeUnits = template.ComputeUnits
		templateData.StorageUnits = template.StorageUnits
		templateData.ScheduleEnabled = template.ScheduleEnabled
		templateData.AutoStart = template.AutoStart
		templateData.Zones = template.Zones
		templateData.Active = template.Active
		templateData.MaxUptime = template.MaxUptime
		templateData.MaxUptimeUnit = template.MaxUptimeUnit
		templateData.IconURL = template.IconURL

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
	if !validate.UUID(templateId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template ID"})
		return
	}

	request := apiclient.TemplateUpdateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Platform == model.PlatformManual {
		request.Job = ""
		request.Volumes = ""
		request.ScheduleEnabled = false
		request.MaxUptimeUnit = "disabled"
	}

	user := r.Context().Value("user").(*model.User)

	templateService := service.GetTemplateService()
	template, err := templateService.GetTemplate(templateId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

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
	template.ComputeUnits = request.ComputeUnits
	template.StorageUnits = request.StorageUnits
	template.ScheduleEnabled = request.ScheduleEnabled
	template.AutoStart = request.AutoStart
	template.Active = request.Active
	template.MaxUptime = request.MaxUptime
	template.MaxUptimeUnit = request.MaxUptimeUnit
	template.IconURL = request.IconURL
	template.Zones = request.Zones

	// Convert schedule
	template.Schedule = make([]model.TemplateScheduleDays, 7)
	for i, day := range request.Schedule {
		template.Schedule[i] = model.TemplateScheduleDays{
			Enabled: day.Enabled,
			From:    day.From,
			To:      day.To,
		}
	}

	// Convert custom fields
	template.CustomFields = make([]model.TemplateCustomField, len(request.CustomFields))
	for i, field := range request.CustomFields {
		template.CustomFields[i] = model.TemplateCustomField{
			Name:        field.Name,
			Description: field.Description,
		}
	}

	err = templateService.UpdateTemplate(template, user)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Audit log
	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateUpdate,
		fmt.Sprintf("Updated template %s", template.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"template_id":     template.Id,
			"template_name":   template.Name,
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

	if request.Platform == model.PlatformManual {
		request.Job = ""
		request.Volumes = ""
		request.ScheduleEnabled = false
		request.MaxUptimeUnit = "disabled"
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

	// Convert custom fields
	var customFields []model.TemplateCustomField
	for _, field := range request.CustomFields {
		customFields = append(customFields, model.TemplateCustomField{
			Name:        field.Name,
			Description: field.Description,
		})
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

	templateService := service.GetTemplateService()
	err = templateService.CreateTemplate(template, user)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Audit log
	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateCreate,
		fmt.Sprintf("Created template %s", template.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"template_id":     template.Id,
			"template_name":   template.Name,
		},
	)

	// Return the ID
	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.TemplateCreateResponse{
		Status: true,
		Id:     template.Id,
	})
}

func HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	templateService := service.GetTemplateService()

	// Get template name for audit log before deletion
	template, err := templateService.GetTemplate(templateId)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Template not found"})
		} else {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}
	templateName := template.Name

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
	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateDelete,
		fmt.Sprintf("Deleted template %s", templateName),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"template_id":     templateId,
			"template_name":   templateName,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template ID"})
		return
	}

	templateService := service.GetTemplateService()
	template, err := templateService.GetTemplate(templateId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Get template usage
	total, deployed, err := templateService.GetTemplateUsage(templateId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	data := apiclient.TemplateDetails{
		Name:             template.Name,
		Description:      template.Description,
		Job:              template.Job,
		Volumes:          template.Volumes,
		Usage:            total,
		Hash:             template.Hash,
		Deployed:         deployed,
		Groups:           template.Groups,
		Zones:            template.Zones,
		Platform:         template.Platform,
		IsManaged:        template.IsManaged,
		WithTerminal:     template.WithTerminal,
		WithVSCodeTunnel: template.WithVSCodeTunnel,
		WithCodeServer:   template.WithCodeServer,
		WithSSH:          template.WithSSH,
		WithRunCommand:   template.WithRunCommand,
		ScheduleEnabled:  template.ScheduleEnabled,
		AutoStart:        template.AutoStart,
		Schedule:         make([]apiclient.TemplateDetailsDay, 7),
		ComputeUnits:     template.ComputeUnits,
		StorageUnits:     template.StorageUnits,
		Active:           template.Active,
		MaxUptime:        template.MaxUptime,
		MaxUptimeUnit:    template.MaxUptimeUnit,
		IconURL:          template.IconURL,
		CustomFields:     make([]apiclient.CustomFieldDef, len(template.CustomFields)),
	}

	if len(template.Schedule) != 7 {
		for i := 0; i < 7; i++ {
			data.Schedule[i] = apiclient.TemplateDetailsDay{
				Enabled: false,
				From:    "12:00am",
				To:      "11:59pm",
			}
		}
	} else {
		for i, day := range template.Schedule {
			data.Schedule[i] = apiclient.TemplateDetailsDay{
				Enabled: day.Enabled,
				From:    day.From,
				To:      day.To,
			}
		}
	}

	for i, field := range template.CustomFields {
		data.CustomFields[i] = apiclient.CustomFieldDef{
			Name:        field.Name,
			Description: field.Description,
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, &data)
}

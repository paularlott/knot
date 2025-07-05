package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {

	db := database.GetInstance()

	user := r.Context().Value("user").(*model.User)

	// Get the query parameter user_id if present load the user
	userId := r.URL.Query().Get("user_id")
	if userId != "" {
		if !user.HasPermission(model.PermissionManageSpaces) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Permission denied"})
			return
		}

		var err error
		user, err = db.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if user == nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "User not found"})
			return
		}
	}

	templates, err := db.GetTemplates()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of data to return to the client
	templateResponse := apiclient.TemplateList{
		Count:     0,
		Templates: []apiclient.TemplateInfo{},
	}

	for _, template := range templates {
		if template.IsDeleted {
			continue
		}

		// If the template has groups and no overlap with the user's groups then skip
		if !user.HasPermission(model.PermissionManageTemplates) && len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
			continue
		}

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

		// Find the number of spaces using this template
		spaces, err := db.GetSpacesByTemplateId(template.Id)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		var deployed int = 0
		var total int = 0
		for _, space := range spaces {
			if !space.IsDeleted {
				total++
				if space.IsDeployed || space.IsPending {
					deployed++
				}
			}
		}

		templateData.Usage = total
		templateData.Deployed = deployed

		templateResponse.Templates = append(templateResponse.Templates, templateData)
		templateResponse.Count++
	}

	rest.SendJSON(http.StatusOK, w, r, templateResponse)
}

func HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template ID"})
		return
	}

	request := apiclient.TemplateUpdateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Platform == model.PlatformManual {
		request.Job = ""
		request.Volumes = ""
		request.ScheduleEnabled = false
		request.MaxUptimeUnit = "disabled"
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template name given"})
		return
	}
	if request.Platform != model.PlatformManual && (!validate.Required(request.Job) || !validate.MaxLength(request.Job, 10*1024*1024)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Job is required and must be less than 10MB"})
		return
	}
	if !validate.MaxLength(request.Volumes, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volumes must be less than 10MB"})
		return
	}
	if !validate.IsPositiveNumber(int(request.ComputeUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Compute units must be a positive number"})
		return
	}
	if !validate.IsPositiveNumber(int(request.StorageUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Storage units must be a positive number"})
		return
	}
	if !validate.IsPositiveNumber(int(request.MaxUptime)) || !validate.OneOf(request.MaxUptimeUnit, []string{"disabled", "minute", "hour", "dat"}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Max uptime must be a positive number and unit must be one of disabled, minute, hour, day"})
		return
	}
	if !validate.OneOf(request.Platform, []string{model.PlatformManual, model.PlatformDocker, model.PlatformPodman, model.PlatformNomad}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid platform"})
		return
	}

	if request.ScheduleEnabled {
		if len(request.Schedule) != 7 {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Schedule must have 7 days"})
			return
		}
		for _, day := range request.Schedule {
			if !validate.IsTime(day.From) || !validate.IsTime(day.To) {
				rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid time format"})
				return
			}
		}
	}

	for _, field := range request.CustomFields {
		if !validate.Required(field.Name) || !validate.VarName(field.Name) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid custom field name given"})
			return
		}
		if !validate.MaxLength(field.Description, 256) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Custom field description must be less than 256 characters"})
			return
		}
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	template, err := db.GetTemplate(templateId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if template.IsManaged {
		rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot update managed template"})
		return
	}

	// Check the groups are present in the system
	for _, groupId := range request.Groups {
		_, err := db.GetGroup(groupId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
			return
		}
	}

	template.Name = request.Name
	template.Description = request.Description
	template.Job = request.Job
	template.Volumes = request.Volumes
	template.UpdatedUserId = user.Id
	template.Groups = request.Groups
	template.WithTerminal = request.WithTerminal
	template.WithVSCodeTunnel = request.WithVSCodeTunnel
	template.WithCodeServer = request.WithCodeServer
	template.WithSSH = request.WithSSH
	template.ComputeUnits = request.ComputeUnits
	template.StorageUnits = request.StorageUnits
	template.ScheduleEnabled = request.ScheduleEnabled
	template.AutoStart = request.AutoStart
	template.Schedule = make([]model.TemplateScheduleDays, 7)
	template.Zones = request.Zones
	template.UpdatedAt = hlc.Now()
	template.UpdatedUserId = user.Id
	template.Active = request.Active
	template.MaxUptime = request.MaxUptime
	template.MaxUptimeUnit = request.MaxUptimeUnit
	template.IconURL = request.IconURL
	template.Platform = request.Platform
	template.CustomFields = make([]model.TemplateCustomField, len(request.CustomFields))

	for i, day := range request.Schedule {
		template.Schedule[i] = model.TemplateScheduleDays{
			Enabled: day.Enabled,
			From:    day.From,
			To:      day.To,
		}
	}

	for i, field := range request.CustomFields {
		template.CustomFields[i] = model.TemplateCustomField{
			Name:        field.Name,
			Description: field.Description,
		}
	}

	template.UpdateHash()

	err = db.SaveTemplate(template, nil)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipTemplate(template)

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
	var templateId string

	request := apiclient.TemplateCreateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Platform == model.PlatformManual {
		request.Job = ""
		request.Volumes = ""
		request.ScheduleEnabled = false
		request.MaxUptimeUnit = "disabled"
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template name given"})
		return
	}
	if request.Platform != model.PlatformManual && (!validate.Required(request.Job) || !validate.MaxLength(request.Job, 10*1024*1024)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Job is required and must be less than 10MB"})
		return
	}
	if !validate.MaxLength(request.Volumes, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volumes must be less than 10MB"})
		return
	}
	if !validate.IsPositiveNumber(int(request.ComputeUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Compute units must be a positive number"})
		return
	}
	if !validate.IsPositiveNumber(int(request.StorageUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Storage units must be a positive number"})
		return
	}
	if !validate.IsPositiveNumber(int(request.MaxUptime)) || !validate.OneOf(request.MaxUptimeUnit, []string{"disabled", "minute", "hour", "dat"}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Max uptime must be a positive number and unit must be one of disabled, minute, hour, day"})
		return
	}
	if !validate.OneOf(request.Platform, []string{model.PlatformManual, model.PlatformDocker, model.PlatformPodman, model.PlatformNomad}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid platform"})
		return
	}

	if request.ScheduleEnabled {
		if len(request.Schedule) != 7 {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Schedule must have 7 days"})
			return
		}
		for _, day := range request.Schedule {
			if !validate.IsTime(day.From) || !validate.IsTime(day.To) {
				rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid time format"})
				return
			}
		}
	}

	for _, field := range request.CustomFields {
		if !validate.Required(field.Name) || !validate.VarName(field.Name) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid custom field name given"})
			return
		}
		if !validate.MaxLength(field.Description, 256) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Custom field description must be less than 256 characters"})
			return
		}
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	// Check the groups are present in the system
	for _, groupId := range request.Groups {
		_, err := db.GetGroup(groupId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
			return
		}
	}

	var scheduleDays = []model.TemplateScheduleDays{}
	for _, day := range request.Schedule {
		scheduleDays = append(scheduleDays, model.TemplateScheduleDays{
			Enabled: day.Enabled,
			From:    day.From,
			To:      day.To,
		})
	}

	var customFields = []model.TemplateCustomField{}
	for _, field := range request.CustomFields {
		customFields = append(customFields, model.TemplateCustomField{
			Name:        field.Name,
			Description: field.Description,
		})
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
		request.ComputeUnits,
		request.StorageUnits,
		request.ScheduleEnabled,
		&scheduleDays,
		request.Zones,
		request.AutoStart,
		request.Active,
		request.MaxUptime,
		request.MaxUptimeUnit,
		request.IconURL,
		customFields,
	)

	err = database.GetInstance().SaveTemplate(template, nil)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	templateId = template.Id

	service.GetTransport().GossipTemplate(template)

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
	rest.SendJSON(http.StatusCreated, w, r, &apiclient.TemplateCreateResponse{
		Status: true,
		Id:     templateId,
	})
}

func HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template ID"})
		return
	}

	template, err := database.GetInstance().GetTemplate(templateId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Find if any spaces are using this template and deny deletion
	spaces, err := database.GetInstance().GetSpacesByTemplateId(templateId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Count the spaces on this server
	activeSpaces := 0
	for _, space := range spaces {
		if !space.IsDeleted {
			activeSpaces++
		}
	}

	if activeSpaces > 0 {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "Template is in use by spaces"})
		return
	}

	// Delete the template
	template.IsDeleted = true
	template.UpdatedAt = hlc.Now()
	template.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	err = database.GetInstance().SaveTemplate(template, []string{"IsDeleted", "UpdatedAt", "UpdatedUserId"})
	if err != nil {
		if errors.Is(err, database.ErrTemplateInUse) {
			rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
		} else {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}

	service.GetTransport().GossipTemplate(template)

	user := r.Context().Value("user").(*model.User)
	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTemplateDelete,
		fmt.Sprintf("Deleted template %s", template.Name),
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

func HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template ID"})
		return
	}

	db := database.GetInstance()
	template, err := db.GetTemplate(templateId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Find the number of spaces using this template
	spaces, err := db.GetSpacesByTemplateId(templateId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	var deployed int = 0
	for _, space := range spaces {
		if space.IsDeployed || space.IsPending {
			deployed++
		}
	}

	data := apiclient.TemplateDetails{
		Name:             template.Name,
		Description:      template.Description,
		Job:              template.Job,
		Volumes:          template.Volumes,
		Usage:            len(spaces),
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

	rest.SendJSON(http.StatusOK, w, r, &data)
}

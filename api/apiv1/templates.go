package apiv1

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		templates, code, err := client.GetTemplates()
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, templates)
	} else {
		user := r.Context().Value("user").(*model.User)

		// Get the query parameter user_id if present load the user
		userId := r.URL.Query().Get("user_id")
		if userId != "" {
			if !user.HasPermission(model.PermissionManageSpaces) {
				rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Permission denied"})
				return
			}

			var err error
			user, err = database.GetInstance().GetUser(userId)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
			if user == nil {
				rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "User not found"})
				return
			}
		}

		templates, err := database.GetInstance().GetTemplates()
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

			// If the template has groups and no overlap with the user's groups then skip
			if !user.HasPermission(model.PermissionManageTemplates) && len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
				continue
			}

			templateData := apiclient.TemplateInfo{}

			templateData.Id = template.Id
			templateData.Name = template.Name
			templateData.Description = template.Description
			templateData.Groups = template.Groups
			templateData.LocalContainer = template.LocalContainer
			templateData.IsManual = template.IsManual
			templateData.ComputeUnits = template.ComputeUnits
			templateData.StorageUnits = template.StorageUnits
			templateData.ScheduleEnabled = template.ScheduleEnabled

			// Find the number of spaces using this template
			spaces, err := database.GetInstance().GetSpacesByTemplateId(template.Id)
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

			templateData.Usage = len(spaces)
			templateData.Deployed = deployed

			templateResponse.Templates = append(templateResponse.Templates, templateData)
			templateResponse.Count++
		}

		rest.SendJSON(http.StatusOK, w, r, templateResponse)
	}
}

func HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := chi.URLParam(r, "template_id")

	request := apiclient.TemplateUpdateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 255) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template name given"})
		return
	}
	if !validate.MaxLength(request.Job, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Job must be less than 10MB"})
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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.UpdateTemplate(templateId, request.Name, request.Job, request.Description, request.Volumes, request.Groups, request.WithTerminal, request.WithVSCodeTunnel, request.WithCodeServer, request.WithSSH, request.ComputeUnits, request.StorageUnits, request.ScheduleEnabled, &request.Schedule)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		template, err := db.GetTemplate(templateId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		if !template.IsManual && !validate.Required(request.Job) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Job is required and must be less than 10MB"})
			return
		}

		if template.IsManual {
			request.Job = ""
			request.Volumes = ""
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
		template.Schedule = make(model.JSONDbScheduleDays, 7)

		for i, day := range request.Schedule {
			template.Schedule[i] = model.TemplateScheduleDays{
				Enabled: day.Enabled,
				From:    day.From,
				To:      day.To,
			}
		}

		template.UpdateHash()

		err = db.SaveTemplate(template)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		api_utils.UpdateTemplateHash(template.Id, template.Hash)
		leaf.UpdateTemplate(template)
	}

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

	if request.IsManual {
		request.Job = ""
		request.Volumes = ""
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 255) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template name given"})
		return
	}
	if (!request.IsManual && !validate.Required(request.Job)) || !validate.MaxLength(request.Job, 10*1024*1024) {
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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var code int
		var err error

		templateId, code, err = client.CreateTemplate(request.Name, request.Job, request.Description, request.Volumes, request.Groups, request.LocalContainer, request.IsManual, request.WithTerminal, request.WithVSCodeTunnel, request.WithCodeServer, request.WithSSH, request.ComputeUnits, request.StorageUnits, request.ScheduleEnabled, &request.Schedule)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
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

		template := model.NewTemplate(request.Name, request.Description, request.Job, request.Volumes, user.Id, request.Groups, request.LocalContainer, request.IsManual, request.WithTerminal, request.WithVSCodeTunnel, request.WithCodeServer, request.WithSSH, request.ComputeUnits, request.StorageUnits, request.ScheduleEnabled, &scheduleDays)

		err = database.GetInstance().SaveTemplate(template)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		templateId = template.Id

		api_utils.UpdateTemplateHash(template.Id, template.Hash)
		leaf.UpdateTemplate(template)
	}

	// Return the ID
	rest.SendJSON(http.StatusCreated, w, r, &apiclient.TemplateCreateResponse{
		Status: true,
		Id:     templateId,
	})
}

func HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := chi.URLParam(r, "template_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.DeleteTemplate(templateId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
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

		if len(spaces) > 0 {
			rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "Template is in use by spaces"})
			return
		}

		// Delete the template
		err = database.GetInstance().DeleteTemplate(template)
		if err != nil {
			if errors.Is(err, database.ErrTemplateInUse) {
				rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
			} else {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			}
			return
		}

		api_utils.DeleteTemplateHash(template.Id)
		leaf.DeleteTemplate(template.Id)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := chi.URLParam(r, "template_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		template, code, err := client.GetTemplate(templateId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, template)
	} else {
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
			LocalContainer:   template.LocalContainer,
			IsManual:         template.IsManual,
			WithTerminal:     template.WithTerminal,
			WithVSCodeTunnel: template.WithVSCodeTunnel,
			WithCodeServer:   template.WithCodeServer,
			WithSSH:          template.WithSSH,
			ScheduleEnabled:  template.ScheduleEnabled,
			Schedule:         make([]apiclient.TemplateDetailsDay, 7),
			ComputeUnits:     template.ComputeUnits,
			StorageUnits:     template.StorageUnits,
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

		rest.SendJSON(http.StatusOK, w, r, &data)
	}
}

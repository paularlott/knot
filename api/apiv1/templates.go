package apiv1

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	templateHashMutex = &sync.RWMutex{}
	templateHashes    = make(map[string]string)
)

func SyncTemplateHashes() {
	if viper.GetBool("server.is_remote") {
		go func() {
			log.Info().Msg("server: starting remote server template hash sync")

			for {
				client := apiclient.NewRemoteServerClient(viper.GetString("server.core_server"))
				hashes, err := client.RemoteFetchTemplateHashes()
				if err != nil {
					log.Error().Msgf("failed to fetch template hashes: %s", err.Error())
				} else {
					templateHashes = *hashes
				}

				time.Sleep(model.REMOTE_SERVER_TEMPLATE_FETCH_HASH_INTERVAL)
			}
		}()
	} else {
		log.Info().Msg("server: loading template hashes")

		db := database.GetInstance()

		// Load the template hashes from the database
		templateHashMutex.Lock()

		templates, err := db.GetTemplates()
		if err != nil {
			log.Fatal().Msgf("server: failed to load templates: %s", err.Error())
		}

		for _, template := range templates {
			templateHashes[template.Id] = template.Hash
		}

		templateHashMutex.Unlock()
	}
}

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {
	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		templates, code, err := client.GetTemplates()
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, templates)
	} else {
		user := r.Context().Value("user").(*model.User)

		// Get the query parameter user_id if present load the user
		userId := r.URL.Query().Get("user_id")
		if userId != "" {
			if !user.HasPermission(model.PermissionManageSpaces) {
				rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "Permission denied"})
				return
			}

			var err error
			user, err = database.GetInstance().GetUser(userId)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}
			if user == nil {
				rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "User not found"})
				return
			}
		}

		templates, err := database.GetInstance().GetTemplates()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Build a json array of data to return to the client
		templateResponse := apiclient.TemplateList{}

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

			// Find the number of spaces using this template
			spaces, err := database.GetInstance().GetSpacesByTemplateId(template.Id)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
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

		rest.SendJSON(http.StatusOK, w, templateResponse)
	}
}

func HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	templateId := chi.URLParam(r, "template_id")

	request := apiclient.TemplateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 255) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template name given"})
		return
	}
	if !validate.Required(request.Job) || !validate.MaxLength(request.Job, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Job is required and must be less than 10MB"})
		return
	}
	if !validate.MaxLength(request.Volumes, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Volumes must be less than 10MB"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.UpdateTemplate(templateId, request.Name, request.Job, request.Description, request.Volumes, request.Groups)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		template, err := database.GetInstance().GetTemplate(templateId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Check the groups are present in the system
		for _, groupId := range request.Groups {
			_, err := db.GetGroup(groupId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
				return
			}
		}

		template.Name = request.Name
		template.Description = request.Description
		template.Job = request.Job
		template.Volumes = request.Volumes
		template.UpdatedUserId = user.Id
		template.Groups = request.Groups
		template.UpdateHash()

		err = database.GetInstance().SaveTemplate(template)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		templateHashMutex.Lock()
		defer templateHashMutex.Unlock()
		templateHashes[template.Id] = template.Hash
	}

	w.WriteHeader(http.StatusOK)
}

func HandleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var templateId string

	request := apiclient.TemplateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 255) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template name given"})
		return
	}
	if !validate.Required(request.Job) || !validate.MaxLength(request.Job, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Job is required and must be less than 10MB"})
		return
	}
	if !validate.MaxLength(request.Volumes, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Volumes must be less than 10MB"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var code int
		var err error

		templateId, code, err = client.CreateTemplate(request.Name, request.Job, request.Description, request.Volumes, request.Groups)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		// Check the groups are present in the system
		for _, groupId := range request.Groups {
			_, err := db.GetGroup(groupId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
				return
			}
		}

		template := model.NewTemplate(request.Name, request.Description, request.Job, request.Volumes, user.Id, request.Groups)

		err = database.GetInstance().SaveTemplate(template)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		templateId = template.Id

		templateHashMutex.Lock()
		defer templateHashMutex.Unlock()
		templateHashes[template.Id] = template.Hash
	}

	// Return the ID
	rest.SendJSON(http.StatusCreated, w, &apiclient.TemplateCreateResponse{
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
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		template, err := database.GetInstance().GetTemplate(templateId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Don't allow the manual template to be deleted
		if template.Id == model.MANUAL_TEMPLATE_ID {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Manual template cannot be deleted"})
			return
		}

		// Find if any spaces are using this template and deny deletion
		spaces, err := database.GetInstance().GetSpacesByTemplateId(templateId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		if len(spaces) > 0 {
			rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "Template is in use by spaces"})
			return
		}

		// Delete the template
		err = database.GetInstance().DeleteTemplate(template)
		if err != nil {
			if errors.Is(err, database.ErrTemplateInUse) {
				rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: err.Error()})
			} else {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			}
			return
		}
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
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, template)
	} else {
		db := database.GetInstance()
		template, err := db.GetTemplate(templateId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Find the number of spaces using this template
		spaces, err := db.GetSpacesByTemplateId(templateId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		var deployed int = 0
		for _, space := range spaces {
			if space.IsDeployed || space.IsPending {
				deployed++
			}
		}

		volumes, _ := template.GetVolumes(nil, nil, nil, false)

		var volumeList []map[string]interface{}

		if volumes != nil {
			for _, volume := range volumes.Volumes {
				var capacityMin int64 = 0
				var capacityMax int64 = 0

				if volume.CapacityMin != nil {
					capacityMin = int64(math.Max(1, math.Ceil(float64(volume.CapacityMin.(int64))/(1024*1024*1024))))
				}

				if volume.CapacityMax != nil {
					capacityMax = int64(math.Max(1, math.Ceil(float64(volume.CapacityMax.(int64))/(1024*1024*1024))))
				}

				volumeList = append(volumeList, map[string]interface{}{
					"id":           volume.Id,
					"name":         volume.Name,
					"capacity_min": capacityMin,
					"capacity_max": capacityMax,
				})
			}
		}

		data := apiclient.TemplateDetails{
			Name:        template.Name,
			Description: template.Description,
			Job:         template.Job,
			Volumes:     template.Volumes,
			Usage:       len(spaces),
			Hash:        template.Hash,
			Deployed:    deployed,
			Groups:      template.Groups,
			VolumeSizes: volumeList,
		}

		rest.SendJSON(http.StatusOK, w, &data)
	}
}

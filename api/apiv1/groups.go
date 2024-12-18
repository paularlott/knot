package apiv1

import (
	"errors"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

func HandleGetGroups(w http.ResponseWriter, r *http.Request) {
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		groups, code, err := client.GetGroups()
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, groups)

	} else {
		groups, err := database.GetInstance().GetGroups()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Build a json array of data to return to the client
		data := apiclient.GroupInfoList{
			Count:  0,
			Groups: []apiclient.GroupInfo{},
		}

		for _, group := range groups {
			g := apiclient.GroupInfo{
				Id:   group.Id,
				Name: group.Name,
			}
			data.Groups = append(data.Groups, g)
			data.Count++
		}

		rest.SendJSON(http.StatusOK, w, r, data)
	}
}

func HandleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupId := chi.URLParam(r, "group_id")

	request := apiclient.UserGroupRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user group name"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.UpdateGroup(groupId, request.Name)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		group, err := db.GetGroup(groupId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		group.Name = request.Name
		group.UpdatedUserId = user.Id

		err = db.SaveGroup(group)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	request := apiclient.UserGroupRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user group name"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		groupId, code, err := client.CreateGroup(request.Name)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusCreated, w, r, apiclient.GroupResponse{
			Status: true,
			Id:     groupId,
		})
	} else {
		group := model.NewGroup(request.Name, user.Id)

		err = database.GetInstance().SaveGroup(group)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Return the ID
		rest.SendJSON(http.StatusCreated, w, r, apiclient.GroupResponse{
			Status: true,
			Id:     group.Id,
		})
	}
}

func HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupId := chi.URLParam(r, "group_id")

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.DeleteGroup(groupId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		group, err := db.GetGroup(groupId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Delete the group
		err = db.DeleteGroup(group)
		if err != nil {
			if errors.Is(err, database.ErrTemplateInUse) {
				rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
			} else {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			}
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetGroup(w http.ResponseWriter, r *http.Request) {
	groupId := chi.URLParam(r, "group_id")

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		group, code, err := client.GetGroup(groupId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, group)
	} else {
		db := database.GetInstance()
		group, err := db.GetGroup(groupId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if group == nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "Group not found"})
			return
		}

		data := apiclient.GroupInfo{
			Id:   group.Id,
			Name: group.Name,
		}

		rest.SendJSON(http.StatusOK, w, r, data)
	}
}

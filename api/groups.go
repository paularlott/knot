package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/cluster"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

func HandleGetGroups(w http.ResponseWriter, r *http.Request) {
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
		if group.IsDeleted {
			continue
		}

		g := apiclient.GroupInfo{
			Id:           group.Id,
			Name:         group.Name,
			MaxSpaces:    group.MaxSpaces,
			ComputeUnits: group.ComputeUnits,
			StorageUnits: group.StorageUnits,
			MaxTunnels:   group.MaxTunnels,
		}
		data.Groups = append(data.Groups, g)
		data.Count++
	}

	rest.SendJSON(http.StatusOK, w, r, data)
}

func HandleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupId := r.PathValue("group_id")

	if !validate.UUID(groupId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid group ID"})
		return
	}

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
	if !validate.IsNumber(int(request.MaxSpaces), 0, 10000) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid max spaces"})
		return
	}
	if !validate.IsPositiveNumber(int(request.ComputeUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid compute units"})
		return
	}
	if !validate.IsPositiveNumber(int(request.StorageUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid storage units"})
		return
	}
	if !validate.IsPositiveNumber(int(request.MaxTunnels)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid tunnel limit"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	group, err := db.GetGroup(groupId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	group.Name = request.Name
	group.UpdatedUserId = user.Id
	group.MaxSpaces = request.MaxSpaces
	group.ComputeUnits = request.ComputeUnits
	group.StorageUnits = request.StorageUnits
	group.MaxTunnels = request.MaxTunnels
	group.UpdatedAt = time.Now().UTC()
	group.UpdatedUserId = user.Id

	err = db.SaveGroup(group)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	cluster.GetInstance().GossipGroup(group)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventGroupUpdate,
		fmt.Sprintf("Updated group %s", group.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"group_id":        group.Id,
			"group_name":      group.Name,
		},
	)

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
	if !validate.IsNumber(int(request.MaxSpaces), 0, 10000) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid max spaces"})
		return
	}
	if !validate.IsPositiveNumber(int(request.ComputeUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid compute units"})
		return
	}
	if !validate.IsPositiveNumber(int(request.StorageUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid storage units"})
		return
	}
	if !validate.IsPositiveNumber(int(request.MaxTunnels)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid tunnel limit"})
		return
	}

	group := model.NewGroup(request.Name, user.Id, request.MaxSpaces, request.ComputeUnits, request.StorageUnits, request.MaxTunnels)

	err = database.GetInstance().SaveGroup(group)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	cluster.GetInstance().GossipGroup(group)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventGroupCreate,
		fmt.Sprintf("Created group %s", group.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"group_id":        group.Id,
			"group_name":      group.Name,
		},
	)

	// Return the ID
	rest.SendJSON(http.StatusCreated, w, r, apiclient.GroupResponse{
		Status: true,
		Id:     group.Id,
	})
}

func HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupId := r.PathValue("group_id")

	if !validate.UUID(groupId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid group ID"})
		return
	}

	db := database.GetInstance()
	group, err := db.GetGroup(groupId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	user := r.Context().Value("user").(*model.User)

	// Delete the group
	group.IsDeleted = true
	group.UpdatedAt = time.Now().UTC()
	group.UpdatedUserId = user.Id
	err = db.SaveGroup(group)
	if err != nil {
		if errors.Is(err, database.ErrTemplateInUse) {
			rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
		} else {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}

	cluster.GetInstance().GossipGroup(group)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventGroupDelete,
		fmt.Sprintf("Deleted group %s", group.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"group_id":        group.Id,
			"group_name":      group.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetGroup(w http.ResponseWriter, r *http.Request) {
	groupId := r.PathValue("group_id")

	if !validate.UUID(groupId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid group ID"})
		return
	}

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
		Id:           group.Id,
		Name:         group.Name,
		MaxSpaces:    group.MaxSpaces,
		ComputeUnits: group.ComputeUnits,
		StorageUnits: group.StorageUnits,
		MaxTunnels:   group.MaxTunnels,
	}

	rest.SendJSON(http.StatusOK, w, r, data)
}

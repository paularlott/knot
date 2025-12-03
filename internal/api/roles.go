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
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetRoles(w http.ResponseWriter, r *http.Request) {
	roles := model.GetRolesFromCache()

	// Build the response
	roleInfoList := apiclient.RoleInfoList{
		Count: len(roles),
		Roles: make([]apiclient.RoleInfo, 0, len(roles)),
	}

	for _, role := range roles {
		if role.IsDeleted {
			continue
		}

		roleInfoList.Roles = append(roleInfoList.Roles, apiclient.RoleInfo{
			Id:   role.Id,
			Name: role.Name,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, roleInfoList)
}

func HandleUpdateRole(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	roleId := r.PathValue("role_id")

	if !validate.UUID(roleId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid role ID"})
		return
	}

	var role *model.Role

	request := apiclient.RoleRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user role name"})
		return
	}

	if roleId == model.RoleAdminUUID {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot update the admin role"})
		return
	}

	db := database.GetInstance()

	role, err = db.GetRole(roleId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	role.Name = request.Name
	role.Permissions = request.Permissions
	role.UpdatedUserId = user.Id
	role.UpdatedAt = hlc.Now()

	err = db.SaveRole(role)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventRoleUpdate,
		fmt.Sprintf("Updated role %s", role.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"role_id":         role.Id,
			"role_name":       role.Name,
		},
	)

	model.SaveRoleToCache(role)
	service.GetTransport().GossipRole(role)
	sse.PublishRolesChanged()

	w.WriteHeader(http.StatusOK)
}

func HandleCreateRole(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	request := apiclient.RoleRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user role name"})
		return
	}

	role := model.NewRole(request.Name, request.Permissions, user.Id)

	err = database.GetInstance().SaveRole(role)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	model.SaveRoleToCache(role)
	service.GetTransport().GossipRole(role)
	sse.PublishRolesChanged()

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventRoleCreate,
		fmt.Sprintf("Created role %s", role.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"role_id":         role.Id,
			"role_name":       role.Name,
		},
	)

	// Return the ID
	rest.WriteResponse(http.StatusCreated, w, r, apiclient.RoleResponse{
		Status: true,
		Id:     role.Id,
	})
}

func HandleDeleteRole(w http.ResponseWriter, r *http.Request) {
	roleId := r.PathValue("role_id")

	if !validate.UUID(roleId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid role ID"})
		return
	}

	if roleId == model.RoleAdminUUID {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot delete the admin role"})
		return
	}

	db := database.GetInstance()
	role, err := db.GetRole(roleId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	user := r.Context().Value("user").(*model.User)

	// Delete the role
	role.UpdatedAt = hlc.Now()
	role.UpdatedUserId = user.Id
	role.IsDeleted = true
	err = db.SaveRole(role)
	if err != nil {
		if errors.Is(err, database.ErrTemplateInUse) {
			rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
		} else {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventRoleDelete,
		fmt.Sprintf("Deleted role %s", role.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"role_id":         role.Id,
			"role_name":       role.Name,
		},
	)

	model.DeleteRoleFromCache(roleId)
	service.GetTransport().GossipRole(role)
	sse.PublishRolesChanged()

	w.WriteHeader(http.StatusOK)
}

func HandleGetRole(w http.ResponseWriter, r *http.Request) {
	roleId := r.PathValue("role_id")

	if !validate.UUID(roleId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid role ID"})
		return
	}

	db := database.GetInstance()
	role, err := db.GetRole(roleId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if role == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Role not found"})
		return
	}

	data := apiclient.RoleDetails{
		Id:          role.Id,
		Name:        role.Name,
		Permissions: role.Permissions,
	}

	rest.WriteResponse(http.StatusOK, w, r, data)
}

package apiv1

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

func HandleGetRoles(w http.ResponseWriter, r *http.Request) {
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		roleInfoList, code, err := client.GetRoles()
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, roleInfoList)
	} else {
		roles := model.GetRolesFromCache()

		// Build the response
		roleInfoList := apiclient.RoleInfoList{
			Count: len(roles),
			Roles: make([]apiclient.RoleInfo, len(roles)),
		}

		for i, role := range roles {
			roleInfoList.Roles[i] = apiclient.RoleInfo{
				Id:   role.Id,
				Name: role.Name,
			}
		}

		rest.SendJSON(http.StatusOK, w, r, roleInfoList)
	}
}

func HandleUpdateRole(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	roleId := r.PathValue("role_id")

	if !validate.UUID(roleId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid role ID"})
		return
	}

	var role *model.Role

	request := apiclient.UserRoleRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user role name"})
		return
	}

	if roleId == model.RoleAdminUUID {
		rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot update the admin role"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.UpdateRole(roleId, request.Name, request.Permissions)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		role = &model.Role{
			Id:          roleId,
			Name:        request.Name,
			Permissions: request.Permissions,
		}
	} else {
		db := database.GetInstance()

		role, err = db.GetRole(roleId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		role.Name = request.Name
		role.Permissions = request.Permissions
		role.UpdatedUserId = user.Id

		err = db.SaveRole(role)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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
	}

	model.SaveRoleToCache(role)
	leaf.UpdateRole(role, nil)

	w.WriteHeader(http.StatusOK)
}

func HandleCreateRole(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	request := apiclient.UserRoleRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user role name"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		roleId, code, err := client.CreateRole(request.Name, request.Permissions)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		role := &model.Role{
			Id:          roleId,
			Name:        request.Name,
			Permissions: request.Permissions,
		}
		model.SaveRoleToCache(role)
		leaf.UpdateRole(role, nil)

		rest.SendJSON(http.StatusCreated, w, r, apiclient.RoleResponse{
			Status: true,
			Id:     roleId,
		})
	} else {
		role := model.NewRole(request.Name, request.Permissions, user.Id)

		err = database.GetInstance().SaveRole(role)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		model.SaveRoleToCache(role)
		leaf.UpdateRole(role, nil)

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
		rest.SendJSON(http.StatusCreated, w, r, apiclient.RoleResponse{
			Status: true,
			Id:     role.Id,
		})
	}
}

func HandleDeleteRole(w http.ResponseWriter, r *http.Request) {
	roleId := r.PathValue("role_id")

	if !validate.UUID(roleId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid role ID"})
		return
	}

	if roleId == model.RoleAdminUUID {
		rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot delete the admin role"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.DeleteRole(roleId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		role, err := db.GetRole(roleId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Delete the role
		err = db.DeleteRole(role)
		if err != nil {
			if errors.Is(err, database.ErrTemplateInUse) {
				rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: err.Error()})
			} else {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			}
			return
		}

		user := r.Context().Value("user").(*model.User)
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
	}

	model.DeleteRoleFromCache(roleId)
	leaf.DeleteRole(roleId, nil)

	w.WriteHeader(http.StatusOK)
}

func HandleGetRole(w http.ResponseWriter, r *http.Request) {
	roleId := r.PathValue("role_id")

	if !validate.UUID(roleId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid role ID"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		role, code, err := client.GetRole(roleId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, role)
	} else {
		db := database.GetInstance()
		role, err := db.GetRole(roleId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if role == nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "Role not found"})
			return
		}

		data := apiclient.RoleDetails{
			Id:          role.Id,
			Name:        role.Name,
			Permissions: role.Permissions,
		}

		rest.SendJSON(http.StatusOK, w, r, data)
	}
}

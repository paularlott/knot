package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetPermissions(w http.ResponseWriter, r *http.Request) {
	permissionList := apiclient.PermissionInfoList{
		Count:       len(model.PermissionNames),
		Permissions: make([]apiclient.PermissionInfo, len(model.PermissionNames)),
	}

	for i, permission := range model.PermissionNames {
		permissionList.Permissions[i] = apiclient.PermissionInfo{
			Id:    permission.Id,
			Name:  permission.Name,
			Group: permission.Group,
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, permissionList)
}

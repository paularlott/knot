package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
)

func HandleGetPermissions(w http.ResponseWriter, r *http.Request) {
	permissionList := apiclient.PermissionInfoList{
		Count:       len(model.PermissionNames),
		Permissions: make([]apiclient.PermissionInfo, len(model.PermissionNames)),
	}

	for i, permission := range model.PermissionNames {
		permissionList.Permissions[i] = apiclient.PermissionInfo{
			Id:   permission.Id,
			Name: permission.Name,
		}
	}

	rest.SendJSON(http.StatusOK, w, r, permissionList)
}

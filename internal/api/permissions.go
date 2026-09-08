package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetPermissions(w http.ResponseWriter, r *http.Request) {
	permissionList := apiclient.PermissionInfoList{
		Count:       len(model.PermissionNames),
		Permissions: make([]apiclient.PermissionInfo, len(model.PermissionNames)),
	}

	for i, permission := range model.PermissionNames {
		permissionList.Permissions[i] = apiclient.PermissionInfo{
			Id:          permission.Id,
			Name:        permission.Name,
			Group:       permission.Group,
			Description: permission.Description,
		}
	}

	// Plugin-declared permissions ride alongside: qualified grant ids a
	// role may carry, grouped by the plugin that declares them.
	if registry := plugins.GetRegistry(); registry != nil {
		for _, plugin := range registry.All() {
			for _, decl := range plugin.Permissions {
				permissionList.PluginPermissions = append(permissionList.PluginPermissions, apiclient.PluginPermissionInfo{
					Id:     decl.Id,
					Plugin: plugin.Name,
					Label:  decl.Label,
				})
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, permissionList)
}

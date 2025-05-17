package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/config"
)

func checkPermissionUseManageSpaces(next http.HandlerFunc) http.HandlerFunc {
	if config.LeafNode {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermission(next http.HandlerFunc, permission uint16) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(permission) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageTemplates(next http.HandlerFunc) http.HandlerFunc {
	if config.LeafNode {
		return next
	}

	return checkPermission(next, model.PermissionManageTemplates)
}

func checkPermissionManageVariables(next http.HandlerFunc) http.HandlerFunc {
	if config.LeafNode {
		return next
	}

	return checkPermission(next, model.PermissionManageVariables)
}

func checkPermissionManageVolumes(next http.HandlerFunc) http.HandlerFunc {
	if config.LeafNode {
		return next
	}

	return checkPermission(next, model.PermissionManageVolumes)
}

func checkPermissionUseTunnels(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionUseTunnels)
}

func checkPermissionViewAuditLogs(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionViewAuditLogs)
}

func checkPermissionManageUsers(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageUsers)
}

func checkPermissionManageGroups(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageGroups)
}

func checkPermissionManageRoles(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageRoles)
}

func checkPermissionViewClusterInfo(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionClusterInfo)
}

package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
)

func checkPermissionUseManageSpaces(next http.HandlerFunc) http.HandlerFunc {
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
	return checkPermission(next, model.PermissionManageTemplates)
}

func checkPermissionManageVariables(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageVariables)
}

func checkPermissionManageVolumes(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageVolumes)
}

func checkPermissionUseTunnels(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionUseTunnels) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
func checkPermissionViewAuditLogs(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionViewAuditLogs) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageUsers(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageUsers) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageGroups(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageGroups)
}

func checkPermissionManageRoles(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageRoles)
}

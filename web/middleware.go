package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
)

func checkPermissionUseManageSpaces(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageTemplates(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageTemplates) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageVariables(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageVariables) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageVolumes(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageVolumes) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
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
		if server_info.RestrictedLeaf || !user.HasPermission(model.PermissionManageUsers) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageGroups(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageGroups) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageRoles(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageRoles) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

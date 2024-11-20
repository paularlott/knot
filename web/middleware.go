package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
)

func checkPermissionManageTemplates(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if origin.RestrictedLeaf || !user.HasPermission(model.PermissionManageTemplates) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageVolumes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !origin.RestrictedLeaf && !user.HasPermission(model.PermissionManageVolumes) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func checkPermissionManageUsers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if origin.RestrictedLeaf || !user.HasPermission(model.PermissionManageUsers) {
			showPageForbidden(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

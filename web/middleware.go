package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/database/model"
)

func checkPermissionManageTemplates(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    user := r.Context().Value("user").(*model.User)
    if !user.HasPermission(model.PermissionManageTemplates) {
      showPageForbidden(w, r)
      return
    }

    next.ServeHTTP(w, r)
  })
}

func checkPermissionManageUsers(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    user := r.Context().Value("user").(*model.User)
    if !user.HasPermission(model.PermissionManageUsers) {
      showPageForbidden(w, r)
      return
    }

    next.ServeHTTP(w, r)
  })
}

func checkPermissionManageUsersOrSelf(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    userId := chi.URLParam(r, "user_id")
    user := r.Context().Value("user").(*model.User)
    if !user.HasPermission(model.PermissionManageUsers) && user.Id != userId {
      showPageForbidden(w, r)
      return
    }

    next.ServeHTTP(w, r)
  })
}

package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleTokenCreatePage(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("page-api-tokens_create_named.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  user := r.Context().Value("user").(*model.User)

  token := model.NewToken(chi.URLParam(r, "token_name"), user.Id)
  db := database.GetInstance()
  err = db.SaveToken(token)
  if err != nil {
    log.Error().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data := map[string]interface{}{
    "username": user.Username,
    "permissionManageUsers": user.HasPermission(model.PermissionManageUsers),
    "permissionManageTemplates": user.HasPermission(model.PermissionManageTemplates),
    "TokenName": token.Name,
    "TokenId": token.Id,
  }

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}

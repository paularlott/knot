package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type DbServiceRequest struct {
  Name string `json:"name"`
  DbType string `json:"db_type"`
  DbHost string `json:"db_host"`
  DbPort int `json:"db_port"`
  DbUser string `json:"db_user"`
  DbPassword string `json:"db_password"`
  ProxyHost string `json:"proxy_host"`
  ProxyPort int `json:"proxy_port"`
  ProxyUser string `json:"proxy_user"`
  ProxyPassword string `json:"proxy_password"`
}

func HandleGetDbServices(w http.ResponseWriter, r *http.Request) {
  services, err := database.GetInstance().GetDbServices()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  data := make([]struct {
    Id string `json:"dbservice_id"`
    Name string `json:"name"`
    DbType string `json:"db_type"`
  }, len(services))

  for i, service := range services {
    data[i].Id = service.Id
    data[i].Name = service.Name
    data[i].DbType = service.DbType
  }

  rest.SendJSON(http.StatusOK, w, data)
}

func HandleUpdateDbService(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  service, err := db.GetDbService(chi.URLParam(r, "dbservice_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  request := DbServiceRequest{}
  err = rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.VarName(request.Name) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid db service name given"})
    return
  }

  service.DbType = request.DbType
  service.DbHost = request.DbHost
  service.DbPort = request.DbPort
  service.DbUser = request.DbUser
  service.DbPassword = request.DbPassword
  service.ProxyHost = request.ProxyHost
  service.ProxyPort = request.ProxyPort
  service.ProxyUser = request.ProxyUser
  service.ProxyPassword = request.ProxyPassword

  err = db.SaveDbService(service)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateDbService(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  request := DbServiceRequest{}
  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.VarName(request.Name) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template variable name given"})
    return
  }

  service := model.NewDbService(request.Name, request.DbType, request.DbHost, request.DbPort, request.DbUser, request.DbPassword, request.ProxyHost, request.ProxyPort, request.ProxyUser, request.ProxyPassword)

  err = db.SaveDbService(service)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    DbServiceID string `json:"dbservice_id"`
  }{
    Status: true,
    DbServiceID: service.Id,
  })
}

func HandleDeleteDbService(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  service, err := db.GetDbService(chi.URLParam(r, "dbservice_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Delete the template variable
  err = db.DeleteDbService(service)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleGetDbService(w http.ResponseWriter, r *http.Request) {
  serviceId := chi.URLParam(r, "dbservice_id")

  db := database.GetInstance()
  service, err := db.GetDbService(serviceId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  data := struct {
    Id string `json:"dbservice_id"`
    Name string `json:"name"`
    DbType string `json:"db_type"`
    DbHost string `json:"db_host"`
    DbPort int `json:"db_port"`
    DbUser string `json:"db_user"`
    DbPassword string `json:"db_password"`
    ProxyHost string `json:"proxy_host"`
    ProxyPort int `json:"proxy_port"`
    ProxyUser string `json:"proxy_user"`
    ProxyPassword string `json:"proxy_password"`
  }{
    Id: service.Id,
    Name: service.Name,
    DbType: service.DbType,
    DbHost: service.DbHost,
    DbPort: service.DbPort,
    DbUser: service.DbUser,
    DbPassword: service.DbPassword,
    ProxyHost: service.ProxyHost,
    ProxyPort: service.ProxyPort,
    ProxyUser: service.ProxyUser,
    ProxyPassword: service.ProxyPassword,
  }

  rest.SendJSON(http.StatusOK, w, data)
}

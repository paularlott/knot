package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

func HandleGetTemplateVars(w http.ResponseWriter, r *http.Request) {
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		templateVars, code, err := client.GetTemplateVars()
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, templateVars)
	} else {
		templateVars, err := database.GetInstance().GetTemplateVars()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Build a json array of data to return to the client
		data := apiclient.TemplateVarList{
			Count:       0,
			TemplateVar: []apiclient.TemplateVar{},
		}

		for _, variable := range templateVars {
			v := apiclient.TemplateVar{
				Id:         variable.Id,
				Name:       variable.Name,
				Location:   variable.Location,
				Local:      variable.Local,
				Protected:  variable.Protected,
				Restricted: variable.Restricted,
			}
			data.TemplateVar = append(data.TemplateVar, v)
			data.Count++
		}

		rest.SendJSON(http.StatusOK, w, r, data)
	}
}

func HandleUpdateTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := r.PathValue("templatevar_id")

	if !validate.UUID(templateVarId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid variable ID"})
		return
	}

	request := apiclient.TemplateVarValue{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.VarName(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template variable name given"})
		return
	}
	if !validate.MaxLength(request.Value, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Value must be less than 10MB"})
		return
	}
	if !validate.MaxLength(request.Location, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Location must be less than 64 characters"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.UpdateTemplateVar(templateVarId, request.Name, request.Location, request.Local, request.Value, request.Protected, request.Restricted)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		templateVar, err := db.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		templateVar.Name = request.Name
		templateVar.Location = request.Location
		templateVar.Local = request.Local
		templateVar.Value = request.Value
		templateVar.Protected = request.Protected
		templateVar.Restricted = request.Restricted
		templateVar.UpdatedUserId = user.Id

		err = db.SaveTemplateVar(templateVar)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventVarUpdate,
			fmt.Sprintf("Updated variable %s", templateVar.Name),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"var_id":          templateVar.Id,
				"var_name":        templateVar.Name,
			},
		)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleCreateTemplateVar(w http.ResponseWriter, r *http.Request) {
	var id string

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	request := apiclient.TemplateVarValue{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.VarName(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid template variable name given"})
		return
	}
	if !validate.MaxLength(request.Value, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Value must be less than 10MB"})
		return
	}
	if !validate.MaxLength(request.Location, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Location must be less than 64 characters"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		var code int
		var err error

		client := remoteClient.(*apiclient.ApiClient)
		id, code, err = client.CreateTemplateVar(request.Name, request.Location, request.Local, request.Value, request.Protected, request.Restricted)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		templateVar := model.NewTemplateVar(request.Name, request.Location, request.Local, request.Value, request.Protected, request.Restricted, user.Id)

		err = db.SaveTemplateVar(templateVar)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		id = templateVar.Id

		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventVarCreate,
			fmt.Sprintf("Created variable %s", templateVar.Name),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"var_id":          templateVar.Id,
				"var_name":        templateVar.Name,
			},
		)
	}

	// Return the ID
	rest.SendJSON(http.StatusCreated, w, r, &apiclient.TemplateVarCreateResponse{
		Status: true,
		Id:     id,
	})
}

func HandleDeleteTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := r.PathValue("templatevar_id")

	if !validate.UUID(templateVarId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid variable ID"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.DeleteTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		templateVar, err := db.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Delete the template variable
		err = db.DeleteTemplateVar(templateVar)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		user := r.Context().Value("user").(*model.User)
		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventVarDelete,
			fmt.Sprintf("Deleted variable %s", templateVar.Name),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"var_id":          templateVar.Id,
				"var_name":        templateVar.Name,
			},
		)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := r.PathValue("templatevar_id")

	if !validate.UUID(templateVarId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid variable ID"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		templateVar, code, err := client.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, templateVar)
	} else {
		db := database.GetInstance()
		templateVar, err := db.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if templateVar == nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "Template variable not found"})
			return
		}

		var val string

		if templateVar.Protected {
			val = ""
		} else {
			val = templateVar.Value
		}

		data := &apiclient.TemplateVarValue{
			Name:       templateVar.Name,
			Value:      val,
			Location:   templateVar.Location,
			Local:      templateVar.Local,
			Protected:  templateVar.Protected,
			Restricted: templateVar.Restricted,
		}

		rest.SendJSON(http.StatusOK, w, r, data)
	}
}

package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

func HandleGetTemplateVars(w http.ResponseWriter, r *http.Request) {
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		templateVars, code, err := client.GetTemplateVars()
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, templateVars)
	} else {
		templateVars, err := database.GetInstance().GetTemplateVars()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Build a json array of data to return to the client
		data := apiclient.TemplateVarList{
			Count:       0,
			TemplateVar: []apiclient.TemplateVar{},
		}

		for _, variable := range templateVars {
			v := apiclient.TemplateVar{
				Id:        variable.Id,
				Name:      variable.Name,
				Protected: variable.Protected,
			}
			data.TemplateVar = append(data.TemplateVar, v)
			data.Count++
		}

		rest.SendJSON(http.StatusOK, w, data)
	}
}

func HandleUpdateTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := chi.URLParam(r, "templatevar_id")

	request := apiclient.TemplateVarValue{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.VarName(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template variable name given"})
		return
	}
	if !validate.MaxLength(request.Value, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Value must be less than 10MB"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.UpdateTemplateVar(templateVarId, request.Name, request.Value, request.Protected)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		templateVar, err := db.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		templateVar.Name = request.Name
		templateVar.Value = request.Value
		templateVar.Protected = request.Protected
		templateVar.UpdatedUserId = user.Id

		err = db.SaveTemplateVar(templateVar)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
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
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.VarName(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template variable name given"})
		return
	}
	if !validate.MaxLength(request.Value, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Value must be less than 10MB"})
		return
	}

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		var code int
		var err error

		client := remoteClient.(*apiclient.ApiClient)
		id, code, err = client.CreateTemplateVar(request.Name, request.Value, request.Protected)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		templateVar := model.NewTemplateVar(request.Name, request.Value, request.Protected, user.Id)

		err = db.SaveTemplateVar(templateVar)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		id = templateVar.Id
	}

	// Return the ID
	rest.SendJSON(http.StatusCreated, w, &apiclient.TemplateVarCreateResponse{
		Status: true,
		Id:     id,
	})
}

func HandleDeleteTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := chi.URLParam(r, "templatevar_id")

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		code, err := client.DeleteTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		templateVar, err := db.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Delete the template variable
		err = db.DeleteTemplateVar(templateVar)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := chi.URLParam(r, "templatevar_id")

	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		templateVar, code, err := client.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, templateVar)
	} else {
		db := database.GetInstance()
		templateVar, err := db.GetTemplateVar(templateVarId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}
		if templateVar == nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "Template variable not found"})
			return
		}

		var val string

		if templateVar.Protected {
			val = ""
		} else {
			val = templateVar.Value
		}

		data := &apiclient.TemplateVarValue{
			Name:      templateVar.Name,
			Value:     val,
			Protected: templateVar.Protected,
		}

		rest.SendJSON(http.StatusOK, w, data)
	}
}

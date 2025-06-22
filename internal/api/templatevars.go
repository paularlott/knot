package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetTemplateVars(w http.ResponseWriter, r *http.Request) {
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
		if variable.IsDeleted {
			continue
		}

		v := apiclient.TemplateVar{
			Id:         variable.Id,
			Name:       variable.Name,
			Zones:      variable.Zones,
			Local:      variable.Local,
			Protected:  variable.Protected,
			Restricted: variable.Restricted,
			IsManaged:  variable.IsManaged,
		}
		data.TemplateVar = append(data.TemplateVar, v)
		data.Count++
	}

	rest.SendJSON(http.StatusOK, w, r, data)
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

	request.Zones, err = cleanZones(request.Zones)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	templateVar, err := db.GetTemplateVar(templateVarId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	templateVar.Name = request.Name
	templateVar.Zones = request.Zones
	templateVar.Local = request.Local
	templateVar.Value = request.Value
	templateVar.Protected = request.Protected
	templateVar.Restricted = request.Restricted
	templateVar.UpdatedUserId = user.Id
	templateVar.UpdatedAt = time.Now().UTC()

	err = db.SaveTemplateVar(templateVar)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipTemplateVar(templateVar)

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

	w.WriteHeader(http.StatusOK)
}

func cleanZones(zones []string) ([]string, error) {
	// Check the zones (max len 64) and remove duplicates and blanks
	zoneSet := make(map[string]struct{})
	cleanZones := make([]string, 0, len(zones))
	for _, zone := range zones {
		zone = strings.Trim(zone, " \r\n")
		if zone == "" {
			continue
		}
		if len(zone) > 64 {
			return nil, fmt.Errorf("zone '%s' exceeds maximum length of 64", zone)
		}
		if _, exists := zoneSet[zone]; !exists {
			zoneSet[zone] = struct{}{}
			cleanZones = append(cleanZones, zone)
		}
	}
	cleanZones = slices.Clip(cleanZones)
	return cleanZones, nil
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

	request.Zones, err = cleanZones(request.Zones)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	templateVar := model.NewTemplateVar(request.Name, request.Zones, request.Local, request.Value, request.Protected, request.Restricted, user.Id)

	err = db.SaveTemplateVar(templateVar)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipTemplateVar(templateVar)

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

	db := database.GetInstance()
	templateVar, err := db.GetTemplateVar(templateVarId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	user := r.Context().Value("user").(*model.User)

	// Delete the template variable
	templateVar.IsDeleted = true
	templateVar.UpdatedAt = time.Now().UTC()
	templateVar.UpdatedUserId = user.Id
	err = db.SaveTemplateVar(templateVar)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipTemplateVar(templateVar)

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

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplateVar(w http.ResponseWriter, r *http.Request) {
	templateVarId := r.PathValue("templatevar_id")

	if !validate.UUID(templateVarId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid variable ID"})
		return
	}

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
		Zones:      templateVar.Zones,
		Local:      templateVar.Local,
		Protected:  templateVar.Protected,
		Restricted: templateVar.Restricted,
		IsManaged:  templateVar.IsManaged,
	}

	rest.SendJSON(http.StatusOK, w, r, data)
}

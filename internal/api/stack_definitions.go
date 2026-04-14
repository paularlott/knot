package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetStackDefinitions(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	filterUserId := r.URL.Query().Get("user_id")

	canManageGlobal := user.HasPermission(model.PermissionManageStackDefinitions)
	canManageOwn := user.HasPermission(model.PermissionManageOwnStackDefinitions) || cfg.LeafNode

	if !canManageGlobal && !canManageOwn {
		rest.WriteResponse(http.StatusOK, w, r, apiclient.StackDefinitionList{Count: 0, Definitions: []apiclient.StackDefinitionInfo{}})
		return
	}

	response := apiclient.StackDefinitionList{
		Count:       0,
		Definitions: []apiclient.StackDefinitionInfo{},
	}

	seen := make(map[string]bool)

	// Fetch global definitions
	if canManageGlobal && filterUserId == "" {
		defs, err := db.GetStackDefinitions()
		if err == nil {
			for _, def := range defs {
				if def.IsDeleted || !def.Active {
					continue
				}
				response.Definitions = append(response.Definitions, stackDefToInfo(def))
				seen[def.Id] = true
				response.Count++
			}
		}
	}

	// Fetch user definitions
	if canManageOwn && (filterUserId == "" || filterUserId == user.Id) {
		defs, err := db.GetStackDefinitionsByUserId(user.Id)
		if err == nil {
			for _, def := range defs {
				if def.IsDeleted || seen[def.Id] {
					continue
				}
				response.Definitions = append(response.Definitions, stackDefToInfo(def))
				seen[def.Id] = true
				response.Count++
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleGetStackDefinition(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	defIdOrName := r.PathValue("stack_definition_id")

	var def *model.StackDefinition
	var err error

	if validate.UUID(defIdOrName) {
		def, err = db.GetStackDefinition(defIdOrName)
	} else {
		def, err = db.GetStackDefinitionByName(defIdOrName, user.Id)
		if def == nil && (user.HasPermission(model.PermissionManageStackDefinitions) || cfg.LeafNode) {
			def, err = db.GetStackDefinitionByName(defIdOrName, "")
		}
	}

	if err != nil || def == nil || def.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Stack definition not found"})
		return
	}

	if !cfg.LeafNode {
		if def.UserId != "" && def.UserId != user.Id && !user.HasPermission(model.PermissionManageStackDefinitions) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Stack definition not found"})
			return
		}
		if def.UserId != "" && def.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnStackDefinitions) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Stack definition not found"})
			return
		}
		if def.UserId == "" && !user.HasPermission(model.PermissionManageStackDefinitions) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Stack definition not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, stackDefToInfo(def))
}

func HandleCreateStackDefinition(w http.ResponseWriter, r *http.Request) {
	request := apiclient.StackDefinitionRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Name == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Name is required"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()

	var ownerUserId string
	isPersonal := request.Scope != "system"

	if !cfg.LeafNode {
		if isPersonal {
			if !user.HasPermission(model.PermissionManageOwnStackDefinitions) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create personal stack definitions"})
				return
			}
			ownerUserId = user.Id
		} else {
			if !user.HasPermission(model.PermissionManageStackDefinitions) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create global stack definitions"})
				return
			}
		}
	} else {
		if isPersonal {
			ownerUserId = user.Id
		}
	}

	// Convert request spaces to model components
	components := make([]model.StackComponent, 0, len(request.Spaces))
	for _, s := range request.Spaces {
		comp := model.StackComponent{
			Name:            s.Name,
			TemplateId:      s.TemplateId,
			Description:     s.Description,
			Shell:           s.Shell,
			StartupScriptId: s.StartupScript,
			DependsOn:       s.DependsOn,
		}
		for _, cf := range s.CustomFields {
			comp.CustomFields = append(comp.CustomFields, model.StackCustomField{Name: cf.Name, Value: cf.Value})
		}
		for _, pf := range s.PortForwards {
			comp.PortForwards = append(comp.PortForwards, model.StackPortForward{ToSpace: pf.ToSpace, LocalPort: int(pf.LocalPort), RemotePort: int(pf.RemotePort)})
		}
		components = append(components, comp)
	}

	def := model.NewStackDefinition(
		request.Name,
		request.Description,
		request.IconURL,
		request.Groups,
		request.Zones,
		true,
		components,
		ownerUserId,
		user.Id,
	)

	db := database.GetInstance()
	err = db.SaveStackDefinition(def, nil)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	sse.PublishStackDefinitionsChanged(def.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventStackDefCreate,
		fmt.Sprintf("Created stack definition %s", def.Name),
		&map[string]interface{}{
			"agent":             r.UserAgent(),
			"IP":                r.RemoteAddr,
			"X-Forwarded-For":   r.Header.Get("X-Forwarded-For"),
			"stack_definition_id": def.Id,
			"stack_definition_name": def.Name,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.StackDefinitionCreateResponse{
		Status: true,
		Id:     def.Id,
	})
}

func HandleUpdateStackDefinition(w http.ResponseWriter, r *http.Request) {
	defIdOrName := r.PathValue("stack_definition_id")
	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	var def *model.StackDefinition
	var err error

	if validate.UUID(defIdOrName) {
		def, err = db.GetStackDefinition(defIdOrName)
	} else {
		def, err = db.GetStackDefinitionByName(defIdOrName, user.Id)
		if def == nil {
			def, err = db.GetStackDefinitionByName(defIdOrName, "")
		}
	}

	if err != nil || def == nil || def.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Stack definition not found"})
		return
	}

	if !cfg.LeafNode {
		if def.UserId != "" && def.UserId != user.Id && !user.HasPermission(model.PermissionManageStackDefinitions) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit this stack definition"})
			return
		}
		if def.UserId != "" && def.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnStackDefinitions) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit own stack definitions"})
			return
		}
		if def.UserId == "" && !user.HasPermission(model.PermissionManageStackDefinitions) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit global stack definitions"})
			return
		}
	}

	request := apiclient.StackDefinitionRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Convert request spaces to model components
	components := make([]model.StackComponent, 0, len(request.Spaces))
	for _, s := range request.Spaces {
		comp := model.StackComponent{
			Name:            s.Name,
			TemplateId:      s.TemplateId,
			Description:     s.Description,
			Shell:           s.Shell,
			StartupScriptId: s.StartupScript,
			DependsOn:       s.DependsOn,
		}
		for _, cf := range s.CustomFields {
			comp.CustomFields = append(comp.CustomFields, model.StackCustomField{Name: cf.Name, Value: cf.Value})
		}
		for _, pf := range s.PortForwards {
			comp.PortForwards = append(comp.PortForwards, model.StackPortForward{ToSpace: pf.ToSpace, LocalPort: int(pf.LocalPort), RemotePort: int(pf.RemotePort)})
		}
		components = append(components, comp)
	}

	def.Name = request.Name
	def.Description = request.Description
	def.IconUrl = request.IconURL
	def.Active = request.Active
	def.Groups = request.Groups
	def.Zones = request.Zones
	def.Components = components
	def.UpdatedUserId = user.Id
	def.UpdatedAt = hlc.Now()

	err = db.SaveStackDefinition(def, nil)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	sse.PublishStackDefinitionsChanged(def.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventStackDefUpdate,
		fmt.Sprintf("Updated stack definition %s", def.Name),
		&map[string]interface{}{
			"agent":               r.UserAgent(),
			"IP":                  r.RemoteAddr,
			"X-Forwarded-For":     r.Header.Get("X-Forwarded-For"),
			"stack_definition_id": def.Id,
			"stack_definition_name": def.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteStackDefinition(w http.ResponseWriter, r *http.Request) {
	defIdOrName := r.PathValue("stack_definition_id")
	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	var def *model.StackDefinition
	var err error

	if validate.UUID(defIdOrName) {
		def, err = db.GetStackDefinition(defIdOrName)
	} else {
		def, err = db.GetStackDefinitionByName(defIdOrName, user.Id)
		if def == nil {
			def, err = db.GetStackDefinitionByName(defIdOrName, "")
		}
	}

	if err != nil || def == nil || def.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Stack definition not found"})
		return
	}

	if !cfg.LeafNode {
		if def.UserId != "" && def.UserId != user.Id && !user.HasPermission(model.PermissionManageStackDefinitions) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete this stack definition"})
			return
		}
		if def.UserId != "" && def.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnStackDefinitions) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete own stack definitions"})
			return
		}
		if def.UserId == "" && !user.HasPermission(model.PermissionManageStackDefinitions) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete global stack definitions"})
			return
		}
	}

	defName := def.Name
	defId := def.Id
	def.IsDeleted = true
	def.Name = def.Id
	def.UpdatedUserId = user.Id
	def.UpdatedAt = hlc.Now()

	err = db.SaveStackDefinition(def, nil)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	sse.PublishStackDefinitionsDeleted(def.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventStackDefDelete,
		fmt.Sprintf("Deleted stack definition %s", defName),
		&map[string]interface{}{
			"agent":               r.UserAgent(),
			"IP":                  r.RemoteAddr,
			"X-Forwarded-For":     r.Header.Get("X-Forwarded-For"),
			"stack_definition_id": defId,
			"stack_definition_name": defName,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleValidateStackDefinition(w http.ResponseWriter, r *http.Request) {
	request := apiclient.StackDefinitionRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	errors := validate.ValidateStackDefinition(&request)

	rest.WriteResponse(http.StatusOK, w, r, apiclient.StackDefinitionValidationResponse{
		Valid:  len(errors) == 0,
		Errors: errors,
	})
}

func stackDefToInfo(def *model.StackDefinition) apiclient.StackDefinitionInfo {
	spaces := make([]apiclient.StackDefSpace, 0, len(def.Components))
	for _, comp := range def.Components {
		s := apiclient.StackDefSpace{
			Name:          comp.Name,
			TemplateId:    comp.TemplateId,
			Description:   comp.Description,
			Shell:         comp.Shell,
			StartupScript: comp.StartupScriptId,
			DependsOn:     comp.DependsOn,
		}
		for _, cf := range comp.CustomFields {
			s.CustomFields = append(s.CustomFields, apiclient.StackDefCustomField{Name: cf.Name, Value: cf.Value})
		}
		for _, pf := range comp.PortForwards {
			s.PortForwards = append(s.PortForwards, apiclient.StackDefPortForward{ToSpace: pf.ToSpace, LocalPort: uint16(pf.LocalPort), RemotePort: uint16(pf.RemotePort)})
		}
		spaces = append(spaces, s)
	}

	scope := "system"
	if def.UserId != "" {
		scope = "personal"
	}

	return apiclient.StackDefinitionInfo{
		Id:          def.Id,
		UserId:      def.UserId,
		Name:        def.Name,
		Description: def.Description,
		IconURL:     def.IconUrl,
		Active:      def.Active,
		Scope:       scope,
		Groups:      def.Groups,
		Zones:       def.Zones,
		Spaces:      spaces,
		IsManaged:   def.IsManaged,
	}
}

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetPools(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pools, err := service.GetPoolService().List(user)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, apiclient.PoolList{Count: len(pools), Pools: pools})
}

func HandleGetPool(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	// ResolveForUser already scopes the pool to the requesting user.
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	info, err := service.GetPoolService().Info(pool, user)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, info)
}

func HandleCreatePool(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	request := apiclient.PoolRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if request.DesiredCount < 1 {
		request.DesiredCount = 1
	}
	pool := model.NewPoolDefinition(request.Name, request.TemplateId, request.StartupScriptId, request.DesiredCount, user.Id)
	pool.Active = request.Active
	pool.LeaseMaxTime = request.LeaseMaxTime
	pool.LeaseMaxExtensions = request.LeaseMaxExtensions
	customFields, status, err := poolCustomFields(r, user, request.TemplateId, request.CustomFields, nil)
	if err != nil {
		rest.WriteResponse(status, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	pool.CustomFields = customFields
	if err := service.GetPoolService().Create(pool, user); err != nil {
		// Creation can partially succeed (definition saved, a member failed).
		// Distinguish that from plain rejection — e.g. a bridged KVM
		// template — by checking the definition actually persisted; the
		// model assigns an ID up front, so pool.Id alone proves nothing.
		if saved, resolveErr := service.GetPoolService().Resolve(pool.Id); resolveErr == nil && saved != nil && !saved.IsDeleted {
			rest.WriteResponse(http.StatusCreated, w, r, apiclient.PoolCreateResponse{Status: true, Id: pool.Id, Message: err.Error()})
		} else {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}
	rest.WriteResponse(http.StatusCreated, w, r, apiclient.PoolCreateResponse{Status: true, Id: pool.Id})
}

func HandleUpdatePool(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	request := apiclient.PoolUpdateRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.StartupScriptId != nil && *request.StartupScriptId != pool.StartupScriptId {
		if pool.Active {
			rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{Error: "Stop the pool before changing the startup script"})
			return
		}
		if err := service.GetPoolService().UpdateStartupScript(pool, *request.StartupScriptId, user); err != nil {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	if request.DesiredCount != nil && *request.DesiredCount != pool.DesiredCount {
		if err := service.GetPoolService().SetSize(pool, *request.DesiredCount, user); err != nil {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	if request.Active != nil && *request.Active != pool.Active {
		if *request.Active {
			if err := service.GetPoolService().Start(pool, user); err != nil {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		} else {
			if err := service.GetPoolService().Stop(pool, user); err != nil {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		}
	}

	if request.CustomFields != nil {
		customFields, status, err := poolCustomFields(r, user, pool.TemplateId, *request.CustomFields, pool.CustomFields)
		if err != nil {
			rest.WriteResponse(status, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if !slices.Equal(customFields, pool.CustomFields) {
			if err := service.GetPoolService().SaveCustomFields(pool, customFields, user); err != nil {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		}
	}

	leaseConfigChanged := false
	if request.LeaseMaxTime != nil && *request.LeaseMaxTime != pool.LeaseMaxTime {
		pool.LeaseMaxTime = *request.LeaseMaxTime
		leaseConfigChanged = true
	}
	if request.LeaseMaxExtensions != nil && *request.LeaseMaxExtensions != pool.LeaseMaxExtensions {
		pool.LeaseMaxExtensions = *request.LeaseMaxExtensions
		leaseConfigChanged = true
	}
	if leaseConfigChanged {
		pool.UpdatedUserId = user.Id
		pool.UpdatedAt = hlc.Now()
		if err := service.GetPoolService().SaveLeaseConfig(pool); err != nil {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// poolCustomFields validates the custom field values for a pool's members
// against its template and returns them as stored on the pool: as given,
// without template defaults, so a later change to a default reaches the
// members. previous holds the pool's current values; select options are
// not re-checked for values it already has.
func poolCustomFields(r *http.Request, user *model.User, templateId string, values []apiclient.CustomFieldValue, previous []model.SpaceCustomField) ([]model.SpaceCustomField, int, error) {
	template, err := database.GetInstance().GetTemplate(templateId)
	if err != nil || template == nil {
		return nil, http.StatusBadRequest, errors.New("template not found")
	}
	return validatePoolCustomFields(r.Context(), user, template, values, previous, pluginFieldOptionKeys)
}

func validatePoolCustomFields(ctx context.Context, user *model.User, template *model.Template, values []apiclient.CustomFieldValue, previous []model.SpaceCustomField, fetchKeys func(context.Context, *model.User, model.TemplateCustomField) ([]string, error)) ([]model.SpaceCustomField, int, error) {
	known := make(map[string]bool, len(template.CustomFields))
	for _, field := range template.CustomFields {
		known[field.Name] = true
	}
	fields := []model.SpaceCustomField{}
	index := make(map[string]int, len(values))
	var unknown []string
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		if !known[name] {
			unknown = append(unknown, name)
			continue
		}
		if i, ok := index[name]; ok {
			fields[i].Value = value.Value
			continue
		}
		index[name] = len(fields)
		fields = append(fields, model.SpaceCustomField{Name: name, Value: value.Value})
	}
	if len(unknown) > 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("template has no custom field(s): %s", strings.Join(unknown, ", "))
	}

	// Validate what members will carry: the values plus template defaults.
	withDefaults := model.ApplyCustomFieldDefaults(template, slices.Clone(fields))
	if missing := model.MissingRequiredCustomFields(template, withDefaults); len(missing) > 0 {
		missing = describeMissingRequired(ctx, user, template, missing, fetchKeys)
		return nil, http.StatusBadRequest, errors.New("missing required custom field(s): " + strings.Join(missing, ", "))
	}
	previousValues := make(map[string]string, len(previous))
	for _, field := range previous {
		previousValues[field.Name] = field.Value
	}
	if invalid, err := invalidCustomFieldOptions(ctx, user, template, withDefaults, previousValues, fetchKeys); err != nil {
		return nil, http.StatusInternalServerError, err
	} else if len(invalid) > 0 {
		return nil, http.StatusBadRequest, errors.New("invalid value for custom field(s): " + strings.Join(invalid, ", "))
	}
	return fields, http.StatusOK, nil
}

func HandleDeletePool(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	if err := service.GetPoolService().Delete(pool, user); err != nil {
		rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
}

func HandleSetPoolSize(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	request := apiclient.PoolSetSizeRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if err := service.GetPoolService().SetSize(pool, request.DesiredCount, user); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
}

func HandlePoolStart(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	if err := service.GetPoolService().Start(pool, user); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
}

func HandlePoolStop(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	if err := service.GetPoolService().Stop(pool, user); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeLeaseError maps lease lifecycle errors onto status codes.
func writeLeaseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrPoolExhausted), errors.Is(err, service.ErrLeaseExtendLimit):
		rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrLeaseNotHolder):
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrLeaseNotFound):
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
	default:
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
	}
}

func HandlePoolAcquire(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	request := apiclient.PoolLeaseAcquireRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	lease, err := service.GetPoolService().Acquire(r.Context(), pool, user, request.DurationSeconds, request.WaitSeconds)
	if err != nil {
		writeLeaseError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, lease)
}

func HandleGetPoolLeases(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	pool, err := service.GetPoolService().ResolveForUser(r.PathValue("id_or_name"), user)
	if err != nil || pool == nil || pool.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Pool not found"})
		return
	}
	leases, err := service.GetPoolService().ListLeases(pool, user)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, apiclient.PoolLeaseList{Count: len(leases), Leases: leases})
}

func HandleSpaceLeaseExtend(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	request := apiclient.PoolLeaseExtendRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	lease, err := service.GetPoolService().Extend(user, r.PathValue("space_id_or_name"), request.DurationSeconds)
	if err != nil {
		writeLeaseError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, lease)
}

func HandleSpaceLeaseRelease(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	var lease *apiclient.LeaseInfo
	var err error
	if r.URL.Query().Get("destroy") == "true" {
		lease, err = service.GetPoolService().ReleaseAndDestroy(user, r.PathValue("space_id_or_name"))
	} else {
		lease, err = service.GetPoolService().Release(user, r.PathValue("space_id_or_name"))
	}
	if err != nil {
		writeLeaseError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, lease)
}

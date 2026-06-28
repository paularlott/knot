package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
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
	if err := service.GetPoolService().Create(pool, user); err != nil {
		if pool.Id != "" && !pool.IsDeleted {
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

	w.WriteHeader(http.StatusOK)
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

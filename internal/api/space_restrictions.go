package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

// restrictionsSpace finds the space named in the path, by id or name, that
// the caller may restrict: theirs, one shared with them, or any with the
// manage spaces permission — the same as starting and stopping it.
func restrictionsSpace(w http.ResponseWriter, r *http.Request) (*model.Space, *model.User, bool) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil || space == nil || space.IsDeleted ||
		(space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces)) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return nil, nil, false
	}
	return space, user, true
}

// spaceOwnerPrincipal is the file storage principal of the space's owner,
// as the space itself sees file storage: limited by its restrictions.
func spaceOwnerPrincipal(space *model.Space) *filestore.Principal {
	p, ok := filestore.PrincipalFor(spaceOwner(space))
	if !ok {
		return nil
	}
	limitToSpace(p, space)
	return p
}

// restrictionsResponse reports a space's restrictions, with the buckets it
// is limited to under their current names, as the space's owner sees them.
func restrictionsResponse(space *model.Space) *apiclient.SpaceRestrictions {
	res := space.Restrictions
	out := &apiclient.SpaceRestrictions{
		SpaceId:      space.Id,
		SpaceName:    space.Name,
		Restricted:   res.Active,
		NoSecrets:    res.NoSecrets,
		NoCli:        res.NoCli,
		Files:        res.Files,
		LimitBuckets: res.LimitBuckets,
		Buckets:      []apiclient.SpaceRestrictBucket{},
	}
	if space.TemplateId != "" {
		if template, err := database.GetInstance().GetTemplate(space.TemplateId); err == nil && template != nil {
			out.TemplateNoFiles = template.DisableFileStorage
			out.TemplateNoSecrets = template.DisableUserSecrets
		}
	}
	if res.LimitBuckets {
		store := filestore.Get()
		owner, _ := filestore.PrincipalFor(spaceOwner(space))
		for _, g := range res.Buckets {
			name := g.Name
			if store != nil {
				current, ok := store.BucketName(g.Id)
				if !ok {
					continue // deleted since
				}
				name = current
			}
			out.Buckets = append(out.Buckets, apiclient.SpaceRestrictBucket{Name: filestore.DisplayName(owner, name), ReadOnly: g.ReadOnly})
		}
	}
	return out
}

func spaceOwner(space *model.Space) *model.User {
	owner, err := database.GetInstance().GetUser(space.UserId)
	if err != nil {
		return nil
	}
	return owner
}

// HandleGetSpaceRestrictions reports a space's restrictions.
func HandleGetSpaceRestrictions(w http.ResponseWriter, r *http.Request) {
	space, _, ok := restrictionsSpace(w, r)
	if !ok {
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, restrictionsResponse(space))
}

// HandleRestrictSpace adds restrictions to a space. They only ever tighten
// what the space may do, so a space may restrict itself, and are cleared
// when the space next starts.
func HandleRestrictSpace(w http.ResponseWriter, r *http.Request) {
	space, user, ok := restrictionsSpace(w, r)
	if !ok {
		return
	}

	request := apiclient.SpaceRestrictRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	request.Files = strings.ToLower(strings.TrimSpace(request.Files))
	if !model.ValidSpaceFilesMode(request.Files) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("unknown file storage mode %q, use ro or none", request.Files)})
		return
	}

	// Serialise changes to the space so concurrent restrictions all apply.
	if transport := service.GetTransport(); transport != nil {
		unlockToken := transport.LockResource(space.Id)
		if unlockToken == "" {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock space"})
			return
		}
		defer transport.UnlockResource(space.Id, unlockToken)
	}
	db := database.GetInstance()
	space, err := db.GetSpace(space.Id)
	if err != nil || space == nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	more := model.SpaceRestrictions{NoSecrets: request.NoSecrets, Files: request.Files, NoCli: request.NoCli}
	if len(request.Buckets) > 0 {
		grants, err := resolveRestrictBuckets(space, request.Buckets)
		if err != nil {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		more.LimitBuckets, more.Buckets = true, grants
	}

	space.Restrictions = space.Restrictions.Narrow(more)
	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"Restrictions", "UpdatedAt"}); err != nil {
		log.WithError(err).Error("HandleRestrictSpace:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "failed to save space"})
		return
	}
	publishSpaceRestrictions(space)

	response := restrictionsResponse(space)
	auditRestrictions(r, user, space, fmt.Sprintf("Restricted space %s", space.Name), response)
	rest.WriteResponse(http.StatusOK, w, r, response)
}

// HandleClearSpaceRestrictions lifts a space's restrictions. Only from
// outside a space: a space can't lift its own, or any other space's.
func HandleClearSpaceRestrictions(w http.ResponseWriter, r *http.Request) {
	if fromSpace, _ := r.Context().Value("space_id").(string); fromSpace != "" {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "restrictions can't be lifted from inside a space, restart the space to lift them"})
		return
	}
	space, user, ok := restrictionsSpace(w, r)
	if !ok {
		return
	}

	if transport := service.GetTransport(); transport != nil {
		unlockToken := transport.LockResource(space.Id)
		if unlockToken == "" {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock space"})
			return
		}
		defer transport.UnlockResource(space.Id, unlockToken)
	}
	db := database.GetInstance()
	space, err := db.GetSpace(space.Id)
	if err != nil || space == nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	if space.Restrictions.Active {
		space.Restrictions = model.SpaceRestrictions{}
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"Restrictions", "UpdatedAt"}); err != nil {
			log.WithError(err).Error("HandleClearSpaceRestrictions:")
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "failed to save space"})
			return
		}
		publishSpaceRestrictions(space)
		auditRestrictions(r, user, space, fmt.Sprintf("Lifted the restrictions on space %s", space.Name), nil)
	}
	rest.WriteResponse(http.StatusOK, w, r, restrictionsResponse(space))
}

// resolveRestrictBuckets turns the bucket names a space is limited to into
// grants by bucket id, so they follow the buckets through renames. Names
// are resolved as the space sees file storage, so a bucket it can no longer
// reach is unknown to it.
func resolveRestrictBuckets(space *model.Space, buckets []apiclient.SpaceRestrictBucket) ([]model.SpaceBucketGrant, error) {
	store := filestore.Get()
	if store == nil {
		return nil, filestore.ErrDisabled
	}
	p := spaceOwnerPrincipal(space)
	if p == nil {
		return nil, fmt.Errorf("the space's owner can't use file storage")
	}

	grants := []model.SpaceBucketGrant{}
	for _, b := range buckets {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			return nil, fmt.Errorf("a bucket name is blank")
		}
		info, err := store.GetBucket(p, filestore.ResolveName(p, name))
		if err != nil {
			return nil, fmt.Errorf("no bucket named %q", name)
		}
		// Named twice: read-only if either says so.
		if i := slices.IndexFunc(grants, func(g model.SpaceBucketGrant) bool { return g.Id == info.Id }); i >= 0 {
			grants[i].ReadOnly = grants[i].ReadOnly || b.ReadOnly
			continue
		}
		grants = append(grants, model.SpaceBucketGrant{Id: info.Id, Name: info.Name, ReadOnly: b.ReadOnly})
	}
	return grants, nil
}

func publishSpaceRestrictions(space *model.Space) {
	agent_server.PushRestrictions(space)
	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)
}

func auditRestrictions(r *http.Request, user *model.User, space *model.Space, details string, res *apiclient.SpaceRestrictions) {
	properties := map[string]interface{}{
		"space_id":   space.Id,
		"space_name": space.Name,
	}
	if fromSpace, _ := r.Context().Value("space_id").(string); fromSpace != "" {
		properties["from_space_id"] = fromSpace
	}
	if res != nil {
		properties["no_secrets"] = res.NoSecrets
		properties["no_cli"] = res.NoCli
		properties["files"] = res.Files
		if res.LimitBuckets {
			names := make([]string, 0, len(res.Buckets))
			for _, b := range res.Buckets {
				if b.ReadOnly {
					names = append(names, b.Name+":ro")
				} else {
					names = append(names, b.Name)
				}
			}
			properties["buckets"] = strings.Join(names, ", ")
		}
	}
	audit.LogWithRequest(r, user.Username, model.AuditActorTypeUser, model.AuditEventSpaceRestrict, details, &properties)
}

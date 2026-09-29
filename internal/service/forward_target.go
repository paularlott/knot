package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"
)

// ForwardTargetError maps a failed resolution to the HTTP response the
// proxy should send. The message is safe to return to the client and is
// what the forwarding agent logs on a failed dial.
type ForwardTargetError struct {
	Status  int
	Message string
}

func (e *ForwardTargetError) Error() string { return e.Message }

// ForwardTarget is a resolved port-forward target.
type ForwardTarget struct {
	// Space is the space traffic should reach — the target itself for
	// space targets, or the pool member picked for this connection (pools
	// re-resolve per connection, round-robin). Nil for pool targets
	// resolved by AuthorizeForwardTarget, which doesn't need a member.
	Space *model.Space
	// Ref is the canonical dial reference for the target: a bare space or
	// pool name when the owner matches the dialing identity, otherwise
	// user--name. Agents dial /proxy/spaces/{ref}/port/{port}.
	Ref string
	// IsPool reports whether the target resolved to a pool (the ref names
	// the pool, not the member).
	IsPool bool
	// OwnerId is the user id the target belongs to — the pool's creator for
	// pool targets, the space's owner otherwise.
	OwnerId string
}

// SplitForwardRef splits a qualified forward target reference into its
// username and space-or-pool name parts. The second return is false for
// bare names (own spaces and pools) and for malformed refs.
func SplitForwardRef(ref string) (string, string, bool) {
	user, name, found := strings.Cut(ref, "--")
	if !found {
		return "", "", false
	}
	// Usernames and names never contain "--" themselves, so the first cut
	// is the only valid one; validate both parts to keep garbage out.
	if !validate.Username(user) || !validate.Name(name) {
		return "", "", false
	}
	return user, name, true
}

// ValidForwardRef reports whether a forward target reference is well
// formed: a space UUID (as stored by older forward entries), a bare name,
// or a qualified user--name.
func ValidForwardRef(ref string) bool {
	if validate.UUID(ref) {
		return true
	}
	if strings.Contains(ref, "--") {
		_, _, ok := SplitForwardRef(ref)
		return ok
	}
	return validate.Name(ref)
}

// AuthorizeForwardTarget resolves a port-forward target reference for a
// requesting user and port, and applies the access rule — but without
// requiring a live pool member. Use this when creating or persisting a
// forward (an empty pool now may have members by the time it dials):
//
//   - a bare name resolves to the requester's own space by that name,
//     falling back to the requester's own pool;
//   - user--name resolves to that user's space, falling back to their pool;
//   - a space UUID resolves to the space directly (the form older stored
//     forward entries use).
//
// The owner may reach any port. Anyone else may only reach a port the
// target's template declares public — checked per connection, so template
// edits apply to running spaces immediately. Pool members all come from the
// pool's template, so the public check covers every member at once.
//
// The returned target carries Space for space targets and Ref/IsPool for
// both; Space is nil for pool targets.
func AuthorizeForwardTarget(requester *model.User, ref string, port uint16) (*ForwardTarget, *ForwardTargetError) {
	db := database.GetInstance()

	// UUID form: stored forward entries from before qualified refs existed.
	if validate.UUID(ref) {
		space, err := db.GetSpace(ref)
		if err != nil || space == nil || space.IsDeleted {
			return nil, &ForwardTargetError{http.StatusNotFound, "target space not found"}
		}
		if ferr := authorizeForwardPort(requester, space, port); ferr != nil {
			return nil, ferr
		}
		return &ForwardTarget{Space: space, Ref: ref, OwnerId: space.UserId}, nil
	}

	targetUserId := requester.Id
	targetUsername := requester.Username
	qualified := false
	if user, name, ok := SplitForwardRef(ref); ok {
		targetUser, err := db.GetUserByUsername(user)
		if err != nil || targetUser == nil {
			return nil, &ForwardTargetError{http.StatusNotFound, "target user not found"}
		}
		targetUserId = targetUser.Id
		targetUsername = targetUser.Username
		qualified = true
		ref = name
	} else if !validate.Name(ref) {
		return nil, &ForwardTargetError{http.StatusBadRequest, "invalid target reference"}
	}

	// Space by name first — a space shadows a pool of the same name, same
	// as the web port routing.
	space, err := db.GetSpaceByName(targetUserId, ref)
	if err == nil && space != nil {
		if ferr := authorizeForwardPort(requester, space, port); ferr != nil {
			return nil, ferr
		}
		dialRef := space.Name
		if qualified {
			dialRef = targetUsername + "--" + space.Name
		}
		return &ForwardTarget{Space: space, Ref: dialRef, OwnerId: space.UserId}, nil
	}

	// Pool by name.
	pool, err := db.GetPoolDefinitionByName(targetUserId, ref)
	if err != nil || pool == nil || pool.IsDeleted {
		return nil, &ForwardTargetError{http.StatusNotFound, "target space or pool not found"}
	}
	if targetUserId != requester.Id {
		template, err := db.GetTemplate(pool.TemplateId)
		if err != nil || template == nil || !template.IsPortPublic(port) {
			return nil, &ForwardTargetError{http.StatusForbidden, fmt.Sprintf("port %d is not public on pool %s", port, ref)}
		}
	}

	dialRef := pool.Name
	if qualified {
		dialRef = targetUsername + "--" + pool.Name
	}
	return &ForwardTarget{Ref: dialRef, IsPool: true, OwnerId: pool.CreatedUserId}, nil
}

// ResolveForwardTarget authorizes the target and, for pools, picks a live
// member for this connection. Distinguishing "no such pool" from "pool has
// no free member" keeps the common transient state from looking like a
// permission failure.
func ResolveForwardTarget(requester *model.User, ref string, port uint16) (*ForwardTarget, *ForwardTargetError) {
	target, ferr := AuthorizeForwardTarget(requester, ref, port)
	if ferr != nil {
		return nil, ferr
	}
	if !target.IsPool {
		return target, nil
	}

	member := GetPoolService().PickMemberForRouting(poolNameFromRef(target.Ref), target.OwnerId)
	if member == nil {
		return nil, &ForwardTargetError{http.StatusServiceUnavailable, fmt.Sprintf("pool %s has no available members", poolNameFromRef(target.Ref))}
	}
	target.Space = member
	return target, nil
}

// poolNameFromRef strips the user-- qualifier from a pool dial ref.
func poolNameFromRef(ref string) string {
	if _, name, ok := SplitForwardRef(ref); ok {
		return name
	}
	return ref
}

// authorizeForwardPort applies the single access rule: owners reach any
// port, everyone else only ports the space's template declares public.
func authorizeForwardPort(requester *model.User, space *model.Space, port uint16) *ForwardTargetError {
	if space.UserId == requester.Id {
		return nil
	}
	template, err := database.GetInstance().GetTemplate(space.TemplateId)
	if err != nil || template == nil || !template.IsPortPublic(port) {
		return &ForwardTargetError{http.StatusForbidden, fmt.Sprintf("port %d is not public on space %s", port, space.Name)}
	}
	return nil
}

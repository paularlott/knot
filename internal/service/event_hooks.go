package service

import (
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// CheckSpaceLifecycleEvents compares the before/after state of a space and
// raises the appropriate system lifecycle events for any transition detected.
// Pass a nil oldSpace with a non-nil newSpace to signal creation.
func CheckSpaceLifecycleEvents(oldSpace, newSpace *model.Space) {
	if oldSpace == nil && newSpace != nil {
		RaiseSystemEvent("space.created", newSpace.Id, newSpace.UserId, map[string]interface{}{
			"space_name":        newSpace.Name,
			"space_id":          newSpace.Id,
			"space_urls":        resolveSpaceURLs(newSpace),
			"template_id":       newSpace.TemplateId,
			"startup_script_id": newSpace.StartupScriptId,
		})
		return
	}

	if oldSpace == nil || newSpace == nil {
		return
	}

	if !oldSpace.IsDeleted && newSpace.IsDeleted {
		RaiseSystemEvent("space.deleted", newSpace.Id, newSpace.UserId, map[string]interface{}{
			"space_name": newSpace.Name,
			"space_id":   newSpace.Id,
			"deleted_at": newSpace.UpdatedAt.Time().UTC().Format(time.RFC3339Nano),
		})
		return
	}

	oldStarted := oldSpace.IsDeployed && !oldSpace.IsPending
	newStarted := newSpace.IsDeployed && !newSpace.IsPending
	if !oldStarted && newStarted {
		RaiseSystemEvent("space.started", newSpace.Id, newSpace.UserId, map[string]interface{}{
			"space_name": newSpace.Name,
			"space_id":   newSpace.Id,
			"space_urls": resolveSpaceURLs(newSpace),
			"node_id":    newSpace.NodeId,
			"started_at": newSpace.StartedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	oldStopped := !oldSpace.IsDeployed && !oldSpace.IsPending
	newStopped := !newSpace.IsDeployed && !newSpace.IsPending
	if !oldStopped && newStopped && !newSpace.IsDeleted {
		RaiseSystemEvent("space.stopped", newSpace.Id, newSpace.UserId, map[string]interface{}{
			"space_name": newSpace.Name,
			"space_id":   newSpace.Id,
			"stopped_at": newSpace.UpdatedAt.Time().UTC().Format(time.RFC3339Nano),
		})
	}
}

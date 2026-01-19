package service

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// ResolveScriptByName resolves a script by name with user override support
// First checks user scripts, then falls back to global scripts
// Returns nil if script not found, deleted, inactive, or not valid for current zone
// Applies variable replacement to global scripts only
// Supports zone-specific overrides - returns the best match for the current zone
func ResolveScriptByName(name string, userId string) (*model.Script, error) {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	// Try user script first
	if userId != "" {
		scripts, err := db.GetScriptsByNameAndUser(name, userId)
		if err == nil {
			// Filter by zone and return first valid match
			for _, script := range scripts {
				if !script.IsDeleted && script.Active && script.IsValidForZone(cfg.Zone) {
					return script, nil
				}
			}
		}
	}

	// Fall back to global script
	scripts, err := db.GetScriptsByName(name)
	if err != nil {
		return nil, fmt.Errorf("script not found")
	}

	// Filter by zone and return first valid match
	for _, script := range scripts {
		if script.IsDeleted || !script.Active {
			continue
		}

		if !script.IsValidForZone(cfg.Zone) {
			continue
		}

		// Apply variable replacement to global scripts
		if script.IsGlobalScript() {
			variables, err := db.GetTemplateVars()
			if err == nil {
				vars := model.FilterVars(variables)
				content, err := model.ApplyVariablesToScript(script, vars)
				if err == nil {
					script.Content = content
				}
			}
		}

		return script, nil
	}

	return nil, fmt.Errorf("script not found")
}

// CanUserExecuteScript checks if a user has permission to execute a script
func CanUserExecuteScript(user *model.User, script *model.Script) bool {
	if script.IsUserScript() {
		// User script: only the owner can execute their own scripts
		// Admins with ManageScripts can also execute user scripts for management purposes
		if script.UserId == user.Id {
			return user.HasPermission(model.PermissionExecuteOwnScripts)
		}
		// Non-owners cannot execute other users' scripts
		return false
	}

	// Global script: need ExecuteScripts permission
	if !user.HasPermission(model.PermissionExecuteScripts) {
		return false
	}

	// Check group membership for non-admin users
	if !user.HasPermission(model.PermissionManageScripts) {
		if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
			return false
		}
	}

	return true
}


// ApplyVariablesToScriptIfGlobal applies variable replacement to a script if it's global
func ApplyVariablesToScriptIfGlobal(script *model.Script, db database.DbDriver) {
	if script.IsGlobalScript() {
		variables, err := db.GetTemplateVars()
		if err == nil {
			vars := model.FilterVars(variables)
			content, err := model.ApplyVariablesToScript(script, vars)
			if err == nil {
				script.Content = content
			}
		}
	}
}

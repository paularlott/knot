package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/scriptling"
)

type ScriptService struct{}

type ScriptListOptions struct {
	FilterUserId          string
	User                  *model.User
	IncludeDeleted        bool
	CheckZoneRestriction  bool
}

var scriptService *ScriptService

func GetScriptService() *ScriptService {
	if scriptService == nil {
		scriptService = &ScriptService{}
	}
	return scriptService
}

// ListScripts returns a filtered list of scripts based on the provided options
func (s *ScriptService) ListScripts(opts ScriptListOptions) ([]*model.Script, error) {
	db := database.GetInstance()
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, fmt.Errorf("failed to get scripts: %v", err)
	}

	cfg := config.GetServerConfig()
	var result []*model.Script

	for _, script := range scripts {
		// Skip deleted scripts unless explicitly requested
		if script.IsDeleted && !opts.IncludeDeleted {
			continue
		}

		// Determine if requesting user scripts or global scripts
		isUserScripts := opts.FilterUserId != ""

		// Filter by user_id
		if isUserScripts {
			if script.UserId != opts.FilterUserId {
				continue
			}
		} else {
			// Global scripts only (empty UserId)
			if script.UserId != "" {
				continue
			}

			// Check group permissions for non-admin users
			if opts.User != nil && !opts.User.HasPermission(model.PermissionManageScripts) {
				if len(script.Groups) > 0 && !opts.User.HasAnyGroup(&script.Groups) {
					continue
				}
			}
		}

		// Check zone restrictions if required
		if opts.CheckZoneRestriction && !script.IsValidForZone(cfg.Zone) {
			continue
		}

		result = append(result, script)
	}

	return result, nil
}

func ExecuteScriptWithMCP(script *model.Script, mcpParams map[string]string, user *model.User, client *apiclient.ApiClient) (string, error) {
	timeout := time.Duration(script.Timeout) * time.Second
	if script.Timeout == 0 {
		timeout = 300 * time.Second // 5 minutes to allow for AI operations with tool calling
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ctx = context.WithValue(ctx, "user", user)

	env, err := NewMCPScriptlingEnv(client, mcpParams, user)
	if err != nil {
		return "", fmt.Errorf("failed to create scriptling environment: %v", err)
	}

	// Register MCP library with parameters and tool access
	mcpLib := scriptling.GetMCPLibrary(mcpParams, GetOpenAIClient())
	env.RegisterLibrary("knot.mcp", mcpLib)

	result, err := env.EvalWithContext(ctx, script.Content)
	if err != nil {
		return "", fmt.Errorf("script execution failed: %v", err)
	}

	output := env.GetOutput()
	if result != nil && result.Inspect() != "None" {
		if output != "" {
			output += "\n"
		}
		output += result.Inspect()
	}

	return strings.TrimRight(output, "\n"), nil
}

// ExecuteScriptLocally is deprecated - scripts should not run on the server
// This function now returns an error to prevent security risks
func ExecuteScriptLocally(script *model.Script, args []string) (string, error) {
	return "", fmt.Errorf("script execution requires an active agent connection")
}

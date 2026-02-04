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
	"github.com/paularlott/scriptling/extlibs"
)

type ScriptService struct{}

type ScriptListOptions struct {
	FilterUserId         string
	User                 *model.User
	IncludeDeleted       bool
	CheckZoneRestriction bool
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

func ExecuteScriptWithMCP(script *model.Script, mcpParams map[string]string, user *model.User) (string, error) {
	timeout := time.Duration(config.GetServerConfig().MCPToolTimeout) * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ctx = context.WithValue(ctx, "user", user)

	// Use MuxClient for direct API calls
	client := apiclient.NewMuxClient(user)

	env, mcpLib, err := NewMCPScriptlingEnv(client, mcpParams, user)
	if err != nil {
		return "", fmt.Errorf("failed to create scriptling environment: %v", err)
	}

	result, err := env.EvalWithContext(ctx, script.Content)
	if err != nil {
		// Check for SystemExit
		if sysExit, ok := extlibs.GetSysExitCode(err); ok {
			if storedResult := mcpLib.GetResult(); storedResult != nil {
				if sysExit.Code == 0 {
					// Success - return the stored result
					return *storedResult, nil
				}
				// Error case - return with MCP_TOOL_ERROR prefix intact
				return "", fmt.Errorf("%s", *storedResult)
			}
			// No result stored
			if sysExit.Code == 0 {
				return "", nil
			}
			return "", fmt.Errorf("script exited with code %d", sysExit.Code)
		}
		// Other errors
		return "", err
	}

	// Check if mcp.return_* was called (result stored without SystemExit)
	if storedResult := mcpLib.GetResult(); storedResult != nil {
		return *storedResult, nil
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

package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/mcp"
)

func InitializeMCPServer(routes *http.ServeMux) *mcp.Server {
	// Create a new MCP server instance
	server := mcp.NewServer("knot-mcp-server", build.Version)
	routes.HandleFunc("POST /mcp", middleware.ApiAuth(server.HandleRequest))

	// Register tools
	server.RegisterTool(
		mcp.NewTool("list_spaces", "List all spaces for a user or all users"),
		listTools,
	)

	server.RegisterTool(
		mcp.NewTool("start_space", "Start a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to start", true),
		startSpace,
	)

	server.RegisterTool(
		mcp.NewTool("stop_space", "Stop a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to stop", true),
		stopSpace,
	)

	server.RegisterTool(
		mcp.NewTool("get_docker_podman_spec", "Get the complete Docker/Podman job specification documentation in markdown format"),
		getContainerSpec,
	)

	return server
}

func listTools(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()

	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get spaces: %v", err)
	}

	var spaceInfos []SpaceInfo
	for _, space := range spaces {
		spaceInfo := SpaceInfo{
			SpaceID:     space.Id,
			Name:        space.Name,
			Description: space.Description,
			IsDeployed:  space.IsDeployed,
			IsPending:   space.IsPending,
			IsDeleting:  space.IsDeleting,
			Zone:        space.Zone,
			UserID:      space.UserId,
		}

		// Get username
		if spaceUser, err := db.GetUser(space.UserId); err == nil {
			spaceInfo.Username = spaceUser.Username
		}

		spaceInfos = append(spaceInfos, spaceInfo)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Found %d spaces:\n%s", len(spaceInfos), formatSpacesList(spaceInfos))), nil
}

func startSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaceID, err := req.String("space_id")
	if err != nil || spaceID == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check if user has permission to start this space
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to start this space")
	}

	// Get the templates
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return nil, fmt.Errorf("Failed to get template: %v", err)
	}

	// Use the container service to start the space
	containerService := service.GetContainerService()
	err = containerService.StartSpace(space, template, user)
	if err != nil {
		return nil, fmt.Errorf("Failed to start space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' (%s) is starting", space.Name, spaceID)), nil
}

func stopSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaceID, err := req.String("space_id")
	if err != nil || spaceID == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check if user has permission to stop this space
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to stop this space")
	}

	// Use the container service to stop the space
	containerService := service.GetContainerService()
	err = containerService.StopSpace(space)
	if err != nil {
		return nil, fmt.Errorf("Failed to stop space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' (%s) is stopping", space.Name, spaceID)), nil
}

func formatSpacesList(spaces []SpaceInfo) string {
	if len(spaces) == 0 {
		return "No spaces found."
	}

	var builder strings.Builder
	for _, space := range spaces {
		status := "stopped"
		if space.IsDeleting {
			status = "deleting"
		} else if space.IsPending {
			status = "pending"
		} else if space.IsDeployed {
			status = "running"
		}

		builder.WriteString(fmt.Sprintf("- Name: %s, ID: %s, Status: %s, Description: %s\n",
			space.Name, space.SpaceID, status, space.Description))
	}

	return builder.String()
}

func getContainerSpec(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	return mcp.NewToolResponseText(getDockerPodmanJobSpecContent()), nil
}

func getDockerPodmanJobSpecContent() string {
	return `# Docker / Podman Job Specification for Knot

Docker/Podman job specification, showcasing all available options:

` + "```yaml" + `
container_name: <container name>
hostname: <host name>
image: "<container image>"
auth:
  username: <username>
  password: <password>
ports:
  - <host port>:<container port>/<transport>
volumes:
  - <host path>:<container path>
command: [
  "<1>",
  "<2>"
]
privileged: <true | false>
network: <network mode>
environment:
  - "<variable>=<value>"
cap_add:
  - <cap>
cap_drop:
  - <cap>
devices:
  - <host path>:<container path>
dns:
  - <nameserver ip>
add_host:
  - <host name>:<ip>
dns_search:
  - <domain name>
` + "```" + `

---

## Job Specification Details

### **container_name**
The unique name assigned to the container. Ensure it does not conflict with other containers on the host.

### **hostname**
The hostname to set inside the container.

### **image**
The container image to use. This can be pulled from public registries like Docker Hub or private registries.

### **auth**
Authentication credentials for private registries:
- **username**: The registry username.
- **password**: The registry password.

### **ports**
Defines port mappings between the host and container in the format ` + "`<host port>:<container port>/<transport>`" + `. The transport protocol (` + "`tcp`" + ` or ` + "`udp`" + `) is optional.

### **volumes**
Specifies volume mappings in the format ` + "`<host path>:<container path>`" + `. This ensures data persists beyond the container's lifecycle.

### **command**
Overrides the default command specified in the container image. Provide commands as a list of strings.

### **privileged**
When set to ` + "`true`" + `, grants the container extended privileges on the host. Use cautiously due to potential security risks.

### **network**
Specifies the network mode for the container. Options include:
- ` + "`bridge`" + `: Default Docker network.
- ` + "`host`" + `: Shares the host's network stack.
- ` + "`none`" + `: Disables networking.
- ` + "`container:<name|id>`" + `: Shares the network stack of another container.

### **environment**
Defines environment variables in the format ` + "`<variable>=<value>`" + `.

### **cap_add / cap_drop**
Adds or removes Linux capabilities for the container, controlling privileged operations.

### **devices**
Maps devices from the host to the container in the format ` + "`<host path>:<container path>`" + `.

### **dns**
Specifies custom DNS servers for the container.

### **add_host**
Adds custom host-to-IP mappings to the container's ` + "`/etc/hosts`" + ` file.

### **dns_search**
Defines custom DNS search domains for the container.`
}

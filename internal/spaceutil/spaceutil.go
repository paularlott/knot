package spaceutil

import (
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/apple"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/container/podman"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/health"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

func NormalizeNomadNamespace(namespace string) string {
	if namespace == "" {
		return "default"
	}
	return namespace
}

func NomadRuntimeKey(space *model.Space) string {
	return NormalizeNomadNamespace(space.NomadNamespace) + "\x00" + space.ContainerId
}

func RuntimeRefRunning(space *model.Space, template *model.Template, refs map[string]bool) bool {
	if template == nil || refs == nil {
		return false
	}

	switch template.Platform {
	case model.PlatformNomad:
		return refs[NomadRuntimeKey(space)]
	default:
		return refs[space.ContainerId]
	}
}

func ListRunningRuntimeRefs(template *model.Template, spaces []*model.Space) (map[string]bool, error) {
	if template == nil || template.IsManual() {
		return map[string]bool{}, nil
	}

	cfg := config.GetServerConfig()
	platform := template.Platform
	if platform == model.PlatformContainer {
		platform = runtime.DetectLocalContainerRuntime(cfg.LocalContainerRuntimePref)
		if platform == "" {
			return nil, fmt.Errorf("no local container runtime detected")
		}
	}

	switch platform {
	case model.PlatformDocker:
		client := docker.NewClient()
		if client == nil {
			return nil, fmt.Errorf("failed to create docker client")
		}
		return client.ListRunningSpaceRuntimeRefs()
	case model.PlatformPodman:
		client := podman.NewClient()
		if client == nil {
			return nil, fmt.Errorf("failed to create podman client")
		}
		return client.ListRunningSpaceRuntimeRefs()
	case model.PlatformApple:
		client := apple.NewClient()
		if client == nil {
			return nil, fmt.Errorf("failed to create apple client")
		}
		return client.ListRunningSpaceRuntimeRefs()
	case model.PlatformNomad:
		client, err := nomad.NewClient()
		if err != nil {
			return nil, err
		}

		namespaces := make([]string, 0, len(spaces))
		for _, space := range spaces {
			if space == nil {
				continue
			}
			namespaces = append(namespaces, space.NomadNamespace)
		}
		return client.ListRunningSpaceRuntimeRefs(namespaces)
	default:
		return nil, fmt.Errorf("unsupported platform for runtime listing: %s", platform)
	}
}

func MarkSpaceStopped(space *model.Space) error {
	db := database.GetInstance()

	oldSpace := *space
	space.IsPending = false
	space.IsDeployed = false
	space.UpdatedAt = hlc.Now()

	if err := db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"}); err != nil {
		return err
	}

	health.Delete(space.Id)

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)
	service.CheckSpaceLifecycleEvents(&oldSpace, space)

	return nil
}

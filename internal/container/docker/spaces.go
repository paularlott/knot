package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/paularlott/gossip/hlc"
	"gopkg.in/yaml.v3"
)

type authConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type jobSpec struct {
	ContainerName string      `yaml:"container_name,omitempty"`
	Hostname      string      `yaml:"hostname,omitempty"`
	Image         string      `yaml:"image"`
	Auth          *authConfig `yaml:"auth,omitempty"`
	Ports         []string    `yaml:"ports,omitempty"`
	Volumes       []string    `yaml:"volumes,omitempty"`
	Command       []string    `yaml:"command,omitempty"`
	Privileged    bool        `yaml:"privileged,omitempty"`
	Network       string      `yaml:"network,omitempty"`
	Environment   []string    `yaml:"environment,omitempty"`
	CapAdd        []string    `yaml:"cap_add,omitempty"`
	CapDrop       []string    `yaml:"cap_drop,omitempty"`
	Devices       []string    `yaml:"devices,omitempty"`
	DNS           []string    `yaml:"dns,omitempty"`
	AddHost       []string    `yaml:"add_host,omitempty"`
	DNSSearch     []string    `yaml:"dns_search,omitempty"`
}

type volInfo struct {
	Volumes map[string]interface{} `yaml:"volumes"`
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (c *DockerClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {

	c.Logger.Debug("creating space job", "space_id", space.Id)

	// Pre-parse the job to fill out the knot variables
	job, err := model.ResolveVariables(template.Job, template, space, user, variables)
	if err != nil {
		return err
	}

	// Parse the job spec
	var spec jobSpec
	err = yaml.Unmarshal([]byte(job), &spec)
	if err != nil {
		return err
	}

	// Check the image is set
	if spec.Image == "" {
		return fmt.Errorf("image must be set")
	}

	// If no host name given then use the space name
	if spec.Hostname == "" {
		spec.Hostname = space.Id
	}

	// If container name not set then use the space name prefixed with the user name
	if spec.ContainerName == "" {
		spec.ContainerName = fmt.Sprintf("%s-%s", user.Username, space.Name)
	}

	// Ensure CAP_AUDIT_WRITE is in the cap_add list
	if !contains(spec.CapAdd, "CAP_AUDIT_WRITE") {
		spec.CapAdd = append(spec.CapAdd, "CAP_AUDIT_WRITE")
	}

	// Create the container config
	containerConfig := &container.Config{
		Image:        spec.Image,
		Hostname:     spec.Hostname,
		Env:          spec.Environment,
		ExposedPorts: nat.PortSet{},
		Cmd:          spec.Command,
	}

	resourcesConfig := container.Resources{
		Devices: []container.DeviceMapping{},
	}

	for _, device := range spec.Devices {
		parts := strings.Split(device, ":")
		if len(parts) != 2 {
			return fmt.Errorf("device must be in the format hostPath:containerPath, got %s", device)
		}

		resourcesConfig.Devices = append(resourcesConfig.Devices, container.DeviceMapping{
			PathOnHost:        parts[0],
			PathInContainer:   parts[1],
			CgroupPermissions: "rwm",
		})
	}

	hostConfig := &container.HostConfig{
		Privileged:  spec.Privileged,
		NetworkMode: container.NetworkMode(spec.Network),
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		Binds:        spec.Volumes,
		PortBindings: nat.PortMap{},
		CapAdd:       spec.CapAdd,
		CapDrop:      spec.CapDrop,
		Resources:    resourcesConfig,
	}

	// Run list of ports and add to config ExposedPorts and host config PortBindings
	for _, port := range spec.Ports {
		// Split the port into host and container ports
		ports := strings.Split(port, ":")
		if len(ports) != 2 {
			return fmt.Errorf("port must be in the format hostPort:containerPort, got %s", port)
		}

		// Add the port to the config
		containerConfig.ExposedPorts[nat.Port(ports[1])] = struct{}{}

		// Add the port to the host config
		hostConfig.PortBindings[nat.Port(ports[1])] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: ports[0],
			},
		}
	}

	// Add custom dns servers if specified
	if len(spec.DNS) > 0 {
		hostConfig.DNS = spec.DNS
	}

	// Add custom hosts if specified
	if len(spec.AddHost) > 0 {
		hostConfig.ExtraHosts = spec.AddHost
	}

	// Add custom DNS search domains if specified
	if len(spec.DNSSearch) > 0 {
		hostConfig.DNSSearch = spec.DNSSearch
	}

	// Record deploying
	db := database.GetInstance()
	cfg := config.GetServerConfig()
	space.IsPending = true
	space.IsDeployed = false
	space.IsDeleting = false
	space.TemplateHash = template.Hash
	space.Zone = cfg.Zone
	space.StartedAt = time.Now().UTC()
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "Zone", "UpdatedAt", "StartedAt"})
	if err != nil {
		c.Logger.Error("creating space job error", "space_id", space.Id)
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	// launch the container in a go routing to avoid blocking
	go func() {
		// Create context with timeout for container operations
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		succeeded := false
		// Clean up on exit
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.Logger.Error("creating space job error", "space_id", space.Id)
			}
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			// Only publish SSE event if we didn't succeed (error/timeout paths)
			// Success path publishes its own event with IsDeployed=true
			if !succeeded {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}()

		// Create a Docker client
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(c.Host))
		if err != nil {
			c.Logger.Error("creating space job error", "space_id", space.Id)
			return
		}

		// Create the registry auth if needed
		var authStr string = ""
		if spec.Auth != nil {
			authConfig := registry.AuthConfig{
				Username: spec.Auth.Username,
				Password: spec.Auth.Password,
			}
			encodedJSON, err := json.Marshal(authConfig)
			if err != nil {
				c.Logger.Error("creating space job error", "space_id", space.Id)
				return
			}
			authStr = base64.URLEncoding.EncodeToString(encodedJSON)
		}

		// Check if context has been cancelled before pulling the image
		select {
		case <-ctx.Done():
			c.Logger.Warn("image pull cancelled due to timeout", "space_id", space.Id, "image", spec.Image)
			return
		default:
		}

		// Pull the container
		c.Logger.Debug("pulling image", "spec_image", spec.Image)
		reader, err := cli.ImagePull(ctx, spec.Image, image.PullOptions{
			RegistryAuth: authStr,
		})
		if err != nil {
			c.Logger.Error("pulling image, error:", "spec_image", spec.Image)
			return
		}
		defer func() {
			if err := reader.Close(); err != nil {
				c.Logger.Error("pulling image, error:", "spec_image", spec.Image)
			}
		}()
		io.Copy(os.Stdout, reader)

		// Check if context has been cancelled before creating the container
		select {
		case <-ctx.Done():
			c.Logger.Warn("container creation cancelled due to timeout", "space_id", space.Id, "container_name", spec.ContainerName)
			return
		default:
		}

		// Create the container
		c.Logger.Debug("creating container", "spec_containername", spec.ContainerName)
		resp, err := cli.ContainerCreate(
			ctx,
			containerConfig,
			hostConfig,
			nil,
			nil,
			spec.ContainerName,
		)
		if err != nil {
			c.Logger.Error("creating container, error:", "spec_containername", spec.ContainerName)
			return
		}

		// Check context again before starting the container
		select {
		case <-ctx.Done():
			c.Logger.Warn("container start cancelled due to timeout", "space_id", space.Id, "container_id", resp.ID)
			// Clean up the created container
			cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})
			return
		default:
		}

		// Start the container
		c.Logger.Debug("starting container,", "spec_containername", spec.ContainerName, "resp_id", resp.ID)
		err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
		if err != nil {
			// Failed to start the container so remove it
			cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})

			c.Logger.Error("starting container,, error:", "spec_containername", spec.ContainerName, "resp_id", resp.ID)
			return
		}

		c.Logger.Debug("container running,", "spec_containername", spec.ContainerName, "resp_id", resp.ID)

		// Record the container ID and that the space is running
		db := database.GetInstance()
		space.ContainerId = resp.ID
		space.IsPending = false
		space.IsDeployed = true
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"ContainerId", "IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			c.Logger.Error("creating space job error", "space_id", space.Id)
			return
		}
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		succeeded = true
		sse.PublishSpaceChanged(space.Id, space.UserId)
	}()

	return nil
}

func (c *DockerClient) DeleteSpaceJob(space *model.Space, onStopped func()) error {
	c.Logger.Debug("deleting space job,", "space_id", space.Id, "space_containerid", space.ContainerId)

	db := database.GetInstance()

	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
	if err != nil {
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	// Run the delete in a go routine to avoid blocking
	go func() {
		// Create context with timeout for container operations
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		succeeded := false
		// Clean up on exit
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.Logger.Error("creating space job error", "space_id", space.Id)
			}

			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			// Only publish SSE event if we didn't succeed (error/timeout paths)
			// Success path publishes its own event with IsDeployed=false
			if !succeeded {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}()

		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(c.Host))
		if err != nil {
			c.Logger.Error("deleting space job error", "space_id", space.Id)
			return
		}

		// Check if context has been cancelled before stopping the container
		select {
		case <-ctx.Done():
			c.Logger.Warn("container stop cancelled due to timeout", "space_id", space.Id, "container_id", space.ContainerId)
			return
		default:
		}

		// Stop the container
		c.Logger.Debug("stopping container", "space_containerid", space.ContainerId)
		err = cli.ContainerStop(ctx, space.ContainerId, container.StopOptions{})
		if err != nil {
			if !strings.Contains(err.Error(), "No such container") {
				c.Logger.Error("stopping container error", "space_containerid", space.ContainerId)
				return
			}
		}

		// Wait for the container to be stopped (max 30s)
		timeout := time.Now().Add(30 * time.Second)
		for {
			// Check if context has been cancelled
			select {
			case <-ctx.Done():
				c.Logger.Warn("container stop cancelled due to timeout", "space_containerid", space.ContainerId)
				return
			default:
			}

			inspect, err := cli.ContainerInspect(ctx, space.ContainerId)
			if err != nil {
				if strings.Contains(err.Error(), "No such container") {
					break // container is gone
				}
				c.Logger.Error("inspecting container error", "space_containerid", space.ContainerId)
				return
			}
			if inspect.State != nil && !inspect.State.Running {
				break
			}
			if time.Now().After(timeout) {
				c.Logger.Error("timeout waiting for container to stop", "space_containerid", space.ContainerId)
				return
			}
			c.Logger.Debug("waiting for container to stop", "space_containerid", space.ContainerId)
			time.Sleep(500 * time.Millisecond)
		}

		// Check if context has been cancelled before removing the container
		select {
		case <-ctx.Done():
			c.Logger.Warn("container removal cancelled due to timeout", "space_id", space.Id, "container_id", space.ContainerId)
			return
		default:
		}

		// Remove the container
		c.Logger.Debug("removing container", "space_containerid", space.ContainerId)
		err = cli.ContainerRemove(ctx, space.ContainerId, container.RemoveOptions{})
		if err != nil {
			if !strings.Contains(err.Error(), "No such container") {
				c.Logger.Error("removing container error", "space_containerid", space.ContainerId)
				return
			}
		}

		space.IsPending = false
		space.IsDeployed = false
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			c.Logger.Error("deleting space job error", "space_id", space.Id)
			return
		}

		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		succeeded = true
		sse.PublishSpaceChanged(space.Id, space.UserId)

		if onStopped != nil {
			onStopped()
		}
	}()

	return nil
}

func (c *DockerClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {

	// Parse the volume definition to fill out the knot variables
	volumes, err := model.ResolveVariables(template.Volumes, template, space, user, variables)
	if err != nil {
		return err
	}

	var volInfo volInfo
	err = yaml.Unmarshal([]byte(volumes), &volInfo)
	if err != nil {
		return err
	}

	if len(volInfo.Volumes) == 0 && len(space.VolumeData) == 0 {
		c.Logger.Debug("no volumes to create")
		return nil
	}

	c.Logger.Debug("checking for required volumes")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(c.Host))
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Store initial volume data to detect changes
	initialVolumeData := make(map[string]model.SpaceVolume)
	for k, v := range space.VolumeData {
		initialVolumeData[k] = v
	}

	defer func() {
		// Only save and publish if volumes actually changed
		volumesChanged := false
		if len(initialVolumeData) != len(space.VolumeData) {
			volumesChanged = true
		} else {
			for k, v := range space.VolumeData {
				if initialV, ok := initialVolumeData[k]; !ok || v != initialV {
					volumesChanged = true
					break
				}
			}
		}

		if volumesChanged {
			space.UpdatedAt = hlc.Now()
			db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			sse.PublishSpaceChanged(space.Id, space.UserId)
		}
	}()

	// Find the volumes that are defined but not yet created in the space and create them
	for volName, _ := range volInfo.Volumes {
		c.Logger.Debug("checking volume", "volname", volName)

		// Check if the volume is already created for the space
		if _, ok := space.VolumeData[volName]; !ok {
			c.Logger.Debug("creating volume", "volname", volName)

			volume, err := cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: volName})
			if err != nil {
				return err
			}

			space.VolumeData[volName] = model.SpaceVolume{
				Id:        volume.Name,
				Namespace: "_docker",
			}
		}
	}

	// Find the volumes deployed in the space but no longer in the template definition and remove them
	for volName, _ := range space.VolumeData {
		// Check if the volume is defined in the template
		if _, ok := volInfo.Volumes[volName]; !ok {
			c.Logger.Debug("deleting volume", "volname", volName)

			err := cli.VolumeRemove(context.Background(), volName, true)
			if err != nil {
				return err
			}

			delete(space.VolumeData, volName)
		}
	}

	c.Logger.Debug("volumes checked")

	return nil
}

func (c *DockerClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	c.Logger.Debug("deleting volumes")

	if len(space.VolumeData) == 0 {
		c.Logger.Debug("no volumes to delete")
		return nil
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithHost(c.Host))
	if err != nil {
		return err
	}

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
		sse.PublishSpaceChanged(space.Id, space.UserId)
	}()

	// For all volumes in the space delete them
	for volName, _ := range space.VolumeData {
		c.Logger.Debug("deleting volume", "volname", volName)

		err := cli.VolumeRemove(context.Background(), volName, true)
		if err != nil {
			return err
		}

		delete(space.VolumeData, volName)
	}

	c.Logger.Debug("volumes deleted")

	return nil
}

package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/logger"
	"gopkg.in/yaml.v3"
)

const spaceStartupTimeout = 30 * time.Minute

type AppleClient struct {
	DriverName string
	logger     logger.Logger
}

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
	Network       string      `yaml:"network,omitempty"`
	Environment   []string    `yaml:"environment,omitempty"`
	DNS           []string    `yaml:"dns,omitempty"`
	AddHost       []string    `yaml:"add_host,omitempty"`
	DNSSearch     []string    `yaml:"dns_search,omitempty"`
	Memory        string      `yaml:"memory,omitempty"`
	CPUs          string      `yaml:"cpus,omitempty"`
}

type containerInspect struct {
	ID     string `json:"ID"`
	Status string `json:"status"`
}

type appleListContainer struct {
	Status        string `json:"status"`
	Configuration struct {
		ID string `json:"id"`
	} `json:"configuration"`
}

func normalizeContainerReference(ref string) string {
	ref = strings.ReplaceAll(ref, "\r\n", "\n")
	ref = strings.ReplaceAll(ref, "\r", "\n")

	lines := strings.Split(ref, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}

	return ""
}

func NewClient() *AppleClient {
	return &AppleClient{
		DriverName: "apple",
		logger:     log.WithGroup("apple"),
	}
}

func (c *AppleClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	c.logger.Debug("creating space job", "space_id", space.Id)

	job, err := model.ResolveVariables(template.Job, template, space, user, variables)
	if err != nil {
		return err
	}

	var spec jobSpec
	err = yaml.Unmarshal([]byte(job), &spec)
	if err != nil {
		return err
	}

	if spec.Image == "" {
		return fmt.Errorf("image must be set")
	}

	if spec.Hostname == "" {
		spec.Hostname = space.Id
	}

	if spec.ContainerName == "" {
		spec.ContainerName = fmt.Sprintf("%s-%s", user.Username, space.Name)
	}
	if err := container.ValidateManagedVolumeBinds(spec.Volumes, space.VolumeData); err != nil {
		return err
	}
	spec.Volumes = container.ResolveManagedPathBinds(spec.Volumes, space.VolumeData)

	db := database.GetInstance()
	space.IsPending = true
	space.IsDeployed = false
	space.IsDeleting = false
	space.TemplateHash = template.Hash
	space.StartedAt = time.Now().UTC()
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "UpdatedAt", "StartedAt"})
	if err != nil {
		c.logger.Error("creating space job error", "space_id", space.Id)
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	go func() {
		// Large image pulls can legitimately take a while; use a long startup window.
		ctx, cancel := context.WithTimeout(context.Background(), spaceStartupTimeout)
		defer cancel()

		succeeded := false
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.logger.Error("creating space job error", "space_id", space.Id)
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

		args := []string{"run", "-d", "--name", spec.ContainerName}

		// Inject port env vars from template, overwriting any existing values
	spec.Environment = container.RemoveExistingPortEnvVars(spec.Environment)
	spec.Environment = append(spec.Environment, container.BuildPortEnvVars(template)...)

	for _, env := range spec.Environment {
			args = append(args, "-e", env)
		}

		for _, port := range spec.Ports {
			args = append(args, "-p", port)
		}

		for _, vol := range spec.Volumes {
			args = append(args, "-v", vol)
		}

		if spec.Network != "" {
			args = append(args, "--network", spec.Network)
		}

		for _, dns := range spec.DNS {
			args = append(args, "--dns", dns)
		}

		for _, search := range spec.DNSSearch {
			args = append(args, "--dns-search", search)
		}

		if spec.Memory != "" {
			args = append(args, "--memory", spec.Memory)
		}

		if spec.CPUs != "" {
			args = append(args, "--cpus", spec.CPUs)
		}

		args = append(args, spec.Image)
		args = append(args, spec.Command...)

		// Check if context has been cancelled before running the command
		select {
		case <-ctx.Done():
			c.logger.Warn("container creation cancelled due to timeout", "space_id", space.Id, "container_name", spec.ContainerName, "timeout", spaceStartupTimeout)
			return
		default:
		}

		c.logger.Debug("running container", "spec_containername", spec.ContainerName)
		cmd := exec.CommandContext(ctx, "container", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			c.logger.Error("creating container, error:, output:", "spec_containername", spec.ContainerName, "output", string(output))
			return
		}

		containerID := normalizeContainerReference(string(output))
		if containerID == "" {
			c.logger.Error("creating container returned empty container reference", "spec_containername", spec.ContainerName, "output", string(output))
			return
		}
		c.logger.Debug("container running,", "spec_containername", spec.ContainerName, "containerid", containerID)

		db := database.GetInstance()
		space.ContainerId = containerID
		space.IsPending = false
		space.IsDeployed = true
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"ContainerId", "IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			c.logger.Error("creating space job error", "space_id", space.Id)
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

func (c *AppleClient) DeleteSpaceJob(space *model.Space, onStopped func()) error {
	containerRef := normalizeContainerReference(space.ContainerId)
	c.logger.Debug("deleting space job,", "space_id", space.Id, "space_containerid", containerRef)

	if containerRef == "" {
		return fmt.Errorf("space container reference is empty")
	}

	db := database.GetInstance()

	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
	if err != nil {
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	go func() {
		// Create context with timeout for container operations
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		succeeded := false
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.logger.Error("creating space job error", "space_id", space.Id)
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

		// Check if context has been cancelled before stopping the container
		select {
		case <-ctx.Done():
			c.logger.Warn("container stop cancelled due to timeout", "space_id", space.Id, "container_id", space.ContainerId)
			return
		default:
		}

		c.logger.Debug("stopping container", "space_containerid", containerRef)
		cmd := exec.CommandContext(ctx, "container", "stop", containerRef)
		output, err := cmd.CombinedOutput()
		if err != nil {
			outputStr := string(output)
			if !strings.Contains(outputStr, "not found") && !strings.Contains(outputStr, "internalError") {
				c.logger.Error("stopping container error, output:", "space_containerid", containerRef, "output", outputStr)
				return
			}
			if strings.Contains(outputStr, "internalError") {
				c.logger.Warn("stop returned XPC error, will wait for container to stop", "space_containerid", containerRef)
			}
		}

		timeout := time.Now().Add(30 * time.Second)
		for {
			// Check if context has been cancelled
			select {
			case <-ctx.Done():
				c.logger.Warn("container stop cancelled due to timeout", "space_containerid", containerRef)
				return
			default:
			}

			cmd := exec.CommandContext(ctx, "container", "inspect", containerRef)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if strings.Contains(string(output), "not found") {
					break
				}
				c.logger.Error("inspecting container error", "space_containerid", containerRef)
				return
			}

			var inspectData []containerInspect
			if err := json.Unmarshal(output, &inspectData); err != nil {
				c.logger.Error("parsing inspect output error", "space_containerid", containerRef)
				return
			}

			if len(inspectData) == 0 || inspectData[0].Status != "running" {
				break
			}

			if time.Now().After(timeout) {
				c.logger.Error("timeout waiting for container to stop", "space_containerid", containerRef)
				return
			}

			c.logger.Debug("waiting for container to stop", "space_containerid", containerRef)
			time.Sleep(500 * time.Millisecond)
		}

		// Check if context has been cancelled before removing the container
		select {
		case <-ctx.Done():
			c.logger.Warn("container removal cancelled due to timeout", "space_id", space.Id, "container_id", space.ContainerId)
			return
		default:
		}

		c.logger.Debug("removing container", "space_containerid", containerRef)
		cmd = exec.CommandContext(ctx, "container", "rm", containerRef)
		output, err = cmd.CombinedOutput()
		if err != nil {
			if !strings.Contains(string(output), "not found") {
				c.logger.Error("removing container error, output:", "space_containerid", containerRef, "output", string(output))
				return
			}
		}

		space.IsPending = false
		space.IsDeployed = false
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			c.logger.Error("deleting space job error", "space_id", space.Id)
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

func (c *AppleClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	volInfo, err := model.LoadLocalStorageFromYaml(template.Volumes, template, space, user, variables)
	if err != nil {
		return err
	}

	if len(volInfo.Volumes) == 0 && len(volInfo.Paths) == 0 && len(space.VolumeData) == 0 {
		c.logger.Debug("no volumes to create")
		return nil
	}

	c.logger.Debug("checking for required volumes")

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

	for volName, spec := range volInfo.Volumes {
		c.logger.Debug("checking volume", "volname", volName)

		if _, ok := space.VolumeData[volName]; !ok {
			c.logger.Debug("creating volume", "volname", volName)

			args := []string{"volume", "create"}
			if spec.Size != "" {
				args = append(args, "-s", spec.Size)
			}
			args = append(args, volName)
			cmd := exec.Command("container", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				c.logger.Error("creating volume error, output:", "volname", volName, "output", string(output))
				return err
			}

			space.VolumeData[volName] = model.SpaceVolume{
				Id:        volName,
				Namespace: "_apple",
			}
		}
	}

	requiredPaths := make(map[string]bool)
	for _, path := range volInfo.Paths {
		c.logger.Debug("checking path", "path", path)
		requiredPaths[path] = true
		data, ok := space.VolumeData[path]
		if !ok || data.Type != container.ManagedPathType {
			c.logger.Debug("creating path", "path", path)
			resolved, err := container.CreateManagedPath(path)
			if err != nil {
				return err
			}
			space.VolumeData[path] = model.SpaceVolume{
				Id:        resolved,
				Namespace: "_path",
				Type:      container.ManagedPathType,
			}
		} else {
			resolved, err := container.ResolveManagedPath(path)
			if err != nil {
				return err
			}
			if _, err := os.Stat(resolved); os.IsNotExist(err) {
				c.logger.Debug("recreating missing path", "path", path)
				if err := os.MkdirAll(resolved, 0755); err != nil {
					return err
				}
			}
		}
	}

	for volName, data := range space.VolumeData {
		if data.Type == container.ManagedPathType {
			if requiredPaths[volName] {
				continue
			}
			c.logger.Debug("deleting path", "path", volName)
			if err := container.DeleteManagedPath(data.Id); err != nil {
				return err
			}
			delete(space.VolumeData, volName)
			continue
		}

		if _, ok := volInfo.Volumes[volName]; !ok {
			c.logger.Debug("deleting volume", "volname", volName)

			cmd := exec.Command("container", "volume", "rm", volName)
			output, err := cmd.CombinedOutput()
			if err != nil {
				c.logger.Error("deleting volume error, output:", "volname", volName, "output", string(output))
				return err
			}

			delete(space.VolumeData, volName)
		}
	}

	c.logger.Debug("volumes checked")

	return nil
}

func (c *AppleClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	c.logger.Debug("deleting volumes")

	if len(space.VolumeData) == 0 {
		c.logger.Debug("no volumes to delete")
		return nil
	}

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
		sse.PublishSpaceChanged(space.Id, space.UserId)
	}()

	for volName, data := range space.VolumeData {
		if data.Type == container.ManagedPathType {
			c.logger.Debug("deleting path", "path", volName)
			if err := container.DeleteManagedPath(data.Id); err != nil {
				return err
			}
			delete(space.VolumeData, volName)
			continue
		}

		c.logger.Debug("deleting volume", "volname", volName)

		cmd := exec.Command("container", "volume", "rm", volName)
		output, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "not found") {
			c.logger.Error("deleting volume error, output:", "volname", volName, "output", string(output))
			return err
		}

		delete(space.VolumeData, volName)
	}

	c.logger.Debug("volumes deleted")

	return nil
}

func isIgnorableAppleCleanupOutput(output string) bool {
	output = strings.ToLower(output)
	return strings.Contains(output, "not found") ||
		strings.Contains(output, "no volume") ||
		strings.Contains(output, "does not exist") ||
		strings.Contains(output, "no such") ||
		strings.Contains(output, "not exist") ||
		strings.Contains(output, "unable to find")
}

func appleCleanupError(action, ref string, err error, output []byte) error {
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return fmt.Errorf("%s %s failed: %w", action, ref, err)
	}
	return fmt.Errorf("%s %s failed: %w: %s", action, ref, err, outputStr)
}

func (c *AppleClient) CleanupSpaceArtifacts(space *model.Space) error {
	containerRef := normalizeContainerReference(space.ContainerId)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if containerRef != "" {
		c.logger.Debug("cleaning migrated space container", "space_id", space.Id, "space_containerid", containerRef)

		cmd := exec.CommandContext(ctx, "container", "stop", containerRef)
		output, err := cmd.CombinedOutput()
		if err != nil {
			outputStr := string(output)
			if !isIgnorableAppleCleanupOutput(outputStr) && !strings.Contains(strings.ToLower(outputStr), "internalerror") {
				return appleCleanupError("stop migrated space container", containerRef, err, output)
			}
		}

		timeout := time.Now().Add(30 * time.Second)
		for {
			cmd := exec.CommandContext(ctx, "container", "inspect", containerRef)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if isIgnorableAppleCleanupOutput(string(output)) {
					break
				}
				return appleCleanupError("inspect migrated space container", containerRef, err, output)
			}

			var inspectData []containerInspect
			if err := json.Unmarshal(output, &inspectData); err != nil {
				return err
			}

			if len(inspectData) == 0 || inspectData[0].Status != "running" {
				break
			}

			if time.Now().After(timeout) {
				return fmt.Errorf("timeout waiting for migrated container to stop")
			}

			time.Sleep(500 * time.Millisecond)
		}

		cmd = exec.CommandContext(ctx, "container", "rm", containerRef)
		output, err = cmd.CombinedOutput()
		if err != nil && !isIgnorableAppleCleanupOutput(string(output)) {
			return appleCleanupError("remove migrated space container", containerRef, err, output)
		}
	}

	for volName, data := range space.VolumeData {
		if data.Type == container.ManagedPathType {
			c.logger.Debug("cleaning migrated space path", "space_id", space.Id, "path", volName)
			if err := container.DeleteManagedPath(data.Id); err != nil {
				return err
			}
			continue
		}

		c.logger.Debug("cleaning migrated space volume", "space_id", space.Id, "volname", volName)
		cmd := exec.CommandContext(ctx, "container", "volume", "rm", volName)
		output, err := cmd.CombinedOutput()
		if err != nil && !isIgnorableAppleCleanupOutput(string(output)) {
			return appleCleanupError("remove migrated space volume", volName, err, output)
		}
	}

	return nil
}

func (c *AppleClient) StopSpaceRuntime(space *model.Space) error {
	containerRef := normalizeContainerReference(space.ContainerId)
	if containerRef == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "container", "stop", containerRef)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if !isIgnorableAppleCleanupOutput(outputStr) && !strings.Contains(outputStr, "internalError") {
			return err
		}
	}

	timeout := time.Now().Add(30 * time.Second)
	for {
		cmd := exec.CommandContext(ctx, "container", "inspect", containerRef)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if isIgnorableAppleCleanupOutput(string(output)) {
				break
			}
			return err
		}

		var inspectData []containerInspect
		if err := json.Unmarshal(output, &inspectData); err != nil {
			return err
		}

		if len(inspectData) == 0 || inspectData[0].Status != "running" {
			break
		}

		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for container to stop")
		}

		time.Sleep(500 * time.Millisecond)
	}

	cmd = exec.CommandContext(ctx, "container", "rm", containerRef)
	output, err = cmd.CombinedOutput()
	if err != nil && !isIgnorableAppleCleanupOutput(string(output)) {
		return err
	}

	return nil
}

func (c *AppleClient) ListRunningSpaceRuntimeRefs() (map[string]bool, error) {
	cmd := exec.Command("container", "ls", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	refs := make(map[string]bool)

	var listResponse []appleListContainer
	if err := json.Unmarshal(output, &listResponse); err == nil {
		for _, container := range listResponse {
			if container.Status != "running" {
				continue
			}
			if container.Configuration.ID != "" {
				refs[container.Configuration.ID] = true
			}
		}
		return refs, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var item appleListContainer
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			if item.Status != "running" {
				continue
			}
			if item.Configuration.ID != "" {
				refs[item.Configuration.ID] = true
			}
		}
	}

	return refs, nil
}

func (c *AppleClient) CreateVolume(vol *model.Volume, variables map[string]interface{}) error {
	c.logger.Debug("creating volume")

	volInfo, err := model.LoadLocalStorageFromYaml(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	if len(volInfo.Volumes) != 1 {
		return fmt.Errorf("volume definition must contain exactly 1 volume")
	}

	for volName, spec := range volInfo.Volumes {
		c.logger.Debug("creating volume:", "volname", volName)

		args := []string{"volume", "create"}
		if spec.Size != "" {
			args = append(args, "-s", spec.Size)
		}
		args = append(args, volName)
		cmd := exec.Command("container", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			c.logger.Error("creating volume error, output:", "volname", volName, "output", string(output))
			return err
		}
	}

	c.logger.Debug("volume created")

	return nil
}

func (c *AppleClient) DeleteVolume(vol *model.Volume, variables map[string]interface{}) error {
	c.logger.Debug("deleting volume")

	volInfo, err := model.LoadLocalStorageFromYaml(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	for volName := range volInfo.Volumes {
		c.logger.Debug("deleting volume:", "volname", volName)

		cmd := exec.Command("container", "volume", "rm", volName)
		output, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "not found") {
			c.logger.Error("deleting volume error, output:", "volname", volName, "output", string(output))
			return err
		}
	}

	c.logger.Debug("volume deleted")

	return nil
}

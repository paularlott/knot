package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"gopkg.in/yaml.v3"
)

// ---- job spec (parsed from template YAML) ----

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
	Memory        string      `yaml:"memory,omitempty"`
	CPUs          string      `yaml:"cpus,omitempty"`
}

type volInfo struct {
	Volumes map[string]interface{} `yaml:"volumes"`
}

// ---- Docker REST API request/response types ----

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type deviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}

type restartPolicy struct {
	Name string `json:"Name"`
}

type containerHostConfig struct {
	Binds         []string                 `json:"Binds,omitempty"`
	PortBindings  map[string][]portBinding `json:"PortBindings,omitempty"`
	Privileged    bool                     `json:"Privileged,omitempty"`
	NetworkMode   string                   `json:"NetworkMode,omitempty"`
	CapAdd        []string                 `json:"CapAdd,omitempty"`
	CapDrop       []string                 `json:"CapDrop,omitempty"`
	Devices       []deviceMapping          `json:"Devices,omitempty"`
	DNS           []string                 `json:"Dns,omitempty"`
	ExtraHosts    []string                 `json:"ExtraHosts,omitempty"`
	DNSSearch     []string                 `json:"DnsSearch,omitempty"`
	Memory        int64                    `json:"Memory,omitempty"`
	NanoCPUs      int64                    `json:"NanoCpus,omitempty"`
	RestartPolicy restartPolicy            `json:"RestartPolicy"`
}

type containerCreateRequest struct {
	Image        string                 `json:"Image"`
	Hostname     string                 `json:"Hostname"`
	Env          []string               `json:"Env,omitempty"`
	Cmd          []string               `json:"Cmd,omitempty"`
	ExposedPorts map[string]struct{}    `json:"ExposedPorts,omitempty"`
	HostConfig   containerHostConfig    `json:"HostConfig"`
}

type containerCreateResponse struct {
	ID string `json:"Id"`
}

type containerInspectResponse struct {
	State *struct {
		Running bool `json:"Running"`
	} `json:"State"`
}

// ---- helpers ----

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// toPortKey ensures a port string has a protocol suffix, e.g. "8080" -> "8080/tcp"
func toPortKey(port string) string {
	if !strings.Contains(port, "/") {
		return port + "/tcp"
	}
	return port
}

func parseMemory(memStr string) (int64, error) {
	if memStr == "" {
		return 0, nil
	}
	if strings.HasSuffix(memStr, "m") || strings.HasSuffix(memStr, "M") {
		val, err := strconv.ParseInt(memStr[:len(memStr)-1], 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1024 * 1024, nil
	} else if strings.HasSuffix(memStr, "g") || strings.HasSuffix(memStr, "G") {
		val, err := strconv.ParseInt(memStr[:len(memStr)-1], 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1024 * 1024 * 1024, nil
	}
	return strconv.ParseInt(memStr, 10, 64)
}

func parseCPUs(cpusStr string) (int64, error) {
	if cpusStr == "" {
		return 0, nil
	}
	cpus, err := strconv.ParseFloat(cpusStr, 64)
	if err != nil {
		return 0, err
	}
	return int64(cpus * 1e9), nil
}

// registryAuthHeader base64-encodes a JSON auth config for the X-Registry-Auth header.
func registryAuthHeader(username, password string) (string, error) {
	b, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ---- Docker API calls ----

// imagePull calls POST /images/create and scans the streaming response for errors.
// Docker returns HTTP 200 immediately and streams JSON progress objects; auth failures
// and other errors appear as {"error":"..."} objects in the stream, not as HTTP error codes.
func (c *DockerClient) imagePull(ctx context.Context, image string, authHeader string) error {
	// Parse image into name + tag.
	// Use raw query string construction to avoid url.Values encoding '/' as '%2F'
	// in registry hostnames (e.g. hub.example.com/library/image).
	name := image
	tag := "latest"
	if idx := strings.LastIndex(image, ":"); idx != -1 && !strings.Contains(image[idx:], "/") {
		name = image[:idx]
		tag = image[idx+1:]
	}

	hc := c.httpClient

	// Build URL for imagePull.
	// fromImage contains a registry path (e.g. registry.example.com:5000/my/image)
	// where '/' must NOT be encoded. We use url.QueryEscape then restore '/' so that
	// only truly unsafe query chars (spaces, &, #, +, etc.) are encoded.
	escapedName := strings.ReplaceAll(url.QueryEscape(name), "%2F", "/")
	rawURL := hc.GetBaseURL() + "/v1.41/images/create?fromImage=" + escapedName + "&tag=" + url.QueryEscape(tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("X-Registry-Auth", authHeader)
	}

	resp, err := hc.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("image pull failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Scan the streaming response for error objects.
	// Docker streams newline-delimited JSON; errors appear as {"error":"message"}.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var obj struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &obj) == nil && obj.Error != "" {
			return fmt.Errorf("image pull error: %s", obj.Error)
		}
	}
	return scanner.Err()
}

func (c *DockerClient) containerCreate(ctx context.Context, name string, req containerCreateRequest) (string, error) {
	var resp containerCreateResponse
	code, err := c.httpClient.PostJSON(ctx, "/v1.41/containers/create?name="+url.QueryEscape(name), req, &resp, http.StatusCreated)
	if err != nil {
		return "", fmt.Errorf("container create failed (HTTP %d): %w", code, err)
	}
	return resp.ID, nil
}

func (c *DockerClient) containerStart(ctx context.Context, id string) error {
	code, err := c.httpClient.PostJSON(ctx, fmt.Sprintf("/v1.41/containers/%s/start", id), nil, nil, http.StatusNoContent)
	if err != nil {
		return fmt.Errorf("container start failed (HTTP %d): %w", code, err)
	}
	return nil
}

func (c *DockerClient) containerStop(ctx context.Context, id string) error {
	// POST /containers/{id}/stop blocks for the container's grace period (default 10s)
	// before Docker sends SIGKILL and responds. We need a client with no timeout so
	// only the caller's context deadline applies. Build a separate http.Client that
	// shares the same transport (connection pool) but has Timeout=0, avoiding any
	// mutation of the shared client and the data race that would cause.
	blockingClient := &http.Client{
		Transport: c.httpClient.HTTPClient.Transport,
		Timeout:   0,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.httpClient.GetBaseURL()+fmt.Sprintf("/v1.41/containers/%s/stop", id), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := blockingClient.Do(req)
	if err != nil {
		return fmt.Errorf("container stop failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// 204 = stopped, 304 = already stopped, 404 = not found — all acceptable
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("container stop failed (HTTP %d)", resp.StatusCode)
}

func (c *DockerClient) containerRemove(ctx context.Context, id string) error {
	// No ?force=true — matches original SDK behaviour (RemoveOptions{} defaults to force=false).
	// Callers must ensure the container is stopped before calling this.
	code, err := c.httpClient.Delete(ctx, fmt.Sprintf("/v1.41/containers/%s", id), nil, nil, http.StatusNoContent)
	if err != nil {
		if code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("container remove failed (HTTP %d): %w", code, err)
	}
	return nil
}

func (c *DockerClient) containerInspect(ctx context.Context, id string) (*containerInspectResponse, int, error) {
	var resp containerInspectResponse
	code, err := c.httpClient.GetJSON(ctx, fmt.Sprintf("/v1.41/containers/%s/json", id), &resp)
	if err != nil {
		return nil, code, err
	}
	return &resp, code, nil
}

// ---- ContainerManager implementation ----

func (c *DockerClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	c.Logger.Debug("creating space job", "space_id", space.Id)

	job, err := model.ResolveVariables(template.Job, template, space, user, variables)
	if err != nil {
		return err
	}

	var spec jobSpec
	if err = yaml.Unmarshal([]byte(job), &spec); err != nil {
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
	if !contains(spec.CapAdd, "CAP_AUDIT_WRITE") {
		spec.CapAdd = append(spec.CapAdd, "CAP_AUDIT_WRITE")
	}

	// Build request structs
	exposedPorts := map[string]struct{}{}
	portBindings := map[string][]portBinding{}
	for _, port := range spec.Ports {
		parts := strings.Split(port, ":")
		if len(parts) != 2 {
			return fmt.Errorf("port must be in the format hostPort:containerPort, got %s", port)
		}
		key := toPortKey(parts[1])
		exposedPorts[key] = struct{}{}
		portBindings[key] = []portBinding{{HostIP: "0.0.0.0", HostPort: parts[0]}}
	}

	devices := []deviceMapping{}
	for _, device := range spec.Devices {
		parts := strings.Split(device, ":")
		if len(parts) != 2 {
			return fmt.Errorf("device must be in the format hostPath:containerPath, got %s", device)
		}
		devices = append(devices, deviceMapping{
			PathOnHost:        parts[0],
			PathInContainer:   parts[1],
			CgroupPermissions: "rwm",
		})
	}

	var memBytes int64
	if spec.Memory != "" {
		if memBytes, err = parseMemory(spec.Memory); err != nil {
			return err
		}
	}

	var nanoCPUs int64
	if spec.CPUs != "" {
		if nanoCPUs, err = parseCPUs(spec.CPUs); err != nil {
			return err
		}
	}

	createReq := containerCreateRequest{
		Image:        spec.Image,
		Hostname:     spec.Hostname,
		Env:          spec.Environment,
		Cmd:          spec.Command,
		ExposedPorts: exposedPorts,
		HostConfig: containerHostConfig{
			Binds:         spec.Volumes,
			PortBindings:  portBindings,
			Privileged:    spec.Privileged,
			NetworkMode:   spec.Network,
			CapAdd:        spec.CapAdd,
			CapDrop:       spec.CapDrop,
			Devices:       devices,
			DNS:           spec.DNS,
			ExtraHosts:    spec.AddHost,
			DNSSearch:     spec.DNSSearch,
			Memory:        memBytes,
			NanoCPUs:      nanoCPUs,
			RestartPolicy: restartPolicy{Name: "unless-stopped"},
		},
	}

	// Build registry auth header if needed
	var authHeader string
	if spec.Auth != nil {
		if authHeader, err = registryAuthHeader(spec.Auth.Username, spec.Auth.Password); err != nil {
			return err
		}
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
	if err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "Zone", "UpdatedAt", "StartedAt"}); err != nil {
		c.Logger.Error("creating space job error", "space_id", space.Id)
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		succeeded := false
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.Logger.Error("creating space job error", "space_id", space.Id)
			}
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			if !succeeded {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}()

		select {
		case <-ctx.Done():
			c.Logger.Warn("image pull cancelled due to timeout", "space_id", space.Id, "image", spec.Image)
			return
		default:
		}

		c.Logger.Debug("pulling image", "image", spec.Image)
		if err := c.imagePull(ctx, spec.Image, authHeader); err != nil {
			c.Logger.Error("pulling image error", "image", spec.Image, "error", err)
			return
		}

		select {
		case <-ctx.Done():
			c.Logger.Warn("container creation cancelled due to timeout", "space_id", space.Id)
			return
		default:
		}

		c.Logger.Debug("creating container", "name", spec.ContainerName)
		containerID, err := c.containerCreate(ctx, spec.ContainerName, createReq)
		if err != nil {
			c.Logger.Error("creating container error", "name", spec.ContainerName, "error", err)
			return
		}

		select {
		case <-ctx.Done():
			c.Logger.Warn("container start cancelled due to timeout", "space_id", space.Id)
			c.containerRemove(ctx, containerID)
			return
		default:
		}

		c.Logger.Debug("starting container", "name", spec.ContainerName, "id", containerID)
		if err := c.containerStart(ctx, containerID); err != nil {
			c.containerRemove(ctx, containerID)
			c.Logger.Error("starting container error", "name", spec.ContainerName, "error", err)
			return
		}

		c.Logger.Debug("container running", "name", spec.ContainerName, "id", containerID)

		space.ContainerId = containerID
		space.IsPending = false
		space.IsDeployed = true
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"ContainerId", "IsPending", "IsDeployed", "UpdatedAt"}); err != nil {
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
	c.Logger.Debug("deleting space job", "space_id", space.Id, "container_id", space.ContainerId)

	db := database.GetInstance()

	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		return err
	}

	service.GetTransport().GossipSpace(space)
	sse.PublishSpaceChanged(space.Id, space.UserId)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		succeeded := false
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.Logger.Error("deleting space job error", "space_id", space.Id)
			}
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			if !succeeded {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}()

		select {
		case <-ctx.Done():
			c.Logger.Warn("container stop cancelled due to timeout", "space_id", space.Id)
			return
		default:
		}

		c.Logger.Debug("stopping container", "container_id", space.ContainerId)
		if err := c.containerStop(ctx, space.ContainerId); err != nil {
			c.Logger.Error("stopping container error", "container_id", space.ContainerId, "error", err)
			return
		}

		// Poll until stopped (max 30s)
		deadline := time.Now().Add(30 * time.Second)
		for {
			select {
			case <-ctx.Done():
				c.Logger.Warn("container stop poll cancelled", "container_id", space.ContainerId)
				return
			default:
			}

			inspect, code, err := c.containerInspect(ctx, space.ContainerId)
			if err != nil {
				if code == http.StatusNotFound {
					break
				}
				c.Logger.Error("inspecting container error", "container_id", space.ContainerId, "error", err)
				return
			}
			if inspect.State != nil && !inspect.State.Running {
				break
			}
			if time.Now().After(deadline) {
				c.Logger.Error("timeout waiting for container to stop", "container_id", space.ContainerId)
				return
			}
			c.Logger.Debug("waiting for container to stop", "container_id", space.ContainerId)
			time.Sleep(500 * time.Millisecond)
		}

		select {
		case <-ctx.Done():
			c.Logger.Warn("container removal cancelled due to timeout", "space_id", space.Id)
			return
		default:
		}

		c.Logger.Debug("removing container", "container_id", space.ContainerId)
		if err := c.containerRemove(ctx, space.ContainerId); err != nil {
			c.Logger.Error("removing container error", "container_id", space.ContainerId, "error", err)
			return
		}

		space.IsPending = false
		space.IsDeployed = false
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"}); err != nil {
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
	volumes, err := model.ResolveVariables(template.Volumes, template, space, user, variables)
	if err != nil {
		return err
	}

	var vi volInfo
	if err = yaml.Unmarshal([]byte(volumes), &vi); err != nil {
		return err
	}

	if len(vi.Volumes) == 0 && len(space.VolumeData) == 0 {
		c.Logger.Debug("no volumes to create")
		return nil
	}

	c.Logger.Debug("checking for required volumes")

	db := database.GetInstance()

	initialVolumeData := make(map[string]model.SpaceVolume)
	for k, v := range space.VolumeData {
		initialVolumeData[k] = v
	}

	defer func() {
		volumesChanged := len(initialVolumeData) != len(space.VolumeData)
		if !volumesChanged {
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

	for volName := range vi.Volumes {
		c.Logger.Debug("checking volume", "name", volName)
		if _, ok := space.VolumeData[volName]; !ok {
			c.Logger.Debug("creating volume", "name", volName)
			name, err := c.volumeCreate(context.Background(), volName)
			if err != nil {
				return err
			}
			space.VolumeData[volName] = model.SpaceVolume{Id: name, Namespace: "_docker"}
		}
	}

	for volName := range space.VolumeData {
		if _, ok := vi.Volumes[volName]; !ok {
			c.Logger.Debug("deleting volume", "name", volName)
			if err := c.volumeRemove(context.Background(), volName); err != nil {
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

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
		sse.PublishSpaceChanged(space.Id, space.UserId)
	}()

	for volName := range space.VolumeData {
		c.Logger.Debug("deleting volume", "name", volName)
		if err := c.volumeRemove(context.Background(), volName); err != nil {
			return err
		}
		delete(space.VolumeData, volName)
	}

	c.Logger.Debug("volumes deleted")
	return nil
}

package apple

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/log"
	"gopkg.in/yaml.v3"
)

type AppleClient struct {
	DriverName string
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
}

type volInfo struct {
	Volumes map[string]interface{} `yaml:"volumes"`
}

type containerInspect struct {
	ID    string `json:"ID"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
}

func NewClient() *AppleClient {
	return &AppleClient{
		DriverName: "apple",
	}
}

func (c *AppleClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	log.Debug(c.DriverName+": creating space job %s", space.Id)

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

	db := database.GetInstance()
	space.IsPending = true
	space.IsDeployed = false
	space.IsDeleting = false
	space.TemplateHash = template.Hash
	space.StartedAt = time.Now().UTC()
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "UpdatedAt", "StartedAt"})
	if err != nil {
		log.Error(c.DriverName+": creating space job %s error %s", space.Id, err)
		return err
	}

	service.GetTransport().GossipSpace(space)

	go func() {
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				log.Error(c.DriverName+": creating space job %s error %s", space.Id, err)
			}
			service.GetTransport().GossipSpace(space)
		}()

		args := []string{"run", "-d", "--name", spec.ContainerName}

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

		args = append(args, spec.Image)
		args = append(args, spec.Command...)

		log.Debug(c.DriverName+": running container %s", spec.ContainerName)
		cmd := exec.Command("container", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(c.DriverName+": creating container %s, error: %s, output: %s", spec.ContainerName, err, string(output))
			return
		}

		containerID := strings.TrimSpace(string(output))
		log.Debug(c.DriverName+": container running %s, %s", spec.ContainerName, containerID)

		db := database.GetInstance()
		space.ContainerId = containerID
		space.IsPending = false
		space.IsDeployed = true
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"ContainerId", "IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			log.Error(c.DriverName+": creating space job %s error %s", space.Id, err)
			return
		}
		service.GetTransport().GossipSpace(space)
	}()

	return nil
}

func (c *AppleClient) DeleteSpaceJob(space *model.Space, onStopped func()) error {
	log.Debug(c.DriverName+": deleting space job %s, %s", space.Id, space.ContainerId)

	db := database.GetInstance()

	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"})
	if err != nil {
		return err
	}

	service.GetTransport().GossipSpace(space)

	go func() {
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				log.Error(c.DriverName+": creating space job %s error %s", space.Id, err)
			}
			service.GetTransport().GossipSpace(space)
		}()

		log.Debug(c.DriverName+": stopping container %s", space.ContainerId)
		cmd := exec.Command("container", "stop", space.ContainerId)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if !strings.Contains(string(output), "not found") {
				log.Error(c.DriverName+": stopping container %s error %s, output: %s", space.ContainerId, err, string(output))
				return
			}
		}

		timeout := time.Now().Add(30 * time.Second)
		for {
			cmd := exec.Command("container", "inspect", space.ContainerId)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if strings.Contains(string(output), "not found") {
					break
				}
				log.Error(c.DriverName+": inspecting container %s error %s", space.ContainerId, err)
				return
			}

			var inspectData []containerInspect
			if err := json.Unmarshal(output, &inspectData); err != nil {
				log.Error(c.DriverName+": parsing inspect output %s error %s", space.ContainerId, err)
				return
			}

			if len(inspectData) > 0 && !inspectData[0].State.Running {
				break
			}

			if time.Now().After(timeout) {
				log.Error(c.DriverName+": timeout waiting for container %s to stop", space.ContainerId)
				return
			}

			log.Debug(c.DriverName+": waiting for container %s to stop", space.ContainerId)
			time.Sleep(500 * time.Millisecond)
		}

		log.Debug(c.DriverName+": removing container %s", space.ContainerId)
		cmd = exec.Command("container", "rm", space.ContainerId)
		output, err = cmd.CombinedOutput()
		if err != nil {
			if !strings.Contains(string(output), "not found") {
				log.Error(c.DriverName+": removing container %s error %s, output: %s", space.ContainerId, err, string(output))
				return
			}
		}

		space.IsPending = false
		space.IsDeployed = false
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"})
		if err != nil {
			log.Error(c.DriverName+": deleting space job %s error %s", space.Id, err)
			return
		}

		service.GetTransport().GossipSpace(space)

		if onStopped != nil {
			onStopped()
		}
	}()

	return nil
}

func (c *AppleClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
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
		log.Debug(c.DriverName + ": no volumes to create")
		return nil
	}

	log.Debug(c.DriverName + ": checking for required volumes")

	db := database.GetInstance()

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
	}()

	for volName := range volInfo.Volumes {
		log.Debug(c.DriverName+": checking volume %s", volName)

		if _, ok := space.VolumeData[volName]; !ok {
			log.Debug(c.DriverName+": creating volume %s", volName)

			cmd := exec.Command("container", "volume", "create", volName)
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Error(c.DriverName+": creating volume %s error %s, output: %s", volName, err, string(output))
				return err
			}

			space.VolumeData[volName] = model.SpaceVolume{
				Id:        volName,
				Namespace: "_apple",
			}
		}
	}

	for volName := range space.VolumeData {
		if _, ok := volInfo.Volumes[volName]; !ok {
			log.Debug(c.DriverName+": deleting volume %s", volName)

			cmd := exec.Command("container", "volume", "rm", volName)
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Error(c.DriverName+": deleting volume %s error %s, output: %s", volName, err, string(output))
				return err
			}

			delete(space.VolumeData, volName)
		}
	}

	log.Debug(c.DriverName + ": volumes checked")

	return nil
}

func (c *AppleClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	log.Debug(c.DriverName + ": deleting volumes")

	if len(space.VolumeData) == 0 {
		log.Debug(c.DriverName + ": no volumes to delete")
		return nil
	}

	defer func() {
		space.UpdatedAt = hlc.Now()
		db.SaveSpace(space, []string{"VolumeData", "UpdatedAt"})
		service.GetTransport().GossipSpace(space)
	}()

	for volName := range space.VolumeData {
		log.Debug(c.DriverName+": deleting volume %s", volName)

		cmd := exec.Command("container", "volume", "rm", volName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(c.DriverName+": deleting volume %s error %s, output: %s", volName, err, string(output))
			return err
		}

		delete(space.VolumeData, volName)
	}

	log.Debug(c.DriverName + ": volumes deleted")

	return nil
}

func (c *AppleClient) CreateVolume(vol *model.Volume, variables map[string]interface{}) error {
	log.Debug(c.DriverName + ": creating volume")

	volumes, err := model.ResolveVariables(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	var volInfo volInfo
	err = yaml.Unmarshal([]byte(volumes), &volInfo)
	if err != nil {
		return err
	}

	if len(volInfo.Volumes) != 1 {
		return fmt.Errorf("volume definition must contain exactly 1 volume")
	}

	for volName := range volInfo.Volumes {
		log.Debug(c.DriverName+": creating volume: %s", volName)

		cmd := exec.Command("container", "volume", "create", volName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(c.DriverName+": creating volume %s error %s, output: %s", volName, err, string(output))
			return err
		}
	}

	log.Debug(c.DriverName + ": volume created")

	return nil
}

func (c *AppleClient) DeleteVolume(vol *model.Volume, variables map[string]interface{}) error {
	log.Debug(c.DriverName + ": deleting volume")

	volumes, err := model.ResolveVariables(vol.Definition, nil, nil, nil, variables)
	if err != nil {
		return err
	}

	var volInfo volInfo
	err = yaml.Unmarshal([]byte(volumes), &volInfo)
	if err != nil {
		return err
	}

	for volName := range volInfo.Volumes {
		log.Debug(c.DriverName+": deleting volume: %s", volName)

		cmd := exec.Command("container", "volume", "rm", volName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(c.DriverName+": deleting volume %s error %s, output: %s", volName, err, string(output))
			return err
		}
	}

	log.Debug(c.DriverName + ": volume deleted")

	return nil
}

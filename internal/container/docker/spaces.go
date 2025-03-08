package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rs/zerolog/log"
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

func (c *DockerClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables *map[string]interface{}) error {

	log.Debug().Msgf("docker: creating space job %s", space.Id)

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
	config := &container.Config{
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
		config.ExposedPorts[nat.Port(ports[1])] = struct{}{}

		// Add the port to the host config
		hostConfig.PortBindings[nat.Port(ports[1])] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: ports[0],
			},
		}
	}

	// Record deploying
	db := database.GetInstance()
	space.IsPending = true
	space.IsDeployed = false
	space.IsDeleting = false
	space.TemplateHash = template.Hash
	space.Location = server_info.LeafLocation
	err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "Location"})
	if err != nil {
		log.Error().Msgf("docker: creating space job %s error %s", space.Id, err)
		return err
	}
	origin.UpdateSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "Location"})

	// launch the container in a go routing to avoid blocking
	go func() {

		// Clean up on exit
		defer func() {
			space.IsPending = false
			if err := db.SaveSpace(space, []string{"IsPending"}); err != nil {
				log.Error().Msgf("docker: creating space job %s error %s", space.Id, err)
			}
			origin.UpdateSpace(space, []string{"IsPending"})
		}()

		// Create a Docker client
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Error().Msgf("docker: creating space job %s error %s", space.Id, err)
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
				log.Error().Msgf("docker: creating space job %s error %s", space.Id, err)
				return
			}
			authStr = base64.URLEncoding.EncodeToString(encodedJSON)
		}

		// Pull the container
		log.Debug().Msgf("docker: pulling image %s", spec.Image)
		reader, err := cli.ImagePull(context.Background(), spec.Image, image.PullOptions{
			RegistryAuth: authStr,
		})
		if err != nil {
			log.Error().Msgf("docker: pulling image %s, error: %s", spec.Image, err)
			return
		}
		defer func() {
			if err := reader.Close(); err != nil {
				log.Error().Msgf("docker: pulling image %s, error: %s", spec.Image, err)
			}
		}()
		io.Copy(os.Stdout, reader)

		// Create the container
		log.Debug().Msgf("docker: creating container %s", spec.ContainerName)
		resp, err := cli.ContainerCreate(
			context.Background(),
			config,
			hostConfig,
			nil,
			nil,
			spec.ContainerName,
		)
		if err != nil {
			log.Error().Msgf("docker: creating container %s, error: %s", spec.ContainerName, err)
			return
		}

		// Start the container
		log.Debug().Msgf("docker: starting container %s, %s", spec.ContainerName, resp.ID)
		err = cli.ContainerStart(context.Background(), resp.ID, container.StartOptions{})
		if err != nil {
			// Failed to start the container so remove it
			cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{})

			log.Error().Msgf("docker: starting container %s, %s, error: %s", spec.ContainerName, resp.ID, err)
			return
		}

		log.Debug().Msgf("docker: container running %s, %s", spec.ContainerName, resp.ID)

		// Record the container ID and that the space is running
		db := database.GetInstance()
		space.ContainerId = resp.ID
		space.IsPending = false
		space.IsDeployed = true
		err = db.SaveSpace(space, []string{"ContainerId", "IsPending", "IsDeployed"})
		if err != nil {
			log.Error().Msgf("docker: creating space job %s error %s", space.Id, err)
			return
		}

		origin.UpdateSpace(space, []string{"ContainerId", "IsPending", "IsDeployed"})
	}()

	return nil
}

func (c *DockerClient) DeleteSpaceJob(space *model.Space) error {
	log.Debug().Msgf("docker: deleting space job %s, %s", space.Id, space.ContainerId)

	db := database.GetInstance()

	space.IsPending = true
	err := db.SaveSpace(space, []string{"IsPending"})
	if err != nil {
		return err
	}
	origin.UpdateSpace(space, []string{"IsPending"})

	// Run the delete in a go routine to avoid blocking
	go func() {
		// Clean up on exit
		defer func() {
			space.IsPending = false
			if err := db.SaveSpace(space, []string{"IsPending"}); err != nil {
				log.Error().Msgf("docker: creating space job %s error %s", space.Id, err)
			}
			origin.UpdateSpace(space, []string{"VolumeData"})
		}()

		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Error().Msgf("docker: deleting space job %s error %s", space.Id, err)
			return
		}

		// Stop the container
		log.Debug().Msgf("docker: stopping container %s", space.ContainerId)
		err = cli.ContainerStop(context.Background(), space.ContainerId, container.StopOptions{})
		if err != nil {
			log.Error().Msgf("docker: stopping container %s error %s", space.ContainerId, err)
			return
		}

		// Remove the container
		log.Debug().Msgf("docker: removing container %s", space.ContainerId)
		err = cli.ContainerRemove(context.Background(), space.ContainerId, container.RemoveOptions{})
		if err != nil {
			log.Error().Msgf("docker: removing container %s error %s", space.ContainerId, err)
			return
		}

		space.IsPending = false
		space.IsDeployed = false
		err = db.SaveSpace(space, []string{"IsPending", "IsDeployed"})
		if err != nil {
			log.Error().Msgf("docker: deleting space job %s error %s", space.Id, err)
			return
		}

		origin.UpdateSpace(space, []string{"IsPending", "IsDeployed"})
	}()

	return nil
}

func (c *DockerClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables *map[string]interface{}) error {

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

	log.Debug().Msg("docker: checking for required volumes")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	db := database.GetInstance()

	// Find the volumes that are defined but not yet created in the space and create them
	for volName, _ := range volInfo.Volumes {
		log.Debug().Msgf("docker: checking volume %s", volName)

		// Check if the volume is already created for the space
		if _, ok := space.VolumeData[volName]; !ok {
			log.Debug().Msgf("docker: creating volume %s", volName)

			volume, err := cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: volName})
			if err != nil {
				db.SaveSpace(space, []string{"VolumeData"}) // Save the space to capture the volumes
				origin.UpdateSpace(space, []string{"VolumeData"})
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
			log.Debug().Msgf("docker: deleting volume %s", volName)

			err := cli.VolumeRemove(context.Background(), volName, true)
			if err != nil {
				db.SaveSpace(space, []string{"VolumeData"}) // Save the space to capture the volumes
				origin.UpdateSpace(space, []string{"VolumeData"})
				return err
			}

			delete(space.VolumeData, volName)
		}
	}

	// Save the space with the volume data
	err = db.SaveSpace(space, []string{"VolumeData"})
	if err != nil {
		return err
	}
	origin.UpdateSpace(space, []string{"VolumeData"})

	log.Debug().Msg("docker: volumes checked")

	return nil
}

func (c *DockerClient) DeleteSpaceVolumes(space *model.Space) error {
	db := database.GetInstance()

	log.Debug().Msg("docker: deleting volumes")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	// For all volumes in the space delete them
	for volName, _ := range space.VolumeData {
		log.Debug().Msgf("docker: deleting volume %s", volName)

		err := cli.VolumeRemove(context.Background(), volName, true)
		if err != nil {
			db.SaveSpace(space, []string{"VolumeData"}) // Save the space to capture the volumes
			origin.UpdateSpace(space, []string{"VolumeData"})

			return err
		}

		delete(space.VolumeData, volName)
		db.SaveSpace(space, []string{"VolumeData"})
		origin.UpdateSpace(space, []string{"VolumeData"})
	}

	log.Debug().Msg("docker: volumes deleted")

	return nil
}

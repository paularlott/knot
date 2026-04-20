package specvalidate

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
	"gopkg.in/yaml.v3"
)

type Issue struct {
	Field   string
	Message string
}

type localContainerAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type localContainerSpec struct {
	ContainerName string              `yaml:"container_name,omitempty"`
	Hostname      string              `yaml:"hostname,omitempty"`
	Image         string              `yaml:"image"`
	Auth          *localContainerAuth `yaml:"auth,omitempty"`
	Ports         []string            `yaml:"ports,omitempty"`
	Volumes       []string            `yaml:"volumes,omitempty"`
	Command       []string            `yaml:"command,omitempty"`
	Privileged    bool                `yaml:"privileged,omitempty"`
	Network       string              `yaml:"network,omitempty"`
	Environment   []string            `yaml:"environment,omitempty"`
	CapAdd        []string            `yaml:"cap_add,omitempty"`
	CapDrop       []string            `yaml:"cap_drop,omitempty"`
	Devices       []string            `yaml:"devices,omitempty"`
	AddHost       []string            `yaml:"add_host,omitempty"`
	DNS           []string            `yaml:"dns,omitempty"`
	DNSSearch     []string            `yaml:"dns_search,omitempty"`
	Memory        string              `yaml:"memory,omitempty"`
	CPUs          string              `yaml:"cpus,omitempty"`
}

type localVolumeSpec struct {
	Volumes map[string]interface{} `yaml:"volumes"`
}

type NomadJobParser func(string) error

func ValidateTemplateSpec(platform, job, volumes string) []Issue {
	return ValidateTemplateSpecWithNomadParser(platform, job, volumes, defaultNomadJobParser)
}

func ValidateTemplateSpecWithNomadParser(platform, job, volumes string, parseNomadJob NomadJobParser) []Issue {
	switch platform {
	case model.PlatformManual:
		return nil
	case model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer:
		issues := validateLocalContainerJob(job)
		issues = append(issues, validateLocalVolumeDefinitions("volumes", volumes, false)...)
		return issues
	case model.PlatformNomad:
		issues := validateNomadJob(job, parseNomadJob)
		issues = append(issues, validateNomadVolumes("volumes", volumes, false)...)
		return issues
	default:
		return []Issue{{Field: "platform", Message: "unsupported platform"}}
	}
}

func ValidateVolumeSpec(platform, definition string) []Issue {
	switch platform {
	case model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer:
		return validateLocalVolumeDefinitions("definition", definition, true)
	case model.PlatformNomad:
		return validateNomadVolumes("definition", definition, true)
	default:
		return []Issue{{Field: "platform", Message: "unsupported platform"}}
	}
}

func defaultNomadJobParser(job string) error {
	client, err := nomad.NewClient()
	if err != nil {
		return err
	}

	_, err = client.ParseJobHCL(job)
	return err
}

func validateNomadJob(job string, parseNomadJob NomadJobParser) []Issue {
	if strings.TrimSpace(job) == "" {
		return []Issue{{Field: "job", Message: "job is required"}}
	}

	if parseNomadJob == nil {
		return []Issue{{Field: "job", Message: "no Nomad validator is configured"}}
	}

	// Resolve template variables before sending to Nomad, so ${{.user.username}} etc. become placeholder values.
	// If resolution fails (e.g. config not initialised), fall back to raw job.
	// Resolve template variables before sending to Nomad, so ${{.user.username}} etc. become placeholder values.
	// If resolution panics or fails (e.g. config not initialised), fall back to raw job.
	resolved := job
	func() {
		defer func() { recover() }()
		if r, err := model.ResolveVariables(job, &model.Template{}, nil, nil, nil); err == nil {
			resolved = r
		}
	}()

	if err := parseNomadJob(resolved); err != nil {
		return []Issue{{Field: "job", Message: err.Error()}}
	}

	return nil
}

func validateLocalContainerJob(job string) []Issue {
	if strings.TrimSpace(job) == "" {
		return []Issue{{Field: "job", Message: "container specification is required"}}
	}

	var spec localContainerSpec
	if err := decodeYAMLStrict(job, &spec); err != nil {
		return []Issue{{Field: "job", Message: err.Error()}}
	}

	var issues []Issue
	if strings.TrimSpace(spec.Image) == "" {
		issues = append(issues, Issue{Field: "job", Message: "image must be set"})
	}

	for _, port := range spec.Ports {
		if err := validatePortMapping(port); err != nil {
			issues = append(issues, Issue{Field: "job", Message: err.Error()})
		}
	}

	for _, mount := range spec.Volumes {
		if err := validateHostContainerPair(mount, "volume"); err != nil {
			issues = append(issues, Issue{Field: "job", Message: err.Error()})
		}
	}

	for _, device := range spec.Devices {
		if err := validateHostContainerPair(device, "device"); err != nil {
			issues = append(issues, Issue{Field: "job", Message: err.Error()})
		}
	}

	for _, entry := range spec.Environment {
		if err := validateEnvEntry(entry); err != nil {
			issues = append(issues, Issue{Field: "job", Message: err.Error()})
		}
	}

	for _, entry := range spec.AddHost {
		if err := validateAddHostEntry(entry); err != nil {
			issues = append(issues, Issue{Field: "job", Message: err.Error()})
		}
	}

	for _, entry := range spec.DNS {
		if err := validateIPEntry(entry, "dns"); err != nil {
			issues = append(issues, Issue{Field: "job", Message: err.Error()})
		}
	}

	if spec.Memory != "" {
		if _, err := util.ConvertToBytes(spec.Memory); err != nil {
			issues = append(issues, Issue{Field: "job", Message: fmt.Sprintf("invalid memory value %q", spec.Memory)})
		}
	}

	if spec.CPUs != "" {
		cpus, err := strconv.ParseFloat(spec.CPUs, 64)
		if err != nil || cpus <= 0 {
			issues = append(issues, Issue{Field: "job", Message: fmt.Sprintf("invalid cpu value %q", spec.CPUs)})
		}
	}

	return issues
}

func validateLocalVolumeDefinitions(field, volumes string, requireSingle bool) []Issue {
	if strings.TrimSpace(volumes) == "" {
		if requireSingle {
			return []Issue{{Field: field, Message: "volume definition is required"}}
		}
		return nil
	}

	var spec localVolumeSpec
	if err := decodeYAMLStrict(volumes, &spec); err != nil {
		return []Issue{{Field: field, Message: err.Error()}}
	}

	if spec.Volumes == nil {
		return []Issue{{Field: field, Message: "volume definition must contain a top-level volumes map"}}
	}
	if requireSingle && len(spec.Volumes) != 1 {
		return []Issue{{Field: field, Message: "volume definition must contain exactly 1 volume"}}
	}
	if len(spec.Volumes) == 0 {
		return []Issue{{Field: field, Message: "volume definition must contain at least 1 volume"}}
	}

	return nil
}

func validateNomadVolumes(field, volumes string, requireSingle bool) []Issue {
	if strings.TrimSpace(volumes) == "" {
		if requireSingle {
			return []Issue{{Field: field, Message: "volume definition is required"}}
		}
		return nil
	}

	definitions, err := model.LoadVolumesFromYaml(volumes, nil, nil, nil, nil)
	if err != nil {
		return []Issue{{Field: field, Message: err.Error()}}
	}

	if requireSingle && len(definitions.Volumes) != 1 {
		return []Issue{{Field: field, Message: "volume definition must contain exactly 1 volume"}}
	}
	if len(definitions.Volumes) == 0 {
		return []Issue{{Field: field, Message: "volume definition must contain at least 1 volume"}}
	}

	var issues []Issue
	for _, volume := range definitions.Volumes {
		if strings.TrimSpace(volume.Name) == "" {
			issues = append(issues, Issue{Field: field, Message: "volume name must be set"})
		}
		if strings.TrimSpace(volume.PuluginId) == "" {
			issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("volume %q must set plugin_id", volume.Name)})
		}
		if volume.Type != "csi" && volume.Type != "host" {
			issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("volume %q has unsupported type %q", volume.Name, volume.Type)})
		}
	}

	return issues
}

func decodeYAMLStrict(data string, target interface{}) error {
	decoder := yaml.NewDecoder(strings.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s", cleanYAMLError(err.Error()))
	}
	return nil
}

// cleanYAMLError strips verbose yaml.v3 internals from error messages.
func cleanYAMLError(msg string) string {
	msg = strings.TrimPrefix(msg, "yaml: ")
	msg = strings.TrimPrefix(msg, "unmarshal errors:\n  ")

	// Simplify type mismatch errors, e.g.
	//   "line 2: cannot unmarshal !!str `paul-te...` into map[string]interface {}"
	// becomes "line 2: unexpected value type"
	if before, _, found := strings.Cut(msg, ": cannot unmarshal"); found {
		return before + ": unexpected value type"
	}

	return msg
}

func validatePortMapping(port string) error {
	parts := strings.Split(port, ":")
	if len(parts) != 2 {
		return fmt.Errorf("port %q must be in the format hostPort:containerPort[/protocol]", port)
	}

	if err := validatePortNumber(parts[0], "host"); err != nil {
		return fmt.Errorf("port %q %w", port, err)
	}

	containerPort := parts[1]
	if strings.Contains(containerPort, "/") {
		subParts := strings.Split(containerPort, "/")
		if len(subParts) != 2 {
			return fmt.Errorf("port %q must be in the format hostPort:containerPort[/protocol]", port)
		}
		containerPort = subParts[0]
		protocol := strings.ToLower(subParts[1])
		if protocol != "tcp" && protocol != "udp" {
			return fmt.Errorf("port %q must use tcp or udp", port)
		}
	}

	if err := validatePortNumber(containerPort, "container"); err != nil {
		return fmt.Errorf("port %q %w", port, err)
	}

	return nil
}

func validatePortNumber(value, side string) error {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("must define a valid %s port", side)
	}
	return nil
}

func validateHostContainerPair(value, kind string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("%s %q must be in the format hostPath:containerPath", kind, value)
	}
	return nil
}

func validateEnvEntry(value string) error {
	if !strings.Contains(value, "=") {
		return fmt.Errorf("environment entry %q must be in the format KEY=value", value)
	}
	key := strings.SplitN(value, "=", 2)[0]
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("environment entry %q must set a variable name", value)
	}
	return nil
}

func validateAddHostEntry(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return fmt.Errorf("add_host entry %q must be in the format hostname:ip", value)
	}
	if net.ParseIP(parts[1]) == nil {
		return fmt.Errorf("add_host entry %q must use a valid IP address", value)
	}
	return nil
}

func validateIPEntry(value, field string) error {
	if net.ParseIP(value) == nil {
		return fmt.Errorf("%s entry %q must be a valid IP address", field, value)
	}
	return nil
}

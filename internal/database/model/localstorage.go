package model

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type LocalVolumeEntry struct {
	Size string `yaml:"size,omitempty"`
}

type PathList []string

func (paths *PathList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("paths entries must be strings")
			}
			path := strings.TrimSpace(item.Value)
			if path != "" {
				*paths = append(*paths, path)
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(value.Content); i += 2 {
			path := strings.TrimSpace(value.Content[i].Value)
			if path != "" {
				*paths = append(*paths, path)
			}
		}
	case yaml.ScalarNode:
		path := strings.TrimSpace(value.Value)
		if path != "" {
			*paths = append(*paths, path)
		}
	default:
		return fmt.Errorf("paths must be a list, map, or string")
	}

	return nil
}

type LocalStorageSpec struct {
	Volumes map[string]LocalVolumeEntry `yaml:"volumes"`
	Paths   PathList                    `yaml:"paths"`
}

type ManagedPathsSpec struct {
	Paths PathList `yaml:"paths"`
}

func LoadLocalStorageFromYaml(yamlData string, t *Template, space *Space, user *User, variables map[string]interface{}) (*LocalStorageSpec, error) {
	var err error

	if t != nil || space != nil || user != nil || variables != nil {
		yamlData, err = ResolveVariables(yamlData, t, space, user, variables)
		if err != nil {
			return nil, err
		}
	}

	spec := &LocalStorageSpec{}
	if err = yaml.Unmarshal([]byte(yamlData), spec); err != nil {
		return nil, err
	}
	if spec.Volumes == nil {
		spec.Volumes = make(map[string]LocalVolumeEntry)
	}

	return spec, nil
}

func LoadManagedPathsFromYaml(yamlData string, t *Template, space *Space, user *User, variables map[string]interface{}) (*ManagedPathsSpec, error) {
	var err error

	if t != nil || space != nil || user != nil || variables != nil {
		yamlData, err = ResolveVariables(yamlData, t, space, user, variables)
		if err != nil {
			return nil, err
		}
	}

	var root yaml.Node
	if err = yaml.Unmarshal([]byte(yamlData), &root); err != nil {
		return nil, err
	}

	spec := &ManagedPathsSpec{}
	if len(root.Content) == 0 {
		return spec, nil
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return spec, nil
	}

	for i := 0; i < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "paths" {
			continue
		}
		if err = doc.Content[i+1].Decode(&spec.Paths); err != nil {
			return nil, err
		}
		break
	}

	return spec, nil
}
